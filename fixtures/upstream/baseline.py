"""Fail-closed, monotonic regression baselines for upstream Rust test runs.

Case identifiers and context are supplied by the runner.  Context is an opaque
JSON object: changing the source revision, target or adapter requires an explicit
new baseline, rather than silently reusing successes from a different run.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path
import sys
from typing import Any


def _json_value(value: Any) -> bool:
    if value is None or type(value) in (bool, int, float, str):
        return True
    if type(value) is list:
        return all(_json_value(item) for item in value)
    if type(value) is dict:
        return all(type(key) is str and _json_value(item) for key, item in value.items())
    return False


def _context(value: Any, label: str, errors: list[str]) -> str | None:
    try:
        if type(value) is not dict or not _json_value(value):
            raise ValueError("not a JSON object")
        # Serialization also rejects NaN/infinity and distinguishes True from 1.
        return json.dumps(value, allow_nan=False, sort_keys=True, separators=(",", ":"))
    except (TypeError, ValueError, RecursionError):
        errors.append(f"{label} must be a finite JSON object")
        return None


def _ids(value: Any, label: str, errors: list[str]) -> set[str] | None:
    if type(value) is not list:
        errors.append(f"{label} must be a nonempty list of case IDs")
        return None
    if not value:
        errors.append(f"{label} must not be empty")
        return None
    if any(type(case_id) is not str or not case_id.strip() for case_id in value):
        errors.append(f"{label} contains an invalid case ID")
        return None
    seen: set[str] = set()
    duplicates: set[str] = set()
    for case_id in value:
        if case_id in seen:
            duplicates.add(case_id)
        seen.add(case_id)
    if duplicates:
        errors.append(f"{label} contains duplicate case IDs: {', '.join(sorted(duplicates))}")
    return seen


def _run(
    context: Any, discovered: Any, results: Any, errors: list[str]
) -> tuple[str | None, set[str] | None, dict[str, str] | None]:
    context_key = _context(context, "context", errors)
    inventory = _ids(discovered, "discovered", errors)
    if type(results) is not dict:
        errors.append("results must be an object mapping case IDs to statuses")
        return context_key, inventory, None
    valid_results: dict[str, str] = {}
    for case_id, status in results.items():
        if type(case_id) is not str or not case_id.strip():
            errors.append("results contains an invalid case ID")
            continue
        if type(status) is not str or not status.strip():
            errors.append(f"results[{case_id!r}] must be a nonempty status string")
            continue
        valid_results[case_id] = status
    if inventory is not None:
        unknown = set(valid_results) - inventory
        if unknown:
            errors.append(f"results contains undiscovered case IDs: {', '.join(sorted(unknown))}")
    return context_key, inventory, valid_results


def _baseline(
    baseline: Any, errors: list[str]
) -> tuple[str | None, set[str] | None, set[str] | None]:
    if type(baseline) is not dict:
        errors.append("baseline must be an object")
        return None, None, None
    if type(baseline.get("schema")) is not int or baseline.get("schema") != 1:
        errors.append("baseline.schema must be integer 1")
    context_key = _context(baseline.get("context"), "baseline.context", errors)
    passed = _ids(baseline.get("passed"), "baseline.passed", errors)
    inventory = _ids(baseline.get("discovered"), "baseline.discovered", errors)
    if passed is not None and inventory is not None:
        unknown = passed - inventory
        if unknown:
            errors.append(f"baseline.passed contains undiscovered case IDs: {', '.join(sorted(unknown))}")
    return context_key, passed, inventory


def validate_baseline(
    baseline: dict,
    context: dict,
    discovered: list[str],
    results: dict[str, str],
    *,
    complete: bool = False,
) -> list[str]:
    """Return errors if a recorded success regresses or run data is malformed.

    By default only previously passing cases must run successfully.  Complete
    validation additionally requires the recorded discovery inventory to match
    exactly and a result for every discovered case; failures/ignored cases that
    have never passed are permitted.
    """
    errors: list[str] = []
    baseline_context, passed, previous_inventory = _baseline(baseline, errors)
    run_context, inventory, run_results = _run(context, discovered, results, errors)
    if baseline_context is not None and run_context is not None and baseline_context != run_context:
        errors.append("context does not match baseline.context")
    if passed is not None:
        for case_id in sorted(passed):
            if inventory is not None and case_id not in inventory:
                errors.append(f"previously passing case disappeared: {case_id}")
            if run_results is not None:
                if case_id not in run_results:
                    errors.append(f"previously passing case has no result: {case_id}")
                elif run_results[case_id] != "passed":
                    errors.append(f"previously passing case regressed: {case_id} ({run_results[case_id]})")
    if complete:
        if inventory is not None and previous_inventory is not None:
            removed = previous_inventory - inventory
            added = inventory - previous_inventory
            if removed:
                errors.append(f"discovery removed case IDs: {', '.join(sorted(removed))}")
            if added:
                errors.append(f"discovery added case IDs: {', '.join(sorted(added))}")
        if inventory is not None and run_results is not None:
            missing = inventory - set(run_results)
            if missing:
                errors.append(f"discovered cases have no result: {', '.join(sorted(missing))}")
    return errors


def promote_baseline(
    baseline: dict | None,
    context: dict,
    discovered: list[str],
    results: dict[str, str],
) -> dict:
    """Create or extend a baseline from a complete run without losing coverage.

    Every discovered case needs a result, but only exact ``passed`` statuses
    enter the passing set.  Existing passing cases must still pass, existing
    discovery entries cannot be removed, and the context cannot change.  The
    returned data is detached from the inputs; invalid input raises ValueError.
    """
    errors: list[str] = []
    _, inventory, run_results = _run(context, discovered, results, errors)
    if baseline is not None:
        errors.extend(validate_baseline(baseline, context, discovered, results))
        if not errors and inventory is not None:
            removed = set(baseline["discovered"]) - inventory
            if removed:
                errors.append(f"promotion would remove discovered case IDs: {', '.join(sorted(removed))}")
    if inventory is not None and run_results is not None:
        missing = inventory - set(run_results)
        if missing:
            errors.append(f"cannot promote incomplete results: {', '.join(sorted(missing))}")
    passed = {case_id for case_id, status in (run_results or {}).items() if status == "passed"}
    if not passed:
        errors.append("cannot promote an empty passing baseline")
    if errors:
        raise ValueError("; ".join(dict.fromkeys(errors)))
    return {
        "schema": 1,
        "context": json.loads(json.dumps(context, allow_nan=False)),
        "passed": sorted(passed),
        "discovered": sorted(inventory),
    }


def validate_history(previous: dict, current: dict) -> list[str]:
    """Reject edits that remove coverage or change an existing baseline's scope.

    CI supplies the baseline from its trusted base revision as ``previous``.
    This complements runtime validation: hand-editing a passing case out of a
    JSON baseline must not weaken the next regression run.
    """
    previous_errors: list[str] = []
    current_errors: list[str] = []
    previous_context, previous_passed, previous_inventory = _baseline(previous, previous_errors)
    current_context, current_passed, current_inventory = _baseline(current, current_errors)
    errors = ["previous: " + error for error in previous_errors]
    errors.extend("current: " + error for error in current_errors)
    if previous_context is not None and current_context is not None and previous_context != current_context:
        errors.append("current context does not match previous context")
    if previous_passed is not None and current_passed is not None:
        removed = previous_passed - current_passed
        if removed:
            errors.append(f"history removed passing case IDs: {', '.join(sorted(removed))}")
    if previous_inventory is not None and current_inventory is not None:
        removed = previous_inventory - current_inventory
        if removed:
            errors.append(f"history removed discovered case IDs: {', '.join(sorted(removed))}")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description="Reject upstream baseline changes that lose coverage.")
    parser.add_argument("previous", type=Path, help="baseline from the trusted base revision")
    parser.add_argument("current", type=Path, help="baseline in the proposed revision")
    args = parser.parse_args()
    try:
        previous = json.loads(args.previous.read_text())
        current = json.loads(args.current.read_text())
    except (OSError, ValueError) as error:
        print(f"baseline history: {error}", file=sys.stderr)
        return 1
    errors = validate_history(previous, current)
    if errors:
        for error in errors:
            print(f"baseline history: {error}", file=sys.stderr)
        return 1
    print("baseline history preserves all previous coverage")
    return 0


if __name__ == "__main__":
    sys.exit(main())
