#!/usr/bin/env python3
"""Validate public export completeness using only the small MIR .api.json sidecar.

The checked inventory separates exported entry points from individually tested
behavior. Derived/default trait methods without a dedicated probe stay explicit.
"""

import argparse
import ast
import hashlib
import json
from pathlib import Path


HERE = Path(__file__).resolve().parent


def read_json(path):
    return json.loads(Path(path).read_text())


def cli_names(path):
    for node in ast.parse(Path(path).read_text()).body:
        if isinstance(node, ast.Assign) and any(isinstance(t, ast.Name) and t.id == "CASES" for t in node.targets):
            assert isinstance(node.value, ast.List), "CLI CASES must be a literal list"
            return {row.elts[0].value for row in node.value.elts}
    raise AssertionError("CLI CASES declaration missing")


def unique(items, key, description):
    result = {}
    for item in items:
        name = item[key]
        assert name not in result, f"duplicate {description}: {name}"
        result[name] = item
    return result


def check_api(api_path, cases_path, inventory_path=None, cli_results_path=None):
    inventory = read_json(inventory_path or HERE / "api-inventory.json")
    actual = read_json(api_path)
    cases = read_json(cases_path)
    assert inventory["schema_version"] == 1
    assert actual["target"] in ("aarch64-unknown-linux-gnu", "x86_64-unknown-linux-gnu")
    assert actual["compiler"] == inventory["source"]["compiler"], "compiler changed; regenerate API inventory"
    assert hashlib.sha256((HERE / "api.rs").read_bytes()).hexdigest() == inventory["source"]["api_rs_sha256"], "API probe source changed; regenerate inventory and references"
    case_map = unique(cases, "name", "native API case")
    assert set(case_map) == set(inventory["probe_cases"]), "native CASES and inventory differ"
    assert sorted(case["id"] for case in cases) == list(range(len(cases))), "invalid native case ids"
    for case in cases:
        reference = Path(cases_path).parent / case["file"]
        assert reference.is_file(), f"missing native API bytes: {reference}"
    processes = cli_names(HERE / "test_cli.py")
    assert processes == set(inventory["process_cases"]), "CLI process CASES changed"
    if cli_results_path is not None:
        assert set(read_json(cli_results_path)) == processes, "native CLI result inventory differs"

    items = unique(actual["public_api"], "name", "compiler public path")
    roots = unique(actual["roots"], "name", "root path")
    upstream = set(items)
    expected_paths = {item["name"] for item in inventory["public_api"]}
    assert upstream == expected_paths, (
        "upstream API inventory changed",
        {"missing": sorted(expected_paths - upstream), "unexpected": sorted(upstream - expected_paths)},
    )
    selected = generic = 0
    # Every selected root belongs to the upstream library; no oracle roots.
    for name, item in items.items():
        if item["status"] == "monomorphic":
            assert item["selected"] is True, f"monomorphic API not selected: {name}"
            assert name in roots and roots[name].get("symbol"), f"API root missing: {name}"
            selected += 1
        else:
            assert item["status"] == "requires_monomorphization", (name, item["status"])
            generic += 1

    for expected in inventory["public_api"]:
        name = expected["name"]
        assert name in items, f"public path disappeared: {name}"
        item = items[name]
        assert item["kind"] == expected["kind"], f"API kind changed: {name}"
        assert item["definition"] == expected["definition"], f"API definition changed: {name}"
        assert item["status"] == expected["status"], f"generic status changed: {name}"
        if "trait_definition" in expected:
            assert item.get("trait_definition") == expected["trait_definition"], f"canonical trait changed: {name}"

    paths = items
    kinds = {"free": "function", "inherent": "inherent_method", "constructor": "constructor"}
    for definition in inventory["functions"]:
        assert set(definition["probes"]) <= set(case_map), f"unknown probe for {definition['id']}"
        assert set(definition["process_probes"]) <= processes, f"unknown process probe for {definition['id']}"
        if definition["kind"] in kinds:
            assert definition["probes"] or definition["process_probes"], f"untested public callable: {definition['id']}"
            for alias in definition["aliases"]:
                assert alias in paths, f"rustdoc public alias missing: {alias}"
                assert paths[alias]["kind"] == kinds[definition["kind"]], f"rustdoc/compiler kind mismatch: {alias}"
        if definition["kind"] == "explicit_trait":
            assert definition["probes"] or definition["process_probes"], f"explicit trait method lacks a probe: {definition['id']}"
    result = {
        "target": actual["target"], "audited_upstream_paths": len(inventory["public_api"]),
        "selected_monomorphic_paths": selected, "generic_declarations": generic,
        "native_api_cases": len(cases), "cli_process_cases": len(processes),
        "derived_default_behavior": "not every exported derived/default method is individually probed",
    }
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("api", type=Path, help="oxide.mir.api.json sidecar; never the full MIR")
    parser.add_argument("--cases", type=Path, required=True, help="native API cases.json")
    parser.add_argument("--inventory", type=Path, default=HERE / "api-inventory.json")
    parser.add_argument("--cli-results", type=Path)
    args = parser.parse_args()
    print(json.dumps(check_api(args.api, args.cases, args.inventory, args.cli_results), indent=2))


if __name__ == "__main__":
    main()
