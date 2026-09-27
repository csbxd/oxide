#!/usr/bin/env python3
"""Exercise coverage rejection against a completed small API sidecar and references."""

import argparse
import copy
import json
from pathlib import Path
import tempfile

from check_api import check_api
from inventory import attach_probes

HERE = Path(__file__).resolve().parent

def monomorphic(api):
    return next(item for item in api["public_api"] if item["status"] == "monomorphic")

def disable(api, inventory):
    monomorphic(api)["selected"] = False

def remove_root(api, inventory):
    name = monomorphic(api)["name"]
    api["roots"] = [root for root in api["roots"] if root["name"] != name]

def remove_alias(api, inventory):
    name = next(item["name"] for item in inventory["public_api"] if item["kind"] == "function")
    api["public_api"] = [item for item in api["public_api"] if item["name"] != name]

def add_api(api, inventory):
    item = copy.deepcopy(next(item for item in api["public_api"] if item["kind"] == "function"))
    item["name"] += "_unexpected"
    api["public_api"].append(item)
    api["roots"].append({"name": item["name"], "symbol": "unexpected"})

def change_trait(api, inventory):
    next(item for item in api["public_api"] if "trait_definition" in item)["trait_definition"] = "core::wrong::Trait"

def bad_probe(api, inventory):
    inventory["functions"][0]["probes"] = ["missing_probe"]

def stale_source(api, inventory):
    inventory["source"]["api_rs_sha256"] = "0" * 64

def no_behavior_probe(api, inventory):
    item = next(item for item in inventory["functions"] if item["kind"] == "inherent")
    item["probes"] = []
    item["process_probes"] = []

def test_rejections(api_path, cases_path, cli_path, inventory_path):
    positive = check_api(api_path, cases_path, inventory_path, cli_path)
    baseline_api = json.loads(api_path.read_text())
    baseline_inventory = json.loads(inventory_path.read_text())
    mutations = [disable, remove_root, remove_alias, add_api, change_trait, bad_probe, stale_source, no_behavior_probe]
    failures = []
    with tempfile.TemporaryDirectory(prefix="coverage-negative-", dir=api_path.parent) as temporary:
        temporary = Path(temporary)
        for mutate in mutations:
            api, inventory = copy.deepcopy(baseline_api), copy.deepcopy(baseline_inventory)
            mutate(api, inventory)
            (temporary / "api.json").write_text(json.dumps(api))
            (temporary / "inventory.json").write_text(json.dumps(inventory))
            try:
                check_api(temporary / "api.json", cases_path, temporary / "inventory.json", cli_path)
            except AssertionError as error:
                failures.append({"mutation": mutate.__name__, "detected": str(error)})
            else:
                raise AssertionError(f"coverage gate accepted {mutate.__name__}")
    # A new method on an already-tested type cannot inherit its siblings' probes.
    unexpected = {"id": "mermaid_rs_renderer::Theme::unmapped_method", "kind": "inherent"}
    try:
        attach_probes([unexpected], {}, set(), set())
    except AssertionError as error:
        assert "unmapped public API" in str(error), error
    else:
        raise AssertionError("new method inherited a false behavior-coverage claim")
    derived = {"id": "<mermaid_rs_renderer::Theme as ExtraTrait>::method", "kind": "derived_trait"}
    attach_probes([derived], {}, set(), set())
    assert derived["probe_status"] == "not_individually_probed" and not derived["probes"]
    return {"positive": positive, "negative_controls": failures,
            "mapping_controls": "new handwritten method rejected; new derived method explicitly untested"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api", type=Path, required=True)
    parser.add_argument("--cases", type=Path, required=True)
    parser.add_argument("--cli-results", type=Path, required=True)
    parser.add_argument("--inventory", type=Path, default=HERE / "api-inventory.json")
    args = parser.parse_args()
    print(json.dumps(test_rejections(args.api, args.cases, args.cli_results, args.inventory), indent=2))


if __name__ == "__main__":
    main()
