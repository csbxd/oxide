"""Exact candidate selection must retain every previously passing test."""

from contextlib import redirect_stdout
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import Mock, patch

import run
from test_parallel import add_case, runner_fixture


class SelectionTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.directory = Path(temporary.name)
        self.runner = runner_fixture()
        self.runner.args.select = None
        self.old = add_case(self.runner, "old_pass")
        self.new = add_case(self.runner, "new_candidate")
        self.other = add_case(self.runner, "unselected")
        self.runner.base = {"schema": 1, "context": self.runner.context,
                            "passed": [self.old], "discovered": sorted(self.runner.cases)}
        self.runner.basefile = self.directory / "baseline.json"
        self.runner.basefile.write_text(json.dumps(self.runner.base))
        self.original_baseline = self.runner.basefile.read_bytes()

    def select(self, value):
        path = self.directory / "selection.json"
        path.write_text(json.dumps(value))
        self.runner.args.select = path

    def prepare_run(self, perform):
        self.runner.prepare = Mock()
        self.runner.discover = Mock()
        self.runner.command = Mock()
        self.runner.run_batches = Mock(side_effect=perform)

    def test_promote_always_includes_previous_passes(self):
        self.select([self.new])
        self.assertEqual(self.runner.select_cases(), {self.old, self.new})

    def test_scan_does_not_add_previously_passing_cases(self):
        self.runner.args.command = "scan"
        self.select([self.new])
        self.assertEqual(self.runner.select_cases(), {self.new})

    def test_disappeared_previous_success_cannot_be_silently_omitted(self):
        self.select([self.new])
        del self.runner.cases[self.old]
        with self.assertRaises((RuntimeError, ValueError)):
            self.runner.select_cases()

    def test_default_selection_covers_the_whole_discovery(self):
        for command in ["scan", "promote"]:
            with self.subTest(command=command):
                self.runner.args.command = command
                self.assertEqual(self.runner.select_cases(), set(self.runner.cases))

    def test_check_selects_existing_passes(self):
        self.runner.args.command = "check"
        self.assertEqual(self.runner.select_cases(), {self.old})

    def test_filter_promote_retains_previous_passes(self):
        self.runner.args.filter = "new_candidate"
        self.assertEqual(self.runner.select_cases(), {self.old, self.new})
        self.runner.args.command = "scan"
        self.assertEqual(self.runner.select_cases(), {self.new})

    def test_empty_filter_cannot_be_hidden_by_existing_passes(self):
        self.runner.args.filter = "no-matching-case"
        with self.assertRaises((RuntimeError, ValueError)):
            self.runner.select_cases()

    def test_empty_unknown_duplicate_and_invalid_selection_shapes_fail(self):
        for selected in [[], ["coretests/unknown"], [self.new, self.new], {}, None,
                         self.new, [1], [None], [""], [" "], [[self.new]]]:
            with self.subTest(selected=selected):
                self.select(selected)
                with self.assertRaises((RuntimeError, ValueError)):
                    self.runner.select_cases()

    def test_invalid_json_fails(self):
        self.select([self.new])
        self.runner.args.select.write_text("[not-json")
        with self.assertRaises((RuntimeError, ValueError)):
            self.runner.select_cases()

    def test_selected_active_cases_must_have_real_outcomes(self):
        for status in [None, "not_run", "ignored", "skipped", "PASSED", ""]:
            with self.subTest(status=status):
                if status is None:
                    self.runner.results.pop(self.new, None)
                else:
                    self.runner.results[self.new] = status
                with self.assertRaises((RuntimeError, ValueError)):
                    self.runner.require_selected_results({self.new})

    def test_real_failures_are_outcomes_and_unselected_cases_can_remain_unrun(self):
        for status in ["passed", "failed", "blocked", "timeout", "native_failed", "export_failed", "go-build_failed"]:
            with self.subTest(status=status):
                self.runner.results[self.new] = status
                self.runner.require_selected_results({self.new})
                self.assertEqual(self.runner.results[self.other], "not_run")

    def test_upstream_ignored_case_can_remain_ignored(self):
        ignored = add_case(self.runner, "ignored", ignored=True)
        self.runner.require_selected_results({ignored})

    def test_promote_runs_old_passes_and_only_protects_actual_new_successes(self):
        self.select([self.new])

        def perform(selected):
            self.assertEqual(selected, {self.old, self.new})
            for key in selected:
                self.runner.record(key, "passed")

        self.prepare_run(perform)
        with redirect_stdout(io.StringIO()):
            self.runner.run()
        promoted = json.loads(self.runner.basefile.read_text())
        self.assertEqual(promoted["passed"], sorted([self.old, self.new]))
        self.assertEqual(promoted["discovered"], sorted(self.runner.cases))
        self.assertNotIn(self.other, promoted["passed"])
        self.assertEqual(self.runner.results[self.other], "not_run")
        self.runner.run_batches.assert_called_once_with({self.old, self.new})

    def test_regressed_old_success_prevents_saving_new_success(self):
        self.select([self.new])

        def perform(selected):
            self.assertEqual(selected, {self.old, self.new})
            self.runner.record(self.old, "failed")
            self.runner.record(self.new, "passed")

        self.prepare_run(perform)
        with self.assertRaisesRegex(RuntimeError, "regress"):
            self.runner.run()
        self.assertEqual(self.runner.basefile.read_bytes(), self.original_baseline)

    def test_initial_selected_promotion_protects_only_executed_successes(self):
        self.runner.base = None
        self.runner.basefile = self.directory / "new-baseline.json"
        self.select([self.new])
        self.prepare_run(lambda selected: self.runner.record(self.new, "passed"))
        with redirect_stdout(io.StringIO()):
            self.runner.run()
        promoted = json.loads(self.runner.basefile.read_text())
        self.assertEqual(promoted["passed"], [self.new])
        self.assertEqual(promoted["discovered"], sorted(self.runner.cases))
        self.assertEqual(self.runner.results[self.old], "not_run")

    def test_failed_candidate_is_not_promoted_but_keeps_existing_success(self):
        self.select([self.new])

        def perform(selected):
            self.runner.record(self.old, "passed")
            self.runner.record(self.new, "failed")

        self.prepare_run(perform)
        with redirect_stdout(io.StringIO()):
            self.runner.run()
        self.assertEqual(json.loads(self.runner.basefile.read_text())["passed"], [self.old])

    def test_blocked_candidate_is_not_promoted_and_old_blocked_success_cannot_be_saved(self):
        self.select([self.new])

        def new_candidate_blocked(selected):
            self.runner.record(self.old, "passed")
            self.runner.record(self.new, "blocked")

        self.prepare_run(new_candidate_blocked)
        with redirect_stdout(io.StringIO()):
            self.runner.run()
        self.assertEqual(json.loads(self.runner.basefile.read_text())["passed"], [self.old])
        saved = self.runner.basefile.read_bytes()

        def old_success_blocked(selected):
            self.runner.record(self.old, "blocked")
            self.runner.record(self.new, "passed")

        self.prepare_run(old_success_blocked)
        with self.assertRaisesRegex(RuntimeError, "regress"):
            self.runner.run()
        self.assertEqual(self.runner.basefile.read_bytes(), saved)

    def test_check_rejects_an_old_success_blocked_by_shared_support(self):
        self.runner.args.command = "check"
        self.prepare_run(lambda selected: self.runner.record(self.old, "blocked"))
        with self.assertRaisesRegex(RuntimeError, "regress"):
            self.runner.run()
        self.assertEqual(self.runner.basefile.read_bytes(), self.original_baseline)

    def test_missing_selected_result_prevents_saving(self):
        self.select([self.new])
        self.prepare_run(lambda selected: self.runner.record(self.old, "passed"))
        with self.assertRaises((RuntimeError, ValueError)):
            self.runner.run()
        self.assertEqual(self.runner.basefile.read_bytes(), self.original_baseline)

    def test_missing_selected_result_is_checked_for_scan_too(self):
        self.runner.args.command = "scan"
        self.select([self.new])
        self.prepare_run(lambda selected: None)
        with self.assertRaises((RuntimeError, ValueError)):
            self.runner.run()


class SelectionCLITests(unittest.TestCase):
    def test_check_and_list_reject_both_selection_flags_before_toolchain_setup(self):
        for command in ["check", "list"]:
            for flag in ["--select", "--filter"]:
                with self.subTest(command=command, flag=flag):
                    result = subprocess.run([sys.executable, str(Path(run.__file__)), command, flag, "unused"],
                                            capture_output=True, text=True, timeout=5)
                    self.assertEqual(result.returncode, 2, result.stderr)
                    self.assertIn(flag, result.stderr)
                    self.assertNotIn("unrecognized arguments", result.stderr)
                    self.assertNotIn("rustc", result.stdout)

    def test_select_and_filter_are_mutually_exclusive(self):
        result = subprocess.run([sys.executable, str(Path(run.__file__)), "scan",
                                 "--select", "unused.json", "--filter", "candidate"],
                                capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("not allowed with argument", result.stderr)

    def test_scan_and_promote_accept_select_without_starting_real_work(self):
        for command in ["scan", "promote"]:
            with self.subTest(command=command), \
                    patch.object(sys, "argv", ["run.py", command, "--select", "candidates.json"]), \
                    patch.dict(run.os.environ, {"OXIDE_FRONTEND": "/unused/frontend"}), \
                    patch.object(run, "Runner") as constructor:
                self.assertEqual(run.main(), 0)
                arguments = constructor.call_args.args[0]
                self.assertEqual(arguments.select, Path("candidates.json"))
                self.assertEqual(arguments.command, command)
                constructor.return_value.run.assert_called_once_with()


if __name__ == "__main__":
    unittest.main()
