#!/usr/bin/env python3
"""Run pinned upstream Rust tests through Oxide and guard recorded successes."""

from __future__ import annotations

import argparse
from collections import Counter
import errno
import fcntl
import hashlib
import json
import os
from pathlib import Path
import platform
import queue
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import threading
import time
import tomllib

from baseline import promote_baseline, validate_baseline
from failure_groups import failure_groups, named_failure_groups
from shared_failure import shared_failure
from source import instrument_tests

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
SUITES = {
    "coretests": ("coretests", "tests/lib.rs"),
    "alloctests": ("alloctests", "tests/lib.rs"),
    "alloctests-internal": ("alloctests", "lib.rs"),
    "c-str-alloc-error": ("alloctests", "tests/c_str_alloc_error.rs"),
    "vec-deque-alloc-error": ("alloctests", "tests/vec_deque_alloc_error.rs"),
}
# Preserve debug assertions and MIR, but use the correct native instruction
# selector. AArch64 GlobalISel double-rounds f16 FMA (LLVM #98389) and narrows
# saturating f16 -> i128/u128 casts. Never use that machine code as an oracle.
FLAGS = ["-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes",
         "-Cllvm-args=-global-isel=0"]
DISCOVERY = r'''
extern crate test as __oxide_test;
#[cfg(oxide_discovery)]
fn __oxide_discover(tests: &[&__oxide_test::TestDescAndFn]) {
    fn hex(s: &str) -> ::std::string::String {
        s.as_bytes().iter().map(|b| ::std::format!("{b:02x}")).collect()
    }
    for t in tests {
        if !matches!(t.testfn, __oxide_test::StaticTestFn(_)) { continue; }
        let (kind, expected) = match t.desc.should_panic {
            __oxide_test::ShouldPanic::No => ("no", ""),
            __oxide_test::ShouldPanic::Yes => ("yes", ""),
            __oxide_test::ShouldPanic::YesWithMessage(s) => ("message", s),
        };
        ::std::println!("{}\t{}\t{}\t{}\t{}", t.desc.name.as_slice(),
            t.desc.ignore, kind, hex(expected), hex(t.desc.ignore_message.unwrap_or("")));
    }
}
'''


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def tree_hash(path: Path) -> str:
    h = hashlib.sha256()
    for file in sorted(p for p in path.rglob("*") if p.is_file()):
        h.update(file.relative_to(path).as_posix().encode() + b"\0")
        h.update(file.read_bytes())
        h.update(b"\0")
    return h.hexdigest()


def save(path: Path, value: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(mode="w", dir=path.parent, prefix=path.name + ".", delete=False) as output:
        temporary = Path(output.name)
        output.write(json.dumps(value, indent=2, sort_keys=True) + "\n")
    try:
        temporary.replace(path)
    finally:
        temporary.unlink(missing_ok=True)


class CommandFailure(RuntimeError):
    def __init__(self, stage: str, log: Path, timeout: bool = False):
        super().__init__(f"{stage} {'timed out' if timeout else 'failed'}; see {log}")
        self.stage, self.log, self.timeout = stage, log, timeout


class InfrastructureFailure(RuntimeError):
    """A resource failure invalidates the run, rather than any candidate test."""

    def __init__(self, stage, log, reason):
        super().__init__(f"infrastructure failure during {stage}: {reason}; see {log}")
        self.stage, self.log, self.reason = stage, log, reason


RESOURCE_ERRNOS = {errno.ENOSPC, errno.EDQUOT, errno.EMFILE, errno.ENFILE, errno.ENOMEM, errno.EAGAIN}
RESOURCE_MESSAGES = (
    "no space left on device", "disk quota exceeded", "too many open files",
    "cannot allocate memory", "resource temporarily unavailable", "failed to create new os thread",
)


def check_infrastructure(log, stage, control):
    diagnostic = log.read_text(errors="replace")
    if diagnostic.startswith("$ "):
        diagnostic = diagnostic.partition("\n")[2]
    lowered = diagnostic.lower()
    for message in RESOURCE_MESSAGES:
        if message in lowered:
            if control is not None:
                control.cancel()
            raise InfrastructureFailure(stage, log, message)


class CommandCancelled(RuntimeError):
    """The run was stopped; this must never become a per-case failure/pass."""


def kill_process_group(proc):
    try:
        os.killpg(proc.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass


class CommandControl:
    """Atomically stop new subprocesses and terminate every active process group."""

    def __init__(self):
        self.lock = threading.RLock()
        self.cancelled = threading.Event()
        self.processes = set()

    def check(self):
        if self.cancelled.is_set():
            raise CommandCancelled("upstream execution cancelled")

    def start(self, args, **options):
        with self.lock:
            self.check()
            proc = subprocess.Popen(args, **options)
            self.processes.add(proc)
            return proc

    def finished(self, proc):
        with self.lock:
            self.processes.discard(proc)

    def cancel(self):
        with self.lock:
            self.cancelled.set()
            for proc in self.processes:
                kill_process_group(proc)


def command(args, *, cwd: Path, env: dict, log: Path, stage: str, timeout: int,
            control: CommandControl | None = None):
    try:
        return _command(args, cwd=cwd, env=env, log=log, stage=stage, timeout=timeout, control=control)
    except OSError as error:
        if error.errno not in RESOURCE_ERRNOS:
            raise
        if control is not None:
            control.cancel()
        raise InfrastructureFailure(stage, log, str(error)) from error


def _command(args, *, cwd: Path, env: dict, log: Path, stage: str, timeout: int,
             control: CommandControl | None):
    log.parent.mkdir(parents=True, exist_ok=True)
    with log.open("w") as output:
        output.write("$ " + repr([str(arg) for arg in args]) + "\n")
        output.flush()
        start = control.start if control is not None else subprocess.Popen
        proc = start([str(arg) for arg in args], cwd=cwd, env=env,
                     stdout=output, stderr=subprocess.STDOUT, start_new_session=True)
        try:
            code = proc.wait(timeout=timeout)
        except subprocess.TimeoutExpired as error:
            kill_process_group(proc)
            proc.wait()
            if control is not None:
                control.check()
            check_infrastructure(log, stage, control)
            raise CommandFailure(stage, log, True) from error
        except BaseException:
            if control is not None:
                control.cancel()
            else:
                kill_process_group(proc)
            proc.wait()
            raise
        finally:
            if control is not None:
                control.finished(proc)
        if control is not None:
            control.check()
        if code:
            check_infrastructure(log, stage, control)
            raise CommandFailure(stage, log)
    return log.read_text(errors="replace").split("\n", 1)[1]


def executable_artifact(output: str, suite: str) -> Path:
    executables = []
    for line in output.splitlines():
        if line.startswith("{"):
            message = json.loads(line)
            if message.get("reason") == "compiler-artifact" and message.get("executable"):
                executables.append(Path(message["executable"]))
    if len(executables) != 1:
        raise RuntimeError(f"expected one executable for {suite}, got {executables}")
    return executables[0]


class Runner:
    def __init__(self, args):
        self.args = args
        self.frontend = Path(os.environ.get("OXIDE_FRONTEND", ROOT / "bin/oxide-rs")).resolve()
        if not self.frontend.is_file():
            raise RuntimeError("build the compiler first: ./oxide-rs/build.sh")
        self.sysroot = Path(subprocess.check_output([self.frontend, "--print-sysroot"], text=True).strip())
        self.cargo, self.rustc = self.sysroot / "bin/cargo", self.sysroot / "bin/rustc"
        version = subprocess.check_output([self.rustc, "-vV"], text=True)
        self.commit = re.search(r"^commit-hash: (.+)$", version, re.M)[1]
        self.arch = {"aarch64": "arm64", "x86_64": "amd64"}.get(platform.machine())
        if sys.platform != "linux" or self.arch is None:
            raise RuntimeError("upstream execution requires Linux arm64 or amd64")
        self.triple = {"arm64": "aarch64", "amd64": "x86_64"}[self.arch] + "-unknown-linux-gnu"
        self.library = self.sysroot / "lib/rustlib/src/rust/library"
        self.lock = json.loads((HERE / "upstream.lock.json").read_text())
        if self.commit != self.lock["rust_commit"]:
            raise RuntimeError("rustc revision differs from upstream.lock.json; review the source upgrade first")
        for name, expected in self.lock["source_sha256"].items():
            source = self.library / name
            actual = tree_hash(source) if source.is_dir() else digest(source)
            if actual != expected:
                raise RuntimeError(f"pinned upstream source changed: {name}")
        self.context = {"rust_commit": self.commit, "source_sha256": self.lock["source_sha256"],
                        "target": self.triple, "profile": "debug", "rustflags": FLAGS,
                        "harness_schema": 1, "cargo_lock_sha256": digest(HERE / "Cargo.lock"),
                        "discovery_sha256": hashlib.sha256(DISCOVERY.encode()).hexdigest(),
                        "adapter_sha256": {name: digest(HERE / name) for name in
                            ("source.py", "export_wrapper.py", "macros/Cargo.toml", "macros/src/lib.rs")}}
        self.basefile = args.baseline or HERE / f"baseline-linux-{self.arch}.json"
        self.cache = ROOT / ".cache/upstream" / self.arch
        self.cache.mkdir(parents=True, exist_ok=True)
        # Protect Cargo artifacts, latest reports and monotonic promotion from
        # overlapping invocations (including an explicitly shared baseline).
        self.locks = []
        lock_paths = [self.cache / "runner.lock", self.cache.parent / (
            "baseline-" + hashlib.sha256(str(self.basefile.resolve()).encode()).hexdigest()[:20] + ".lock")]
        for path in sorted(lock_paths):
            handle = path.open("w")
            try:
                fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError as error:
                handle.close()
                raise RuntimeError(f"another upstream run owns {path}") from error
            self.locks.append(handle)
        self.base = json.loads(self.basefile.read_text()) if self.basefile.exists() else None
        # Never reuse exports or execution results across invocations. Cargo's
        # dependency artifacts may be reused, but the frontend always runs.
        self.work = Path(tempfile.mkdtemp(prefix="run-", dir=self.cache))
        self.report_path = args.report or self.cache / "latest.json"
        self.env = os.environ.copy()
        for key in list(self.env):
            if key.startswith("CARGO_PROFILE_"):
                del self.env[key]
        self.env.update(RUSTC=str(self.rustc), RUSTC_WRAPPER=str(HERE / "export_wrapper.py"),
                        OXIDE_UPSTREAM_FRONTEND=str(self.frontend),
                        RUSTC_WORKSPACE_WRAPPER="", RUSTC_BOOTSTRAP="1", OXIDE_EXPORT="",
                        OXIDE_ROOTS="", RUSTFLAGS="", CARGO_ENCODED_RUSTFLAGS="\x1f".join(FLAGS),
                        CARGO_TARGET_DIR=str(self.cache / "cargo-target"),
                        CARGO_INCREMENTAL="0",
                        CARGO_PROFILE_DEV_OPT_LEVEL="0", CARGO_PROFILE_TEST_OPT_LEVEL="0",
                        CARGO_PROFILE_DEV_DEBUG_ASSERTIONS="true", CARGO_PROFILE_TEST_DEBUG_ASSERTIONS="true",
                        GOOS="linux", GOARCH=self.arch, CGO_ENABLED="0", GOWORK="off", GOFLAGS="")
        self.env.setdefault("GOMAXPROCS", "4")
        self.env.setdefault("GOGC", "50")
        self.env.setdefault("GOMEMLIMIT", "8GiB")
        self.configure_go_storage()
        self.cases, self.results, self.details = {}, {}, {}
        self.native = {}
        self.batch_number = 0
        self.started = time.time()
        self.state_lock = threading.RLock()
        self.control = CommandControl()

    def command(self, *args, **kwargs):
        return command(*args, control=self.control, **kwargs)

    def configure_go_storage(self):
        # Go's cache is concurrency-safe; temporary compiler outputs get a
        # unique run directory. Neither should fill an ambient /tmp tmpfs.
        for variable, directory in (("GOCACHE", self.cache / "go-cache"),
                                    ("GOTMPDIR", self.work / "go-tmp")):
            directory.mkdir(parents=True, exist_ok=True)
            self.env[variable] = str(directory.resolve())

    def cargo_args(self, action, suite):
        package = SUITES[suite][0]
        return [self.cargo, action, "-Zbuild-std=std,panic_unwind,test", "--locked",
                "--manifest-path", self.work / "library" / package / "Cargo.toml",
                "--target", self.triple, "--test", suite.replace("-", "_")]

    def prepare(self):
        staged = self.work / "library"
        for name in ("coretests", "alloctests", "alloc"):
            shutil.copytree(self.library / name, staged / name)
        for path in staged.rglob("*.rs"):
            path.write_text(instrument_tests(path.read_text()))
        for suite, (package, entry) in SUITES.items():
            path = staged / package / entry
            source = path.read_text()
            has_test = bool(re.search(r"#!\[feature\([^)]*\btest\b[^)]*\)\]", source))
            features = "custom_test_frameworks" + ("" if has_test else ", test")
            # Inner doc comments must remain before the injected items.
            header = (f"#![feature({features})]\n"
                      "#![cfg_attr(oxide_discovery, test_runner(__oxide_discover))]\n"
                      "#![allow(unexpected_cfgs)]\n")
            path.write_text(header + source + DISCOVERY)
        for package in ("coretests", "alloctests"):
            upstream = tomllib.loads((self.library / package / "Cargo.toml").read_text())
            dependencies = upstream["dev-dependencies"]
            manifest = ('[package]\nname = "' + package + '"\nversion = "0.0.0"\n'
                        'edition = "2024"\nautolib = false\nautotests = false\nautobenches = false\n'
                        '[dependencies]\noxide-upstream-macros = { path = ' + json.dumps(str(HERE / "macros")) + ' }\n')
            for dependency, config in dependencies.items():
                if isinstance(config, str):
                    config = {"version": config}
                fields = ['version = "=' + self.lock["dependencies"][dependency] + '"']
                if "default-features" in config:
                    fields.append("default-features = " + str(config["default-features"]).lower())
                if "features" in config:
                    fields.append("features = " + json.dumps(config["features"]))
                manifest += dependency + " = { " + ", ".join(fields) + " }\n"
            for suite, (owner, entry) in SUITES.items():
                if owner == package:
                    manifest += f'[[test]]\nname = "{suite.replace("-", "_")}"\npath = "{entry}"\n'
            (staged / package / "Cargo.toml").write_text(manifest)
        (staged / "Cargo.toml").write_text('[workspace]\nmembers = ["coretests", "alloctests"]\nresolver = "3"\n')
        shutil.copyfile(HERE / "Cargo.lock", staged / "Cargo.lock")

    def discover(self):
        for suite in SUITES:
            print(f"Discovering {suite} through rustc/libtest...", flush=True)
            # JSON compiler artifacts identify the executable without guessing a hash.
            args = self.cargo_args("test", suite) + ["--no-run", "--message-format=json"]
            output = self.command(args, cwd=ROOT, env=self.env, log=self.work / f"{suite}-native.log",
                             stage="native-build", timeout=self.args.build_timeout)
            executable = executable_artifact(output, suite)
            # Copy before the discovery compilation replaces Cargo's executable.
            self.native[suite] = self.work / f"{suite}.native"
            shutil.copyfile(executable, self.native[suite])
            self.native[suite].chmod(0o755)
            native_list = self.command([self.native[suite], "--list", "--format=terse"], cwd=ROOT,
                                  env=self.env, log=self.work / f"{suite}-list.log",
                                  stage="native-list", timeout=60)
            names = {line.removesuffix(": test") for line in native_list.splitlines() if line.endswith(": test")}
            discovery_build = self.command(self.cargo_args("rustc", suite) + ["--message-format=json", "--", "--cfg=oxide_discovery"], cwd=ROOT,
                                      env=self.env, log=self.work / f"{suite}-discover-build.log",
                                      stage="discovery-build", timeout=self.args.build_timeout)
            output = self.command([executable_artifact(discovery_build, suite)], cwd=ROOT, env=self.env,
                             log=self.work / f"{suite}-discovery.log", stage="discovery", timeout=60)
            discovered = set()
            for line in output.splitlines():
                parts = line.split("\t")
                if len(parts) != 5 or parts[1] not in ("true", "false") or parts[2] not in ("no", "yes", "message"):
                    raise RuntimeError(f"invalid compiler discovery record: {line!r}")
                name, ignore, panic, expected, reason = parts
                if name in discovered:
                    raise RuntimeError(f"duplicate upstream test: {suite}/{name}")
                discovered.add(name)
                case_id = suite + "/" + name
                module, _, function = name.rpartition("::")
                path = (module + "::" if module else "") + "__oxide_upstream_" + function
                self.cases[case_id] = {"suite": suite, "name": name, "ignored": ignore == "true",
                                       "should_panic": panic, "expected": bytes.fromhex(expected).decode(),
                                       "ignore_reason": bytes.fromhex(reason).decode(),
                                       "root": suite.replace("-", "_") + "::" + path}
            if discovered != names or not discovered:
                raise RuntimeError(f"native and instrumented discovery differ for {suite}")
            print(f"  {len(discovered)} tests", flush=True)
        self.results = {key: "ignored" if value["ignored"] else "not_run" for key, value in self.cases.items()}
        self.write_report()

    def write_report(self):
        # Hold the lock through the atomic replace: an older snapshot must not
        # overwrite results that another worker has already published.
        with self.state_lock:
            save(self.report_path, {"schema": 1, "context": self.context, "discovered": sorted(self.cases),
                                   "results": self.results, "cases": self.cases, "details": self.details,
                                   "counts": dict(Counter(self.results.values())), "logs": str(self.work),
                                   "limits": {"jobs": self.args.jobs, "batch_size": self.args.batch_size,
                                              "test_timeout_seconds": self.args.timeout,
                                              "build_timeout_seconds": self.args.build_timeout},
                                   "go_storage": {key: self.env.get(key) for key in ("GOCACHE", "GOTMPDIR")},
                                   "elapsed_seconds": round(time.time() - self.started, 2)})

    def record(self, case_id, status, detail=""):
        with self.state_lock:
            self.results[case_id] = status
            if detail:
                self.details[case_id] = str(detail)

    def native_check(self, case_id):
        case = self.cases[case_id]
        path = self.work / "native-results" / (hashlib.sha256(case_id.encode()).hexdigest()[:16] + ".log")
        try:
            output = self.command([self.native[case["suite"]], case["name"], "--exact", "--test-threads=1", "--nocapture"],
                             cwd=ROOT, env=self.env, log=path, stage="native", timeout=self.args.timeout)
            if not re.search(r"test result: ok\. 1 passed; 0 failed; 0 ignored;", output):
                raise CommandFailure("native-result", path)
        except CommandFailure as error:
            self.record(case_id, "native_timeout" if error.timeout else "native_failed", error)
            return False
        return True

    def batch(self, suite, ids, cargo_target=None):
        self.control.check()
        if not ids:
            return
        with self.state_lock:
            self.batch_number += 1
            stage = self.work / f"batch-{self.batch_number:05}"
            stage.mkdir()
        roots = [self.cases[key]["root"] for key in ids]
        env = self.env | {"OXIDE_ROOTS": ",".join(roots)}
        if cargo_target is not None:
            env["CARGO_TARGET_DIR"] = str(cargo_target)
        mir = stage / "oxide.mir.json"
        generated = stage / "go"
        try:
            artifacts = self.test_artifact_snapshot(suite, Path(env["CARGO_TARGET_DIR"]))
            try:
                self.command(self.cargo_args("rustc", suite) + ["--", "--emit=metadata", "--oxide-export=" + str(mir)],
                        cwd=ROOT, env=env, log=stage / "export.log", stage="export", timeout=self.args.build_timeout)
            finally:
                # Exported MIR is in this batch's stage. Drop only new native
                # test-crate outputs before any emit failure causes recursion.
                self.clean_test_artifacts(artifacts)
            api = json.loads(mir.with_suffix(".api.json").read_text())
            if {root["name"] for root in api["roots"]} != set(roots):
                raise RuntimeError("export roots differ from requested tests")
            self.command([self.cache / "oxide", "emit", "-mir", mir, "-out", generated, "-package", "upstream"],
                    cwd=ROOT, env=self.env, log=stage / "emit.log", stage="emit", timeout=self.args.build_timeout)
            names = {}
            for source in generated.glob("oxide_gen*.go"):
                for go_name, rust_name in re.findall(r"^// (\w+) translates (.+)\.$", source.read_text(), re.M):
                    if rust_name in roots:
                        names[rust_name] = go_name
            if set(names) != set(roots):
                raise RuntimeError("generated Go entry points differ from requested tests")
            harness = ('package upstream\nimport ("testing"; oxide "github.com/csbxd/oxide/oxide-go/runtime")\n')
            for index, root in enumerate(roots):
                harness += (f'func TestUpstream{index:05}(t *testing.T) {{\n'
                            'ctx := oxide.NewContext(); defer ctx.Close()\n'
                            f'if got := {names[root]}(ctx); got != 0 {{ t.Fatalf("Rust test outcome = %d", got) }}\n'
                            'if ctx.Failed() { t.Fatal("uncaught Rust panic") }\n}\n')
            (generated / "upstream_test.go").write_text(harness)
            (generated / "go.mod").write_text('module oxide-upstream-tests\n\ngo 1.27.1\n'
                'require github.com/csbxd/oxide/oxide-go v0.0.0\n'
                'replace github.com/csbxd/oxide/oxide-go => ' + json.dumps(str(ROOT / "oxide-go")) + '\n')
            self.command(["go", "test", "-mod=mod", "-p=1", "-c", "-o", "upstream.test", "."],
                    cwd=generated, env=self.env, log=stage / "go-build.log", stage="go-build", timeout=self.args.build_timeout)
        except CommandFailure as error:
            if error.stage == "emit" and not error.timeout:
                proof = shared_failure(mir, error.log)
                if proof is not None and set(proof["roots"]) == set(roots):
                    self.control.check()
                    proof_log = stage / "shared-failure.log"
                    proof_log.write_text(json.dumps(proof, indent=2, sort_keys=True) + "\n")
                    for case_id in ids:
                        self.record(case_id, "blocked", f"{error}; shared support proof: {proof_log}")
                    self.write_report()
                    print(f"  {suite}: {len(ids)} cases blocked by shared support; proof: {proof_log}", flush=True)
                    self.clean_artifacts(stage)
                    return
            # A failed batch is diagnostic only. Bisect to avoid marking other
            # tests failed just because they share a compilation with one gap.
            if len(ids) > 1:
                groups = (failure_groups(generated, error.log, roots) if error.stage == "go-build"
                          else named_failure_groups(error.log, roots))
                if groups:
                    by_root = dict(zip(roots, ids))
                    print(f"  Retrying {len(ids)} cases in diagnostic groups "
                          f"{[len(group) for group in groups]}", flush=True)
                    suspects, remaining = groups
                    retry_groups = [[root] for root in suspects] if len(suspects) <= 4 else [suspects]
                    for group in retry_groups + [remaining]:
                        self.batch(suite, [by_root[root] for root in group], cargo_target=cargo_target)
                else:
                    middle = len(ids) // 2
                    self.batch(suite, ids[:middle], cargo_target=cargo_target)
                    self.batch(suite, ids[middle:], cargo_target=cargo_target)
            else:
                self.record(ids[0], error.stage + ("_timeout" if error.timeout else "_failed"), error)
            self.write_report()
            self.clean_artifacts(stage)
            return
        for index, case_id in enumerate(ids):
            self.control.check()
            log = stage / f"test-{index:05}.log"
            try:
                output = self.command([generated / "upstream.test", f"-test.run=^TestUpstream{index:05}$",
                                  "-test.count=1", "-test.v", f"-test.timeout={self.args.timeout}s"],
                                 cwd=generated, env=self.env, log=log, stage="run", timeout=self.args.timeout + 5)
                if f"--- PASS: TestUpstream{index:05} " not in output:
                    raise CommandFailure("missing-result", log)
                self.record(case_id, "passed")
            except CommandFailure as error:
                go_timeout = re.search(r"^panic: test timed out after ",
                                       error.log.read_text(errors="replace"), re.M)
                self.record(case_id, "timeout" if error.timeout or go_timeout else "failed", error)
        self.write_report()
        with self.state_lock:
            print(f"  {suite}: {dict(Counter(self.results.values()))}", flush=True)
        self.clean_artifacts(stage)

    def clean_artifacts(self, stage):
        if not self.args.keep_generated:
            shutil.rmtree(stage / "go", ignore_errors=True)
            for path in stage.glob("*.json"):
                path.unlink()

    def test_artifact_snapshot(self, suite, cargo_target):
        package = SUITES[suite][0]
        parents = [cargo_target / self.triple / "debug/build" / package,
                   cargo_target / "debug/build" / package]
        return {parent: {entry.name for entry in parent.iterdir()} if parent.is_dir() else set()
                for parent in parents if not parent.is_symlink()}

    def clean_test_artifacts(self, before):
        for parent, existing in before.items():
            if not parent.is_dir() or parent.is_symlink():
                continue
            for entry in parent.iterdir():
                if entry.name not in existing and entry.is_dir() and not entry.is_symlink():
                    shutil.rmtree(entry)

    def prepare_worker_caches(self):
        """Seed independent writable Cargo targets after discovery completes."""
        source = self.cache / "cargo-target"
        targets = []
        for index in range(self.args.jobs):
            target = self.cache / f"cargo-target-worker-{index}"
            target.mkdir(parents=True, exist_ok=True)
            # Reflinks share disk blocks until modified, never writable inodes.
            # Recopy on each invocation, including after an interrupted copy.
            self.command(["cp", "--reflink=auto", "-a", str(source) + "/.", target],
                         cwd=ROOT, env=self.env, log=self.work / f"worker-{index}-cache.log",
                         stage="worker-cache", timeout=self.args.build_timeout)
            targets.append(target)
        return targets

    def run_batches(self, selected):
        tasks = queue.Queue()
        for suite in SUITES:
            ids = [key for key in sorted(selected) if key in self.cases and self.cases[key]["suite"] == suite
                   and not self.cases[key]["ignored"]]
            for start in range(0, len(ids), self.args.batch_size):
                tasks.put((suite, ids[start:start + self.args.batch_size]))

        def run_group(suite, ids, cargo_target):
            group = []
            for case_id in ids:
                self.control.check()
                if self.native_check(case_id):
                    group.append(case_id)
            self.control.check()
            self.batch(suite, group, cargo_target=cargo_target)
            self.write_report()

        if self.args.jobs == 1:
            try:
                while not tasks.empty():
                    run_group(*tasks.get_nowait(), None)
            except BaseException:
                self.control.cancel()
                raise
            return
        if tasks.empty():
            return

        targets = self.prepare_worker_caches()
        errors = []
        error_lock = threading.Lock()

        def original_error():
            # Resource detection cancels peers before the detecting worker can
            # enqueue its exception. A peer's cancellation may arrive first.
            return next((error for error in errors if not isinstance(error, CommandCancelled)), errors[0])

        def abort_report(error):
            try:
                self.write_report()
            except OSError as report_error:
                # A full disk may also prevent saving partial diagnostics.
                # Preserve the original failing stage and its log location.
                error.add_note(f"partial report could not be saved: {report_error}")

        def worker(target, finished):
            try:
                while True:
                    self.control.check()
                    try:
                        suite, ids = tasks.get_nowait()
                    except queue.Empty:
                        return
                    run_group(suite, ids, target)
            except BaseException as error:
                with error_lock:
                    errors.append(error)
                self.control.cancel()
            finally:
                finished.set()

        finished = [threading.Event() for _ in targets]
        threads = [threading.Thread(target=worker, args=(target, finished[index]), name=f"upstream-{index}")
                   for index, target in enumerate(targets)]
        started = []

        def join_workers():
            for thread, done in started:
                if thread.ident is None:
                    continue
                # Wait on our own completion marker before join. Interrupted
                # Thread.join can mark a still-running thread stopped on some
                # Python versions; an Event does not lose that tracking.
                while not done.is_set():
                    try:
                        done.wait(timeout=0.2)
                    except KeyboardInterrupt:
                        self.control.cancel()
                thread.join()

        try:
            for thread, done in zip(threads, finished):
                self.control.check()
                started.append((thread, done))
                thread.start()
            for _, done in started:
                while not done.wait(timeout=0.2):
                    pass
            join_workers()
        except BaseException as error:
            self.control.cancel()
            join_workers()
            # Keep partial diagnostics, but never reach baseline promotion.
            if isinstance(error, CommandCancelled) and errors:
                cause = original_error()
                abort_report(cause)
                raise cause
            abort_report(error)
            raise
        if errors:
            cause = original_error()
            abort_report(cause)
            raise cause
        self.control.check()

    def select_cases(self):
        """Choose work by ID only; promotion always reruns previous successes."""
        if self.args.command == "check":
            selected = set(self.base["passed"])
        elif self.args.select is not None:
            candidates = json.loads(self.args.select.read_text())
            if not isinstance(candidates, list) or not candidates:
                raise RuntimeError("--select must contain a nonempty JSON array of case IDs")
            if any(type(case_id) is not str or not case_id.strip() for case_id in candidates):
                raise RuntimeError("--select contains an invalid case ID")
            selected = set(candidates)
            if len(selected) != len(candidates):
                raise RuntimeError("--select contains duplicate case IDs")
        else:
            selected = {key for key in self.cases if self.args.filter is None or self.args.filter in key}
        # Reject an empty candidate request even if an existing baseline would
        # otherwise add work. Typos must not look like a successful promotion.
        if not selected:
            raise RuntimeError("selection matched no upstream tests")
        if self.args.command == "promote" and self.base is not None:
            selected.update(self.base["passed"])
        unknown = selected - self.cases.keys()
        if unknown:
            raise RuntimeError("selection contains unknown or disappeared case IDs: " + ", ".join(sorted(unknown)))
        return selected

    def require_selected_results(self, selected):
        """Every selected, nonignored case must reach an actual terminal result."""
        completed = {"passed", "failed", "blocked", "timeout", "native_failed", "native_timeout"}
        completed.update(stage + suffix for stage in ("export", "emit", "go-build")
                         for suffix in ("_failed", "_timeout"))
        missing = [case_id for case_id in sorted(selected)
                   if not self.cases[case_id]["ignored"] and self.results.get(case_id) not in completed]
        if missing:
            raise RuntimeError("unfinished selected cases: " + ", ".join(missing))

    def run(self):
        if self.args.command == "check" and self.base is None:
            raise RuntimeError(f"no recorded baseline for linux/{self.arch}: {self.basefile}")
        if self.args.command in ("check", "promote") and self.base is not None and self.base.get("context") != self.context:
            raise RuntimeError("baseline context differs; compiler/source/dependency changes require an explicit reviewed baseline")
        self.prepare()
        self.discover()
        if self.args.command == "list":
            for case_id in sorted(self.cases):
                print(case_id + (" [ignored]" if self.cases[case_id]["ignored"] else ""))
            return
        selected = self.select_cases()
        self.command(["go", "build", "-o", self.cache / "oxide", "./cmd/oxide"], cwd=ROOT / "oxide-go",
                env=self.env, log=self.work / "oxide-build.log", stage="oxide-build", timeout=self.args.build_timeout)
        self.run_batches(selected)
        self.control.check()
        self.write_report()
        self.require_selected_results(selected)
        if self.base is not None and self.args.command in ("check", "promote"):
            errors = validate_baseline(self.base, self.context, sorted(self.cases), self.results)
            if errors:
                raise RuntimeError("regression gate failed:\n" + "\n".join(errors))
        if self.args.command == "promote":
            updated = promote_baseline(self.base, self.context, sorted(self.cases), self.results)
            save(self.basefile, updated)
            print(f"Saved {len(updated['passed'])} passing tests to {self.basefile}")
        print(json.dumps(dict(Counter(self.results.values())), sort_keys=True))
        print(f"Report: {self.report_path}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("check", "scan", "promote", "list"))
    selection = parser.add_mutually_exclusive_group()
    selection.add_argument("--filter", help="scan/promote: substring of suite/test name")
    selection.add_argument("--select", type=Path,
                           help="scan/promote: JSON array of exact case IDs to rerun (not prior results)")
    parser.add_argument("--baseline", type=Path, help="override the target-specific baseline path")
    parser.add_argument("--report", type=Path, help="write structured results here")
    parser.add_argument("--batch-size", type=int, default=128)
    parser.add_argument("--jobs", type=int, default=1, choices=range(1, 5),
                        help="parallel batch workers, each with its own Cargo cache (default: 1)")
    parser.add_argument("--timeout", type=int, default=30, help="per-test execution timeout, seconds")
    parser.add_argument("--build-timeout", type=int, default=600)
    parser.add_argument("--keep-generated", action="store_true")
    args = parser.parse_args()
    if (args.filter is not None or args.select is not None) and args.command not in ("scan", "promote"):
        parser.error("--filter/--select are only allowed for scan/promote; check always reruns all baseline cases")
    if min(args.batch_size, args.timeout, args.build_timeout) <= 0:
        parser.error("batch size and timeouts must be positive")
    try:
        if not os.environ.get("OXIDE_FRONTEND"):
            # Rebuild from the current checkout: an old bin/oxide-rs must not
            # make a frontend source regression appear to pass.
            build_env = os.environ.copy()
            for key in ("RUSTC_WRAPPER", "RUSTC_WORKSPACE_WRAPPER", "CARGO_ENCODED_RUSTFLAGS", "OXIDE_EXPORT", "OXIDE_ROOTS"):
                build_env.pop(key, None)
            subprocess.run(["sh", ROOT / "oxide-rs/build.sh"], cwd=ROOT, env=build_env, check=True)
        runner = Runner(args)

        def interrupt(_signum, _frame):
            raise KeyboardInterrupt

        previous_term = signal.signal(signal.SIGTERM, interrupt)
        try:
            runner.run()
        finally:
            runner.control.cancel()
            signal.signal(signal.SIGTERM, previous_term)
    except KeyboardInterrupt:
        print("upstream: interrupted", file=sys.stderr)
        return 130
    except (RuntimeError, ValueError, OSError, subprocess.CalledProcessError) as error:
        print(f"upstream: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
