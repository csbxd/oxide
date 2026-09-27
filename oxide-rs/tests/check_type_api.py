#!/usr/bin/env python3
"""Exercise compiler-derived public type/container metadata without Go or renderer glue."""

import argparse
import json
import os
from pathlib import Path
import platform
import subprocess

root = Path(__file__).resolve().parents[2]
cache = root / ".cache/type-api"
cache.mkdir(parents=True, exist_ok=True)
sysroot = root / ".cache/rust-2026-09-15"
parser = argparse.ArgumentParser()
parser.add_argument("--target", action="append")
args = parser.parse_args()
targets = args.target or ["aarch64-unknown-linux-gnu", "x86_64-unknown-linux-gnu"]
env = dict(os.environ, RUSTC=str(sysroot / "bin/rustc"), RUSTC_WRAPPER=str(root / "bin/oxide-rs"),
           RUSTC_WORKSPACE_WRAPPER="", OXIDE_ROOTS="", OXIDE_EXPORT="", RUSTFLAGS="",
           CARGO_ENCODED_RUSTFLAGS="\x1f".join(["-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes"]),
           CARGO_TARGET_DIR=str(root / ".cache/frontend-panic/target"))
for target in targets:
    output = cache / (target + ".json")
    pending = cache / f"{target}-{os.getpid()}.json"
    subprocess.run([str(sysroot / "bin/cargo"), "rustc", "-Zbuild-std=std,panic_unwind", "--manifest-path",
                    str(Path(__file__).with_name("type_api") / "Cargo.toml"), "--lib", "--target", target,
                    "--", "--oxide-export=" + str(pending)], env=env, check=True)
    pending.replace(output)
    pending.with_suffix(".api.json").replace(output.with_suffix(".api.json"))
    p = json.loads(output.with_suffix(".api.json").read_text())
    assert any(item["name"] == "oxide_type_api::spoof::Path" and item["status"] == "unsupported_unsized_value" and not item["selected"] for item in p["public_api"])
    assert not any(item["name"] == "oxide_type_api::spoof::Path" for item in p["roots"])
    types = {ty["id"]: ty for ty in p["api_types"]}
    aliases = {alias["name"]: types[alias["type"]] for alias in p["public_types"]}
    document = aliases["oxide_type_api::Document"]
    fields = {field["name"]: field for field in document["members"][0]}
    assert [field["public"] for field in document["members"][0]] == [True, True, True, False]
    assert fields["title"]["type"] in types and fields["nodes"]["type"] in types
    assert not any("hidden::Secret" in ty["name"] for ty in types.values())
    assert aliases["oxide_type_api::Word"]["kind"] == "u64"
    assert aliases["oxide_type_api::Bytes"]["kind"] == "slice"
    assert aliases["oxide_type_api::Aligned"]["align"] == 64
    assert aliases["oxide_type_api::Packed"]["members"][0][1]["offset"] == 1
    tail = aliases["oxide_type_api::PrivateTail"]["members"][0][1]
    assert not tail["public"] and types[tail["type"]]["kind"] == "slice"
    for name in ("String", "Vec", "Path"):
        assert "container" not in aliases["oxide_type_api::spoof::" + name]
    kinds = {ty.get("container", {}).get("kind") for ty in types.values()}
    assert {"string", "vec", "result", "option", "str_ref", "slice_ref", "path_ref", "os_str_ref", "path_buf", "os_string", "box"} <= kinds, kinds
    for ty in types.values():
        assert ty["canonical"] in types, ty
        assert not ty["name"].startswith("type#"), ty
        if ty["needs_drop"] and ty["sized"]:
            assert ty.get("drop_symbol"), ty
    assert any(ty.get("container", {}).get("zst") for ty in types.values())
    assert document.get("default_symbol")
    string = types[fields["title"]["type"]]
    assert string["display_symbol"] and string["default_symbol"]
    event = aliases["oxide_type_api::Event"]
    debug = event["debug"]
    assert debug["value_type"] == event["canonical"] and not debug["by_reference"]
    assert debug["template"] == [192, 0] and debug["string_type"] == string["canonical"]
    assert any(ty.get("debug", {}).get("by_reference") for ty in types.values() if not ty["sized"])
    program = json.loads(output.read_text())
    functions = {function["symbol"]: function for function in program["functions"]}
    assert p["process_exit_symbol"] == program["process_exit_symbol"]
    assert functions[p["process_exit_symbol"]]["name"] == "std::process::exit"
    assert functions[p["process_exit_symbol"]].get("body")
    layouts = {ty["id"]: ty for ty in program["types"]}
    roots = {root["name"]: root for root in program["roots"]}
    for name, expected, pointer_kind, mutable in (("dyn_ref", "vtable", "ref", False), ("dyn_mut", "vtable", "ref", True), ("dst_ref", "length", "ref", False), ("dst_raw", "length", "raw", True)):
        pointer = types[roots["oxide_type_api::" + name]["return"]]
        assert pointer["pointer_kind"] == pointer_kind and pointer["mutable"] == mutable
        assert pointer["container"]["kind"] == "reference"
        assert pointer["container"]["metadata"] == expected
        assert pointer["container"]["meta_offset"] == pointer["container"]["len_offset"] == pointer["abi_pair"]["b_offset"]
    calls = set(functions[roots["oxide_type_api::debug_event"]["symbol"]]["calls"].values())
    for key in ("argument_symbol", "arguments_symbol", "format_symbol"):
        assert debug[key] in calls, (key, debug[key], calls)
        assert functions[debug[key]].get("body"), (key, debug[key])
    assert document["default_symbol"] == roots["<oxide_type_api::Document as core::default::Default>::default"]["symbol"]
    maps = aliases["oxide_type_api::MapOwner"]
    for member in maps["members"][0]:
        map_type = types[member["type"]]
        assert map_type["container"]["kind"] == {"hash": "hash_map", "tree": "btree_map"}[member["name"]]
        assert "element" not in map_type["container"]
        for field, suffix in (("iteration", ""), ("mutable_iteration", "_mut")):
            iteration = map_type[field]
            assert iteration["iterator_type"] in types and iteration["item_type"] in types
            assert types[iteration["item_type"]]["container"]["kind"] == "option"
            call_root = roots[f"oxide_type_api::next_{member['name']}{suffix}"]
            calls = set(functions[call_root["symbol"]]["calls"].values())
            assert iteration["symbol"] in calls and iteration["next_symbol"] in calls, (iteration, calls)
            assert call_root["return"] == iteration["item_type"]
    if target.startswith({"aarch64": "aarch64", "x86_64": "x86_64"}[platform.machine()]):
        vector = types[fields["nodes"]["type"]]
        offsets = [string["container"][key] for key in ("data_offset", "len_offset", "capacity_offset")]
        offsets += [vector["container"][key] for key in ("data_offset", "len_offset", "capacity_offset")]
        offsets.append(layouts[debug["arguments_type"]]["fields"][0])
        native = cache / "native"
        subprocess.run([str(sysroot / "bin/rustc"), "--edition=2024", "--cfg", "oxide_native",
                        "-Awarnings", str(Path(__file__).with_name("type_api.rs")), "-o", str(native)], check=True)
        actual = list(map(int, subprocess.check_output([str(native), *map(str, offsets)], text=True).split()))
        assert actual == [document["size"], document["align"], *[field["offset"] for field in document["members"][0]]], (actual, document)
    print(f"PASS {target}: {len(types)} API types, {len(aliases)} public names, {len(p['public_drop_types'])} drops")
