#!/usr/bin/env python3
"""Run native Rust -> MIR -> Go SVG/PNG differential tests.

Run from any directory after oxide-rs/build.sh:
    python3 oxide/fixtures/renderer/test.py

The persistent cache retains reference images, MIR, generated Go and actual
images when a compiler/runtime stage fails. Use --stage go to retry lowering
and comparison without rebuilding Rust. Default dependency features stay on.
"""

import argparse
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile


def run(args, **kwargs):
    print("+", " ".join(str(arg) for arg in args), flush=True)
    subprocess.run([str(arg) for arg in args], check=True, **kwargs)


def main():
    fixture = Path(__file__).resolve().parent
    root = fixture.parents[1]
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--stage", choices=("all", "native", "export", "go"), default="all")
    parser.add_argument("--chaos", action="store_true", help="run randomized ownership/leak checks with memory.counters")
    parser.add_argument("--cache", type=Path, default=root / ".cache" / "renderer-conformance")
    parser.add_argument("--frontend", type=Path, default=Path(os.environ.get("OXIDE_FRONTEND", root / "bin" / "oxide-rs")))
    args = parser.parse_args()
    cache = args.cache.resolve()
    cache.mkdir(parents=True, exist_ok=True)
    machine = platform.machine()
    if platform.system() != "Linux" or machine not in ("aarch64", "x86_64"):
        parser.error("renderer differential tests support only linux/arm64 and linux/amd64")
    arch = "arm64" if machine == "aarch64" else "amd64"
    triple = "aarch64-unknown-linux-gnu" if arch == "arm64" else "x86_64-unknown-linux-gnu"
    frontend = args.frontend.resolve()
    sysroot = Path(subprocess.check_output([str(frontend), "--print-sysroot"], text=True).strip())
    manifest = fixture / "Cargo.toml"
    target = cache / "cargo-target"
    reference = cache / "reference"
    mir_path = cache / "oxide.mir.json"
    output = cache / "go"
    env = dict(os.environ)
    env.update({
        "RUSTC": str(sysroot / "bin" / "rustc"),
        "RUSTC_WRAPPER": str(frontend),
        "RUSTC_WORKSPACE_WRAPPER": "",
        "RUSTC_BOOTSTRAP": "1",
        "OXIDE_EXPORT": "",
        "OXIDE_ROOTS": "oxide_renderer_fixture::render_svg,oxide_renderer_fixture::write_png",
        "RUSTFLAGS": "",
        "CARGO_ENCODED_RUSTFLAGS": "\x1f".join(("-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes")),
        "CARGO_TARGET_DIR": str(target),
    })
    cargo_args = ["-Zbuild-std=std,panic_unwind", "--locked", "--manifest-path", manifest, "--target", triple]
    if args.stage in ("all", "native"):
        run([sysroot / "bin" / "cargo", "build", *cargo_args, "--bin", "oxide-renderer-native"], env=env)
        run([target / triple / "debug" / "oxide-renderer-native", fixture / "cases", reference], env=env)
    if args.stage in ("all", "export"):
        # Cargo fingerprints the unique final argument, so every requested
        # export runs even when Cargo can reuse all dependency artifacts.
        with tempfile.TemporaryDirectory(prefix="export-", dir=cache) as stage:
            export = Path(stage) / "oxide.mir.json"
            run([sysroot / "bin" / "cargo", "rustc", *cargo_args, "--lib", "--", "--oxide-export=" + str(export)], env=env)
            export.replace(mir_path)
    if args.stage in ("all", "go"):
        output.mkdir(parents=True, exist_ok=True)
        goenv = dict(os.environ, GOOS="linux", GOARCH=arch, CGO_ENABLED="0", GOWORK="off")
        # A complete renderer package produces large compiler objects. Keep
        # both caches on the workspace filesystem instead of a small /tmp
        # tmpfs, and reuse them on subsequent acceptance runs.
        for variable, directory in (("GOTMPDIR", cache / "go-tmp"), ("GOCACHE", cache / "go-cache")):
            directory.mkdir(parents=True, exist_ok=True)
            goenv[variable] = str(directory)
        run(["go", "run", "./cmd/oxide", "emit", "-mir", mir_path, "-out", output, "-package", "rendererfixture"], cwd=root / "oxide-go", env=goenv)
        module = root / "oxide-go"
        (output / "go.mod").write_text("module oxide-renderer-conformance\n\ngo 1.27.1\n\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => " + json.dumps(str(module)) + "\n")
        shutil.copyfile(fixture / "generated_test.go", output / "oxide_gen_test.go")
        shutil.copyfile(fixture / "chaos_test.go", output / "chaos_test.go")
        for source, destination in ((fixture / "cases", output / "cases"), (reference, output / "reference")):
            if destination.exists():
                shutil.rmtree(destination)
            shutil.copytree(source, destination)
        tests = ["go", "test", "-mod=mod", "-count=1", "-timeout=20m", "-v"]
        if args.chaos:
            tests += ["-tags=memory.counters", "-run", "^TestRendererOwnershipChaos$"]
        run([*tests, "."], cwd=output, env=goenv)


if __name__ == "__main__":
    main()
