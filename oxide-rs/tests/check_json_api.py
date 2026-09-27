#!/usr/bin/env python3
"""Verify serde_json bindings use real instances, not project instantiation wrappers."""
import argparse
import json
import os
from pathlib import Path
import platform
import subprocess
import tempfile

root = Path(__file__).resolve().parents[2]
cache = root / ".cache/json-api"
cache.mkdir(parents=True, exist_ok=True)
sysroot = root / ".cache/rust-2026-09-15"
manifest = Path(__file__).with_name("json_api") / "Cargo.toml"
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--target", action="append")
args = parser.parse_args()
targets = args.target or ["aarch64-unknown-linux-gnu", "x86_64-unknown-linux-gnu"]
env = dict(os.environ, RUSTC=str(sysroot / "bin/rustc"), RUSTC_WRAPPER=str(root / "bin/oxide-rs"),
           RUSTC_WORKSPACE_WRAPPER="", OXIDE_ROOTS="oxide_json_api::inspect", OXIDE_EXPORT="", RUSTFLAGS="",
           CARGO_ENCODED_RUSTFLAGS="\x1f".join(["-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes"]),
           CARGO_TARGET_DIR=str(root / ".cache/frontend-panic/target"))

def run(argv, **kw):
    print("+", " ".join(map(str, argv)), flush=True)
    return subprocess.run(list(map(str, argv)), env=env, check=True, **kw)

def check(program, enabled):
    api = {ty["id"]: ty for ty in program["api_types"]}
    public = {t["name"].rsplit("::", 1)[-1]: api[t["type"]] for t in program["public_types"]}
    functions = {f["symbol"]: f for f in program["functions"]}
    layouts = {t["id"]: t for t in program["types"]}
    def canonical(ty):
        return api[ty["canonical"]]
    if not enabled:
        assert all("json" not in ty for ty in api.values()), "unrelated local serde_json module acquired bindings"
        return {"types": len(api), "json_types": 0}
    for name, encode, decode in [("Packet", True, True), ("EncodeOnly", True, False), ("FloatOrder", True, False),
                                 ("DecodeOnly", False, True), ("Neither", False, False), ("Borrowed", True, True)]:
        ty = canonical(public[name])
        binding = ty.get("json", {})
        assert bool(binding.get("serialize_symbol")) == encode, (name, binding)
        assert bool(binding.get("value_symbol")) == encode, (name, binding)
        assert bool(binding.get("deserialize_symbol")) == decode, (name, binding)
    extra = next(f for f in canonical(public["Packet"])["members"][0] if f["name"] == "extra")
    assert canonical(api[extra["type"]])["json"].get("deserialize_symbol")
    checks = 0
    for ty in api.values():
        for operation in ("serialize", "deserialize", "value"):
            binding = ty.get("json", {})
            symbol = binding.get(operation + "_symbol")
            if not symbol:
                continue
            checks += 1
            function = functions[symbol]
            assert function["name"].startswith("serde_json::") and function["body"], function["name"]
            body = function["body"]
            assert body["arg_count"] == 1
            argument = layouts[body["locals"][1]["ty"]]
            result = layouts[body["locals"][0]["ty"]]
            assert result["id"] == binding[operation + "_result_type"] and result["id"] in api
            assert result["adt_kind"] == "Enum" and result["variant_names"] == ["Ok", "Err"]
            if operation in ("serialize", "value"):
                assert canonical(api[argument["pointee"]])["id"] == ty["canonical"]
                if operation == "value":
                    returned = canonical(api[result["variant_field_types"][0][0]])
                    assert returned["definition"] == "serde_json::value::Value", returned
                    assert function["name"].startswith("serde_json::to_value"), function["name"]
            else:
                assert layouts[argument["pointee"]]["kind"] == "str"
                returned = result["variant_field_types"][0][0]
                assert canonical(api[returned])["id"] == ty["canonical"]
            assert api[result["id"]].get("drop_symbol") or not api[result["id"]]["needs_drop"]
    # The sole Rust root contains no call to a JSON entry point. All generic
    # functions were selected by the compiler metadata planner itself.
    roots = program["roots"]
    assert len(roots) == 1 and roots[0]["name"] == "oxide_json_api::inspect"
    calls = functions[roots[0]["symbol"]]["calls"].values()
    assert not any(functions.get(symbol, {}).get("name", "").startswith("serde_json::") for symbol in calls)
    assert not any(f["name"] == "oxide_json_api::main" for f in functions.values())
    return {"types": len(api), "json_types": sum("json" in ty for ty in api.values()), "operations": checks}

summary = {}
for target in targets:
    for enabled in (True, False):
        label = target + ("" if enabled else "-no-json")
        with tempfile.TemporaryDirectory(prefix="export-", dir=cache) as temporary:
            pending = Path(temporary) / "oxide.mir.json"
            flags = [] if enabled else ["--no-default-features"]
            run([sysroot / "bin/cargo", "rustc", "-Zbuild-std=std,panic_unwind", "--manifest-path", manifest,
                 "--target", target, "--lib", *flags, "--", "--oxide-export=" + str(pending)])
            output = cache / (label + ".json")
            pending.replace(output)
            pending.with_suffix(".api.json").replace(output.with_suffix(".api.json"))
        program = json.loads(output.read_text())
        summary[label] = check(program, enabled)
        del program
        print("PASS", label, summary[label], flush=True)
    machine = {"aarch64": "aarch64-unknown-linux-gnu", "x86_64": "x86_64-unknown-linux-gnu"}[platform.machine()]
    if target == machine:
        run([sysroot / "bin/cargo", "rustc", "-Zbuild-std=std,panic_unwind", "--manifest-path", manifest,
             "--target", target, "--bin", "oxide-json-api-native", "--", "--cfg", "oxide_native"])
        binary = Path(env["CARGO_TARGET_DIR"]) / target / "debug/oxide-json-api-native"
        result = subprocess.check_output([str(binary)])
        (cache / "native.stdout").write_bytes(result)
        assert len(result.splitlines()) == 6
        direct, through_value = result.splitlines()[-1].split(b"|")
        assert direct == b'{"z":0.1,"a":7}'
        assert through_value == b'{"a":7,"z":0.10000000149011612}'
summary["native_reference"] = "native.stdout (host target, compiled without the library export cfg)"
(cache / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
