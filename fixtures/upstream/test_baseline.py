"""Regression scenarios for the upstream test gate (standard-library only)."""

import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from baseline import promote_baseline, validate_baseline, validate_history


class BaselineGateTests(unittest.TestCase):
    def setUp(self):
        self.context = {"rust_commit": "abc", "target": "x86_64", "sources": {"core": "sha"}}
        self.discovered = ["core::good", "core::todo", "alloc::ignored"]
        self.results = {
            "core::good": "passed",
            "core::todo": "failed",
            "alloc::ignored": "ignored",
        }
        self.baseline = promote_baseline(None, self.context, self.discovered, self.results)

    def check(self, **changes):
        arguments = dict(
            baseline=self.baseline,
            context=self.context,
            discovered=self.discovered,
            results=self.results,
        )
        arguments.update(changes)
        return validate_baseline(**arguments)

    def promote(self, **changes):
        arguments = dict(
            baseline=self.baseline,
            context=self.context,
            discovered=self.discovered,
            results=self.results,
        )
        arguments.update(changes)
        return promote_baseline(**arguments)

    def test_known_failures_and_ignored_cases_do_not_block_the_gate(self):
        self.assertEqual(self.check(), [])
        self.assertEqual(self.check(complete=True), [])

    def test_each_nonpassing_outcome_regresses_an_existing_success(self):
        for status in ["failed", "blocked", "ignored", "skipped", "timeout", "error", "PASSED", " passed"]:
            with self.subTest(status=status):
                results = dict(self.results, **{"core::good": status})
                self.assertTrue(self.check(results=results))
                with self.assertRaises(ValueError):
                    self.promote(results=results)

    def test_missing_result_cannot_hide_a_regression(self):
        del self.results["core::good"]
        self.assertTrue(self.check())
        with self.assertRaises(ValueError):
            self.promote()

    def test_removed_test_cannot_be_marked_as_passed_by_the_report(self):
        self.discovered.remove("core::good")
        self.assertTrue(self.check())
        with self.assertRaises(ValueError):
            self.promote()

    def test_an_empty_or_malformed_baseline_never_disables_the_gate(self):
        for baseline in [None, {}, [], {**self.baseline, "passed": []}, {**self.baseline, "discovered": []}]:
            with self.subTest(baseline=baseline):
                self.assertTrue(self.check(baseline=baseline))

    def test_unknown_results_cannot_seed_fake_successes(self):
        self.results["core::invented"] = "passed"
        self.assertTrue(self.check())
        with self.assertRaises(ValueError):
            self.promote(baseline=None)

    def test_baseline_success_must_have_been_discovered(self):
        self.baseline["passed"].append("core::invented")
        self.discovered.append("core::invented")
        self.results["core::invented"] = "passed"
        self.assertTrue(self.check())

    def test_duplicate_case_ids_are_rejected_in_every_inventory(self):
        for field in ["passed", "discovered"]:
            with self.subTest(field=field):
                baseline = copy.deepcopy(self.baseline)
                baseline[field].append(baseline[field][0])
                self.assertTrue(self.check(baseline=baseline))
        self.discovered.append(self.discovered[0])
        self.assertTrue(self.check())
        with self.assertRaises(ValueError):
            self.promote(baseline=None)

    def test_context_change_cannot_reuse_previous_successes(self):
        changed = copy.deepcopy(self.context)
        changed["sources"]["core"] = "different hash"
        self.assertTrue(self.check(context=changed))
        with self.assertRaises(ValueError):
            self.promote(context=changed)

    def test_context_comparison_is_order_independent_and_type_sensitive(self):
        reordered = dict(reversed(list(self.context.items())))
        self.assertEqual(self.check(context=reordered), [])
        baseline = copy.deepcopy(self.baseline)
        baseline["context"]["flag"] = True
        self.assertTrue(self.check(baseline=baseline, context={**self.context, "flag": 1}))

    def test_invalid_json_context_is_rejected(self):
        for context in [None, [], {1: "value"}, {"number": float("nan")}, {"tuple": (1,)}]:
            with self.subTest(context=context):
                self.assertTrue(self.check(context=context))
                with self.assertRaises(ValueError):
                    self.promote(baseline=None, context=context)

    def test_invalid_status_types_and_blank_statuses_fail_closed(self):
        for status in [True, 1, None, [], {}, "", "  "]:
            with self.subTest(status=status):
                results = dict(self.results, **{"core::todo": status})
                self.assertTrue(self.check(results=results))
                with self.assertRaises(ValueError):
                    self.promote(baseline=None, results=results)

    def test_malformed_ids_and_result_shapes_return_errors(self):
        for discovered in [None, {}, [], [""], [" "], [1], [["nested"]]]:
            with self.subTest(discovered=discovered):
                self.assertTrue(self.check(discovered=discovered))
        for results in [None, [], {1: "passed"}, {"": "passed"}]:
            with self.subTest(results=results):
                self.assertTrue(self.check(results=results))

    def test_schema_is_explicit_and_not_a_boolean(self):
        for schema in [None, True, 0, 2, "1"]:
            with self.subTest(schema=schema):
                self.assertTrue(self.check(baseline={**self.baseline, "schema": schema}))

    def test_default_gate_can_run_only_the_previous_successes(self):
        self.assertEqual(self.check(results={"core::good": "passed"}), [])
        self.assertTrue(self.check(results={"core::good": "passed"}, complete=True))

    def test_complete_check_requires_unchanged_discovery(self):
        self.discovered.append("alloc::new")
        self.results["alloc::new"] = "failed"
        self.assertEqual(self.check(), [])
        self.assertTrue(self.check(complete=True))
        self.discovered.remove("alloc::new")
        del self.results["alloc::new"]
        self.discovered.remove("core::todo")
        del self.results["core::todo"]
        self.assertEqual(self.check(), [])
        self.assertTrue(self.check(complete=True))

    def test_promotion_adds_successes_and_immediately_protects_them(self):
        self.results["core::todo"] = "passed"
        self.discovered.append("alloc::new")
        self.results["alloc::new"] = "passed"
        promoted = self.promote()
        self.assertEqual(promoted["passed"], ["alloc::new", "core::good", "core::todo"])
        self.assertEqual(promoted["discovered"], sorted(self.discovered))
        self.results["alloc::new"] = "failed"
        self.assertTrue(self.check(baseline=promoted))
        with self.assertRaises(ValueError):
            self.promote(baseline=promoted)

    def test_promotion_rejects_incomplete_runs_even_for_previous_failures(self):
        del self.results["core::todo"]
        self.assertEqual(self.check(), [])
        with self.assertRaises(ValueError):
            self.promote()

    def test_promotion_cannot_delete_known_cases(self):
        self.discovered.remove("core::todo")
        del self.results["core::todo"]
        with self.assertRaises(ValueError):
            self.promote()

    def test_new_baseline_needs_at_least_one_real_success(self):
        self.results["core::good"] = "failed"
        with self.assertRaises(ValueError):
            self.promote(baseline=None)

    def test_promotion_is_idempotent_and_does_not_mutate_inputs(self):
        original = copy.deepcopy((self.baseline, self.context, self.discovered, self.results))
        promoted = self.promote()
        self.assertEqual(promoted, self.baseline)
        self.assertEqual(original, (self.baseline, self.context, self.discovered, self.results))
        promoted["context"]["sources"]["core"] = "mutated"
        promoted["passed"].append("extra")
        self.assertEqual(original, (self.baseline, self.context, self.discovered, self.results))


class BaselineHistoryTests(unittest.TestCase):
    def setUp(self):
        self.context = {"rust_commit": "abc", "target": "arm64"}
        self.discovered = ["core/good", "alloc/good", "alloc/todo"]
        self.results = {"core/good": "passed", "alloc/good": "passed", "alloc/todo": "failed"}
        self.previous = promote_baseline(None, self.context, self.discovered, self.results)

    def test_hand_deleting_a_passing_case_is_rejected_even_if_inventory_retains_it(self):
        current = copy.deepcopy(self.previous)
        current["passed"].remove("alloc/good")
        errors = validate_history(self.previous, current)
        self.assertTrue(any("passing" in error and "alloc/good" in error for error in errors))

    def test_hand_deleting_a_previously_failing_case_is_also_rejected(self):
        current = copy.deepcopy(self.previous)
        current["discovered"].remove("alloc/todo")
        errors = validate_history(self.previous, current)
        self.assertTrue(any("discovered" in error and "alloc/todo" in error for error in errors))

    def test_actual_promotion_extends_history_without_changing_inputs(self):
        before = copy.deepcopy(self.previous)
        discovered = self.discovered + ["core/new"]
        results = {**self.results, "alloc/todo": "passed", "core/new": "passed"}
        current = promote_baseline(self.previous, self.context, discovered, results)
        self.assertEqual(validate_history(self.previous, current), [])
        self.assertEqual(self.previous, before)
        self.assertEqual(validate_history(current, current), [])

    def test_changed_context_cannot_be_disguised_as_promotion(self):
        current = copy.deepcopy(self.previous)
        current["context"]["rust_commit"] = "different"
        self.assertTrue(any("context" in error for error in validate_history(self.previous, current)))

    def test_context_uses_strict_json_types(self):
        self.previous["context"]["flag"] = True
        current = copy.deepcopy(self.previous)
        current["context"]["flag"] = 1
        self.assertTrue(validate_history(self.previous, current))

    def test_malformed_baselines_on_either_side_never_disable_history(self):
        for broken in [
            None,
            {},
            {**self.previous, "schema": True},
            {**self.previous, "schema": 2},
            {**self.previous, "context": []},
            {**self.previous, "passed": []},
            {**self.previous, "passed": ["core/good", "core/good"]},
            {**self.previous, "discovered": []},
            {**self.previous, "discovered": self.discovered + ["core/good"]},
            {**self.previous, "passed": ["core/invented"]},
        ]:
            with self.subTest(broken=broken):
                self.assertTrue(validate_history(self.previous, broken))
                self.assertTrue(validate_history(broken, self.previous))

    def test_cli_rejects_json_edit_and_accepts_real_promotion(self):
        with tempfile.TemporaryDirectory() as temporary:
            previous_file = Path(temporary) / "previous.json"
            current_file = Path(temporary) / "current.json"
            previous_file.write_text(json.dumps(self.previous))
            command = [sys.executable, str(Path(__file__).with_name("baseline.py")),
                       str(previous_file), str(current_file)]
            current = copy.deepcopy(self.previous)
            current["passed"].remove("core/good")
            current_file.write_text(json.dumps(current))
            rejected = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(rejected.returncode, 1)
            self.assertIn("core/good", rejected.stderr)
            current = promote_baseline(self.previous, self.context, self.discovered,
                                       {**self.results, "alloc/todo": "passed"})
            current_file.write_text(json.dumps(current))
            accepted = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(accepted.returncode, 0, accepted.stderr)
            current["context"]["target"] = "amd64"
            current_file.write_text(json.dumps(current))
            rejected = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(rejected.returncode, 1)
            self.assertIn("context", rejected.stderr)

    def test_cli_missing_or_invalid_json_fails(self):
        with tempfile.TemporaryDirectory() as temporary:
            previous_file = Path(temporary) / "previous.json"
            current_file = Path(temporary) / "current.json"
            previous_file.write_text(json.dumps(self.previous))
            command = [sys.executable, str(Path(__file__).with_name("baseline.py")),
                       str(previous_file), str(current_file)]
            self.assertEqual(subprocess.run(command, capture_output=True).returncode, 1)
            current_file.write_text("invalid json")
            self.assertEqual(subprocess.run(command, capture_output=True).returncode, 1)


if __name__ == "__main__":
    unittest.main()
