#!/usr/bin/env python3
"""Run native Rust -> MIR -> Go library API and SVG/PNG differential tests.

Run from any directory after oxide-rs/build.sh:
    python3 oxide/fixtures/renderer/test.py

The persistent cache retains reference images, MIR, generated Go and actual
images when a compiler/runtime stage fails. Use --stage go to retry lowering
and comparison without rebuilding Rust. Default dependency features stay on;
scene is enabled so the complete library API is available.
"""

import argparse
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile

from check_api import check_api
from check_dependencies import check_dependencies


def run(args, **kwargs):
    print("+", " ".join(str(arg) for arg in args), flush=True)
    subprocess.run([str(arg) for arg in args], check=True, **kwargs)


def main():
    fixture = Path(__file__).resolve().parent
    root = fixture.parents[1]
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--stage", choices=("all", "native", "export", "go"), default="all")
    parser.add_argument("--chaos", action="store_true", help="run randomized ownership/leak checks with memory.counters")
    parser.add_argument("--cache", type=Path, default=None)
    parser.add_argument("--frontend", type=Path, default=Path(os.environ.get("OXIDE_FRONTEND", root / "bin" / "oxide-rs")))
    args = parser.parse_args()
    machine = platform.machine()
    if platform.system() != "Linux" or machine not in ("aarch64", "x86_64"):
        parser.error("renderer differential tests support only linux/arm64 and linux/amd64")
    arch = "arm64" if machine == "aarch64" else "amd64"
    triple = "aarch64-unknown-linux-gnu" if arch == "arm64" else "x86_64-unknown-linux-gnu"
    cache = (args.cache or root / ".cache" / "renderer-direct" / arch).resolve()
    cache.mkdir(parents=True, exist_ok=True)
    frontend = args.frontend.resolve()
    sysroot = Path(subprocess.check_output([str(frontend), "--print-sysroot"], text=True).strip())
    manifest = fixture / "Cargo.toml"
    upstream = root.parent / "mermaid-rs-renderer" / "Cargo.toml"
    target = cache / "cargo-target"
    reference = cache / "reference"
    mir_path = cache / "oxide.mir.json"
    output = cache / "go"
    scratch = cache / "tmp"
    scratch.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ)
    env.update({
        "TMPDIR": str(scratch),
        "RUSTC": str(sysroot / "bin" / "rustc"),
        "RUSTC_WRAPPER": str(frontend),
        "RUSTC_WORKSPACE_WRAPPER": "",
        "RUSTC_BOOTSTRAP": "1",
        "OXIDE_EXPORT": "",
        # Export the upstream library itself; no native oracle is a dependency.
        "OXIDE_ROOTS": "",
        "RUSTFLAGS": "",
        "CARGO_ENCODED_RUSTFLAGS": "\x1f".join(("-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes")),
        "CARGO_TARGET_DIR": str(target),
    })
    cargo_args = ["-Zbuild-std=std,panic_unwind", "--locked", "--manifest-path", manifest, "--target", triple]
    closure = check_dependencies(sysroot / "bin" / "cargo", manifest, upstream, triple, env)
    (cache / "dependency-closure.json").write_text(json.dumps(closure, indent=2) + "\n")
    if args.stage in ("all", "native"):
        run([sysroot / "bin" / "cargo", "build", *cargo_args, "--bin", "oxide-renderer-native"], env=env)
        run([target / triple / "debug" / "oxide-renderer-native", fixture / "cases", reference], env=env)
        run([sysroot / "bin" / "cargo", "build", *cargo_args, "--bin", "oxide-renderer-api-native", "--bin", "oxide-renderer-cli-native"], env=env)
        run([target / triple / "debug" / "oxide-renderer-api-native", reference / "api"], env=env)
        run(["python3", fixture / "test_cli.py", "--native", target / triple / "debug" / "oxide-renderer-cli-native", "--cache", cache / "cli"], env=env)
    upstream_args = ["-Zbuild-std=std,panic_unwind", "--locked", "--manifest-path", upstream, "--package", "mermaid-rs-renderer", "--features", "scene", "--target", triple]
    if args.stage in ("all", "export"):
        # Cargo fingerprints the unique final argument, so every requested
        # export runs even when Cargo can reuse all dependency artifacts.
        with tempfile.TemporaryDirectory(prefix="export-", dir=cache) as stage:
            export = Path(stage) / "oxide.mir.json"
            run([sysroot / "bin" / "cargo", "rustc", *upstream_args, "--lib", "--", "--oxide-export=" + str(export)], env=env)
            export.replace(mir_path)
            export.with_suffix(".api.json").replace(cache / "oxide.mir.api.json")
    if args.stage in ("all", "go"):
        coverage = check_api(cache / "oxide.mir.api.json", reference / "api" / "cases.json", cli_results_path=cache / "cli" / "native" / "results.json")
        (cache / "api-coverage.json").write_text(json.dumps(coverage, indent=2) + "\n")
        print(json.dumps(coverage, sort_keys=True), flush=True)
        output.mkdir(parents=True, exist_ok=True)
        goenv = dict(os.environ, GOOS="linux", GOARCH=arch, CGO_ENABLED="0", GOWORK="off", TMPDIR=str(scratch))
        # The complete library is a large Go package. Keep compiler/vet peaks
        # bounded without disabling checks; explicit caller settings win.
        for variable, value in (("GOGC", "25"), ("GOMEMLIMIT", "20GiB"), ("GOMAXPROCS", "4")):
            goenv.setdefault(variable, value)
        # A complete renderer package produces large compiler objects. Keep
        # both caches on the workspace filesystem instead of a small /tmp
        # tmpfs, and reuse them on subsequent acceptance runs.
        for variable, directory in (("GOTMPDIR", cache / "go-tmp"), ("GOCACHE", cache / "go-cache")):
            directory.mkdir(parents=True, exist_ok=True)
            goenv[variable] = str(directory)
        run(["go", "run", "-p=1", "./cmd/oxide", "emit", "-mir", mir_path, "-out", output, "-package", "rendererfixture"], cwd=root / "oxide-go", env=goenv)
        module = root / "oxide-go"
        (output / "go.mod").write_text("module oxide-renderer-conformance\n\ngo 1.27.1\n\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => " + json.dumps(str(module)) + "\n")
        shutil.copyfile(fixture / "generated_test.go", output / "oxide_gen_test.go")
        for source in sorted(fixture.glob("api_*_test.go")):
            shutil.copyfile(source, output / source.name)
        shutil.copyfile(fixture / "chaos_test.go", output / "chaos_test.go")
        shutil.copyfile(fixture / "heap_trace_test.go", output / "heap_trace_test.go")
        for name in ("direct_test.go", "owned_return_test.go"):
            shutil.copyfile(fixture / name, output / name)
        for source, destination in ((fixture / "cases", output / "cases"), (reference, output / "reference")):
            if destination.exists():
                shutil.rmtree(destination)
            shutil.copytree(source, destination)
        tests = ["go", "test", "-p=1", "-mod=mod", "-tags=memory.counters", "-count=1", "-timeout=20m", "-v"]
        if args.chaos:
            tests += ["-run", "^Test(RendererOwnershipChaos|PublicOwnedReturns)$"]
        run([*tests, "."], cwd=output, env=goenv)
        if not args.chaos:
            cli = output / "cmd" / "renderer"
            cli.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(fixture / "cli" / "main.go", cli / "main.go")
            binary = output / "renderer-cli"
            run(["go", "build", "-p=1", "-mod=mod", "-tags=memory.counters", "-o", binary, "./cmd/renderer"], cwd=output, env=goenv)
            run(["python3", fixture / "test_cli.py", "--go", binary, "--cache", cache / "cli"], env=goenv)


if __name__ == "__main__":
    main()
