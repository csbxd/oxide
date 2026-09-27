#!/usr/bin/env python3
"""Export the real standard-library panic runtime for both Linux targets."""

import json
import os
from pathlib import Path
import subprocess


root = Path(__file__).resolve().parents[2]
cache = root / ".cache/frontend-panic"
cache.mkdir(parents=True, exist_ok=True)
sysroot = root / ".cache/rust-2026-09-15"
env = dict(os.environ, RUSTC=str(sysroot / "bin/rustc"),
           RUSTC_WRAPPER=str(root / "bin/oxide-rs"), OXIDE_ROOTS="",
           RUSTFLAGS="-Zalways-encode-mir -Zmir-opt-level=0 -Coverflow-checks=yes")
for target in ("aarch64-unknown-linux-gnu", "x86_64-unknown-linux-gnu"):
    output = cache / f"{target}.json"
    # A fresh path in Cargo's final arguments ensures the exporter is run.
    pending = cache / f"{target}-{os.getpid()}.json"
    subprocess.run([
        str(sysroot / "bin/cargo"), "rustc", "-Zbuild-std=std,panic_unwind",
        "--manifest-path", str(Path(__file__).with_name("panic") / "Cargo.toml"),
        "--lib", "--target", target, "--target-dir", str(cache / "target"),
        "--", f"--oxide-export={pending}",
    ], env=env, check=True)
    pending.replace(output)
    program = json.loads(output.read_text())
    assert program["panic_strategy"] == "Unwind"
    functions = {f["name"]: f for f in program["functions"]}
    for name in ("std::panicking::panic_handler", "panic_unwind::__rust_start_panic",
                 "panic_unwind::__rust_panic_cleanup", "panic_unwind::imp::panic",
                 "panic_unwind::imp::cleanup", "std::panicking::catch_unwind::do_call",
                 "std::panicking::catch_unwind::do_catch"):
        assert functions[name].get("body"), f"missing linked Rust implementation: {name}"
    types = {t["id"]: t for t in program["types"]}
    exceptions = [t for t in types.values() if t.get("variant_names") == ["Exception"]]
    assert len(exceptions) == 1, exceptions
    exception = exceptions[0]
    assert (exception["size"], exception["align"], exception["fields"]) == (64, 16, [0, 32, 40]), exception
    fields = exception["variant_field_types"][0]
    assert types[fields[0]]["variant_names"] == ["_Unwind_Exception"]
    assert types[fields[2]]["size"] == 16, "exception must own a Rust Box<dyn Any + Send>"
    raise_function = functions["unwind::libunwind::_Unwind_RaiseException"]
    signature = raise_function["signature"]
    assert signature["abi"] == "C" and signature["can_unwind"] and not signature["variadic"], signature
    assert len(signature["params"]) == 1 and types[signature["params"][0]]["kind"] == "pointer"
    result = types[signature["return"]]
    assert result["size"] == 4 and result["discriminants"] == [str(i) for i in range(10)]
    assert any("take_box" in name and "PanicPayload" in name for name in functions)
    assert any(f.get("assert_calls") for f in functions.values()), "missing checked arithmetic panic entry"
    boundaries = [f for f in functions.values() if f.get("runtime_boundary")]
    assert len(boundaries) == 1, boundaries
    args = boundaries[0]
    assert args["name"] == "std::sys::args::unix::imp::argc_argv" and args["runtime_boundary"] == "std_args"
    assert args.get("body"), "startup boundary must retain the original Rust MIR"
    print(f"{target}: linked panic handler/runtime, catch callbacks, owned Exception layout passed")
