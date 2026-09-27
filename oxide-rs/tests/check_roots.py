#!/usr/bin/env python3
"""Check public root discovery against compiler-resolved multi-crate APIs."""

import json
import os
from pathlib import Path
import subprocess
import tempfile

root = Path(__file__).resolve().parents[2]
frontend = root / "bin/oxide-rs"
sysroot = Path(subprocess.check_output([str(frontend), "--print-sysroot"], text=True).strip())
source = Path(__file__).with_name("api")

for target in ("aarch64-unknown-linux-gnu", "x86_64-unknown-linux-gnu"):
    with tempfile.TemporaryDirectory(prefix="oxide-roots-") as temporary:
        directory = Path(temporary)
        flags = ["--edition=2024", "--crate-type=rlib", "--target", target,
                 "-Zalways-encode-mir", "-Zmir-opt-level=0", "-Coverflow-checks=no", "-Awarnings"]
        dep = directory / "libdependency.rlib"
        subprocess.run([str(sysroot / "bin/rustc"), *flags, str(source / "dependency.rs"),
                        "--crate-name", "dependency", "-o", str(dep)], check=True)
        output = directory / "mir.json"
        facade = directory / "libfacade.rlib"
        command = [str(frontend), "--export", str(output), "--", *flags,
                   str(source / "facade.rs"), "--crate-name", "facade", "--extern", f"dependency={dep}",
                   "-L", f"dependency={directory}", "-o", str(facade)]
        env = {**os.environ, "OXIDE_ROOTS": ""}
        subprocess.run(command, env=env, check=True)
        program = json.loads(output.read_text())
        sidecar = json.loads(output.with_suffix(".api.json").read_text())
        assert sidecar == {key: program[key] for key in ("compiler", "target", "roots", "public_api", "public_drop_types")}
        roots = {value["name"]: value["symbol"] for value in program["roots"]}
        root_types = {value["name"]: value for value in program["roots"]}
        inventory = program["public_api"]
        functions = {value["symbol"]: value for value in program["functions"]}
        types = {value["id"]: value for value in program["types"]}
        drops = {value["type"]: value for value in program["public_drop_types"]}
        assert len(drops) == len(program["public_drop_types"]) == 1
        owned = root_types["facade::consume_drop"]["params"][0]
        assert drops[owned]["name"] == "dependency::DropParameter"
        drop_body = functions[drops[owned]["symbol"]]["body"]
        assert drop_body["arg_count"] == 1
        assert types[drop_body["locals"][1]["ty"]]["pointee"] == owned
        for name in ("facade::borrow_only", "facade::raw_only"):
            for ty in [*root_types[name]["params"], root_types[name]["return"]]:
                assert ty not in drops and types[ty].get("pointee") not in drops
        callable_root = next(value for name, value in root_types.items() if name.startswith("<facade::CallOnce as ") and name.endswith("::call_once"))
        callable_body = functions[callable_root["symbol"]]["body"]
        assert callable_body["spread_arg"] == 2
        assert len(callable_root["params"]) == 3 and callable_root["params"][1] == owned
        for value in root_types.values():
            function = functions[value["symbol"]]
            if body := function.get("body"):
                params = []
                for index in range(1, body["arg_count"] + 1):
                    ty = body["locals"][index]["ty"]
                    params.extend(types[ty]["field_types"] if index == body["spread_arg"] else [ty])
                assert value["params"] == params and value["return"] == body["locals"][0]["ty"]
            else:
                assert value["params"] == function["signature"]["params"]
                assert value["return"] == function["signature"]["return"]
        assert len(roots) == len(program["roots"])
        assert len(functions) == len(program["functions"])
        assert roots["facade::shared"] == roots["facade::alias"]
        assert roots["facade::left::same"] == roots["facade::renamed_module::same"]
        assert roots["facade::left::same"] != roots["facade::right::same"]
        for method in ("new", "value"):
            assert roots[f"facade::Point::{method}"] == roots[f"facade::RenamedPoint::{method}"]
            assert roots[f"facade::Point::{method}"] == roots[f"facade::PointAlias::{method}"]
            assert roots[f"facade::Point::{method}"] == roots[f"facade::nested::Point::{method}"]
        for name in roots:
            assert "not_exported" not in name and "crate_only" not in name
            assert "::private" not in name and "Unreachable" not in name
        assert "facade::expanded" in roots
        assert "facade::r#type" in roots
        assert "facade::Byte::specialized" in roots
        assert "facade::Generic::specialized" not in roots
        assert roots["<facade::Point as core::clone::Clone>::clone"] == roots["<facade::RenamedPoint as core::clone::Clone>::clone"]
        assert any(api["name"] == "<facade::Point as core::clone::Clone>::clone" and api["trait_definition"] == "core::clone::Clone" for api in inventory)
        assert "<facade::Point as core::clone::Clone>::clone_from" in roots
        assert "<facade::Point as core::default::Default>::default" in roots
        assert "<facade::Point as dependency::Measure>::default_method" in roots
        assert all(functions[symbol].get("body") is not None or "constructor" in functions[symbol] for symbol in roots.values())
        assert roots["facade::Tuple"] == roots["facade::RenamedTuple"]
        assert "facade::TupleAlias" not in roots and "facade::PrivateTuple" not in roots
        assert roots["facade::Choice::Tuple"] == roots["facade::ChoiceAlias::Tuple"]
        assert "facade::Choice::Empty" in roots
        assert "facade::Choice::Named" not in roots and "facade::Choice::Unit" not in roots
        for name in ("facade::identity", "facade::Point::generic", "facade::Byte::borrowed"):
            assert any(api["name"] == name and api["status"] == "requires_monomorphization" for api in inventory)
            assert name not in roots
        assert any(api["name"] == "facade::cycle::again" and api["status"] == "recursive_reexport" for api in inventory)
        assert any(api["kind"] == "trait_method" and api["name"].endswith("::generic_method") and api["status"] == "requires_monomorphization" for api in inventory)
        assert any(api["name"] == "<facade::NotClone as dependency::Conditional>::sized_only" and api["status"] == "unsatisfied_predicates" for api in inventory)
        assert any(api["name"] == "<facade::NotClone as core::ops::Drop>::drop" and api["status"] == "compiler_managed" for api in inventory)
        assert not any("PrivateTrait" in api["name"] for api in inventory)
        assert not any(api.get("selected") and api["status"] != "monomorphic" for api in inventory)

        # Independent downstream rustc checks that the alias/type/method paths really work.
        subprocess.run([str(sysroot / "bin/rustc"), *flags, str(source / "consumer.rs"),
                        "--extern", f"facade={facade}", "-L", f"dependency={directory}",
                        "-o", str(directory / "libconsumer.rlib")], check=True)
        subprocess.run(command, env={**env, "OXIDE_ROOTS": "facade::alias,facade::selected_private"}, check=True)
        selected = json.loads(output.read_text())
        assert {value["name"] for value in selected["roots"]} == {"facade::alias", "facade::selected_private"}
        pair = next(name for name in roots if name.startswith("<facade::Point as dependency::Pair<"))
        subprocess.run(command, env={**env, "OXIDE_ROOTS": pair + ",facade::alias"}, check=True)
        assert {value["name"] for value in json.loads(output.read_text())["roots"]} == {pair, "facade::alias"}
        for requested in ("facade::identity", "facade::alias,facade::missing"):
            result = subprocess.run(command, env={**env, "OXIDE_ROOTS": requested}, capture_output=True, text=True)
            assert result.returncode != 0 and "cannot select root" in result.stderr, result.stderr
        # An identical user-defined path must not acquire a standard-library boundary.
        subprocess.run([str(frontend), "--export", str(output), "--", *flags,
                        str(source / "fake_std.rs"), "--crate-name", "std",
                        "-o", str(directory / "libfake_std.rlib")], env=env, check=True)
        fake = json.loads(output.read_text())
        assert fake["roots"][0]["name"] == "std::sys::args::unix::imp::argc_argv"
        assert not any(f.get("runtime_boundary") for f in fake["functions"])
        subprocess.run([str(frontend), "--export", str(output), "--", *flags,
                        str(source / "fake_std_facade.rs"), "--crate-name", "fake_facade",
                        "--extern", f"std={directory / 'libfake_std.rlib'}",
                        "-o", str(directory / "libfake_facade.rlib")], env=env, check=True)
        fake = json.loads(output.read_text())
        assert any(f["name"] == "std::sys::args::unix::imp::argc_argv" for f in fake["functions"])
        assert not any(f.get("runtime_boundary") for f in fake["functions"])
        print(f"PASS {target}: {len(roots)} public roots, {len(inventory)} API entries")
