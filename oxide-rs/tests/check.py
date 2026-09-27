#!/usr/bin/env python3
"""Check the compiler metadata contract on both supported Linux targets."""

import json
from pathlib import Path
import platform
import subprocess
import tempfile


root = Path(__file__).resolve().parents[2]
frontend = root / "bin/oxide-rs"
source = Path(__file__).with_name("layout.rs")
for target in ("aarch64-unknown-linux-gnu", "x86_64-unknown-linux-gnu"):
    with tempfile.TemporaryDirectory(prefix="oxide-layout-") as directory:
        directory = Path(directory)
        output = directory / "mir.json"
        subprocess.run(
            [
                str(frontend), "--export", str(output), "--",
                str(source), "--crate-name", "layout", "--crate-type", "lib",
                "--emit=metadata", "--target", target,
                "-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=yes",
                "-o", str(directory / "layout.rmeta"),
            ],
            check=True,
        )
        program = json.loads(output.read_text())
        types = {value["id"]: value for value in program["types"]}
        assert program["target"] == target
        assert program["panic_strategy"] == "Unwind"
        assert program["runtime_checks"] == {"UbChecks": True, "ContractChecks": False, "OverflowChecks": True}
        assert len(program["roots"]) == 16, program["roots"]
        assert any(api["name"] == "layout::Value::value" and api["status"] == "requires_trait_resolution" for api in program["public_api"])
        assert all("layout_error" not in value for value in types.values())
        assert all(value["kind"] != "unsupported" for value in types.values())
        for value in types.values():
            if value["kind"] in ("fn", "dynamic"):
                continue
            assert value["value_abi"] in ("Scalar", "ScalarPair", "Vector", "Aggregate", "ScalableVector"), value
            assert ("abi_scalar" in value) == (value["value_abi"] == "Scalar"), value
            assert ("abi_pair" in value) == (value["value_abi"] == "ScalarPair"), value
            if "abi_pair" in value:
                assert 0 < value["abi_pair"]["b_offset"] < value["size"], value
        assert any(value["kind"] == "aggregate" and value.get("abi_scalar") == {"Pointer": 0} for value in types.values()), "missing compiler scalar ABI for pointer newtype"
        signatures = [value for value in types.values() if value["kind"] == "fnptr"]
        assert signatures, "missing higher-ranked function pointer"
        for signature in signatures:
            assert "fn_inputs" in signature and "fn_output" in signature, signature
            assert signature["size"] == 8 and signature["align"] == 8, signature
            assert signature["fn_output"] in types, signature
            assert all(value in types for value in signature["fn_inputs"]), signature
        assert any(
            types[signature["fn_inputs"][0]]["size"] == 16
            for signature in signatures if signature["fn_inputs"]
        ), "missing dyn Write fat pointer argument"
        tails = [
            value for value in types.values()
            if not value.get("sized", True) and value["kind"] == "aggregate"
        ]
        assert tails, "missing struct DST layout"
        assert all(value["fields"] == [0, 8] for value in tails), tails
        assert len(program["vtables"]) >= 2, program["vtables"]
        assert any(value["empty_drop"] for value in program["functions"])
        assert any(
            value["name"] == "layout::tracked" and value["track_caller"]
            for value in program["functions"]
        ), "missing track_caller metadata"
        reified = 0
        for function in program["functions"]:
            for block in function.get("body", {}).get("blocks", []):
                for statement in block["statements"]:
                    kind = statement["kind"]
                    if not isinstance(kind, dict) or "Assign" not in kind:
                        continue
                    value = kind["Assign"][1]
                    if "Cast" not in value:
                        continue
                    cast, operand, _ = value["Cast"]
                    coercion = cast.get("PointerCoercion") if isinstance(cast, dict) else None
                    if not isinstance(coercion, dict) or "ReifyFnPointer" not in coercion:
                        continue
                    reified += 1
                    ty = types[operand["Constant"]["const_"]["ty"]]
                    assert ty.get("function"), ty
                    reified_function = next(value for value in program["functions"] if value["symbol"] == ty["function"])
                    assert not reified_function["track_caller"], reified_function
        assert reified, "missing dyn argument function reification"
        functions = {value["symbol"]: value for value in program["functions"]}
        assert len(functions) == len(program["functions"]), "duplicate function symbols"
        assertions = 0
        for function in functions.values():
            for block, call in function.get("assert_calls", {}).items():
                assertions += 1
                callee = functions[call["symbol"]]
                assert callee["track_caller"], callee
                assert block in function["call_locations"], function
                assert isinstance(call["optional"], bool), call
                count = callee["body"]["arg_count"] if "body" in callee else len(callee["signature"]["params"])
                assert len(call["args"]) == count, call
        assert assertions, "missing compiler assertion panic calls"
        rust_calls = 0
        for function in functions.values():
            for block, argument in function.get("call_untuple", {}).items():
                call = function["body"]["blocks"][int(block)]["terminator"]["kind"]["Call"]
                assert argument == len(call["args"]) - 1, call
                rust_calls += 1
        assert rust_calls, "missing compiler RustCall tuple adaptation"
        assert "strlen" in functions, "missing shared external declaration"
        external = {value["external"]["symbol"]: value["external"] for value in program["allocations"] if "external" in value}
        weak = external["oxide_missing_weak_probe"]
        assert weak["weak"] and weak["linkage"] == "ExternalWeak" and weak["kind"] == "function", weak
        assert types[weak["function_type"]]["kind"] == "fnptr", weak
        strong = external["oxide_required_probe"]
        assert not strong["weak"] and strong["kind"] == "static", strong
        variadic = functions["oxide_variadic_probe"]["signature"]
        assert variadic["variadic"] and variadic["fixed_count"] == 1 and variadic["abi"] == "C", variadic
        assert not variadic["can_unwind"], variadic
        locations = [location for function in functions.values() for location in function.get("call_locations", {}).values()]
        assert any(location.get("inherited") for location in locations), "missing inherited caller location"
        allocated = {value["id"] for value in program["allocations"]}
        assert any("allocation" in location for location in locations), "missing static caller location"
        assert all(location.get("inherited") or location["allocation"] in allocated for location in locations)
        type_id_addresses = {value["id"] for value in program["allocations"] if value.get("address") == 0}
        assert type_id_addresses, "missing TypeId zero base address"
        if target.startswith(platform.machine()):
            native = directory / "typeid.rs"
            native.write_text('fn main() { let bytes: [u8; 16] = unsafe { std::mem::transmute(std::any::TypeId::of::<u32>()) }; println!("{:?}", bytes); }')
            sysroot = subprocess.check_output([str(frontend), "--print-sysroot"], text=True).strip()
            subprocess.run([str(Path(sysroot) / "bin/rustc"), str(native), "-o", str(directory / "typeid")], check=True)
            native_bytes = json.loads(subprocess.check_output([str(directory / "typeid")], text=True))
            pending = [program]
            found_type_id = False
            while pending:
                value = pending.pop()
                if isinstance(value, list):
                    pending.extend(value)
                elif isinstance(value, dict):
                    if any(pair[1] in type_id_addresses for pair in value.get("provenance", {}).get("ptrs", [])):
                        assert value["bytes"] == native_bytes, "TypeId hash differs from native rustc"
                        found_type_id = True
                    pending.extend(value.values())
            assert found_type_id, "missing TypeId relocation"
        external_calls = 0
        for function in functions.values():
            for index, block in enumerate(function.get("body", {}).get("blocks", [])):
                callee = functions.get(function.get("calls", {}).get(str(index)))
                if callee is None or "signature" not in callee:
                    continue
                terminator = block["terminator"]["kind"]
                if "Call" not in terminator:
                    continue
                external_calls += 1
                if callee["signature"].get("variadic"):
                    assert len(terminator["Call"]["args"]) >= callee["signature"]["fixed_count"], callee
                else:
                    assert len(terminator["Call"]["args"]) == len(callee["signature"]["params"]), callee
        assert external_calls, "missing external MIR signature check"
        unchecked = directory / "unchecked.json"
        subprocess.run(
            [str(frontend), "--export", str(unchecked), "--", str(source),
             "--crate-name", "layout", "--crate-type", "lib", "--emit=metadata",
             "--target", target, "-Zmir-opt-level=0", "-Coverflow-checks=no",
             "-Zub-checks=no", "-Zcontract-checks=yes", "-o", str(directory / "unchecked.rmeta")],
            check=True,
        )
        assert json.loads(unchecked.read_text())["runtime_checks"] == {
            "UbChecks": False, "ContractChecks": True, "OverflowChecks": False,
        }
        print(f"{target}: higher-ranked signatures, vtables and struct DST layouts passed")
