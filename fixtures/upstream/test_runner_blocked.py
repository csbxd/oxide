"""A proven shared emit failure is a diagnostic outcome, never a test pass."""

from contextlib import redirect_stdout
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch

import run
from run import CommandFailure
from test_parallel import add_case, runner_fixture


class SharedBlockerRunnerTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.runner = runner_fixture()
        self.runner.work = Path(temporary.name)
        self.runner.args.keep_generated = False
        self.runner.batch_number = 0
        self.ids = [add_case(self.runner, "first"), add_case(self.runner, "second")]
        for case_id in self.ids:
            self.runner.cases[case_id]["root"] = "coretests::__oxide_upstream_" + case_id.split("/")[1]
        self.runner.cargo_args = Mock(return_value=["cargo", "rustc"])
        self.exports = []
        self.failure_stage = "emit"
        self.timeout = False
        self.has_shared_path = True
        self.runner.command = self.command

    def command(self, args, *, env, log, stage, **kwargs):
        log = Path(log)
        log.parent.mkdir(parents=True, exist_ok=True)
        if stage == "export":
            mir = Path(next(str(arg).removeprefix("--oxide-export=") for arg in args
                            if str(arg).startswith("--oxide-export=")))
            roots = env["OXIDE_ROOTS"].split(",")
            self.exports.append(roots)
            functions = [{"name": root, "symbol": root} for root in roots]
            functions.extend([
                {"name": "support::exit", "symbol": "exit",
                 "calls": {"0": "bad"} if self.has_shared_path else {}},
                {"name": "support::bad", "symbol": "bad"},
            ])
            program = {"process_exit_symbol": "exit", "functions": functions,
                       "roots": [{"name": root, "symbol": root} for root in roots]}
            mir.write_text(json.dumps(program))
            mir.with_suffix(".api.json").write_text(json.dumps({"roots": program["roots"]}))
            if self.failure_stage != "export":
                return ""
        if stage == self.failure_stage:
            log.write_text("support::bad: unsupported body\n")
            raise CommandFailure(stage, log, self.timeout)
        self.fail(f"unexpected build stage {stage}")

    def execute(self):
        with redirect_stdout(io.StringIO()):
            self.runner.batch("coretests", self.ids)

    def test_proven_shared_emit_failure_blocks_without_redundant_exports(self):
        self.execute()
        self.assertEqual(len(self.exports), 1)
        self.assertEqual(self.runner.results, {case_id: "blocked" for case_id in self.ids})
        self.runner.require_selected_results(set(self.ids))
        proof_path = self.runner.work / "batch-00001/shared-failure.log"
        proof = json.loads(proof_path.read_text())
        self.assertEqual(proof["failed_function"]["symbol"], "bad")
        self.assertEqual(set(proof["roots"]), set(self.exports[0]))
        self.assertEqual(proof["edges"][0]["callee"], "bad")
        for case_id in self.ids:
            self.assertIn(str(proof_path), self.runner.details[case_id])
            self.assertIn("emit.log", self.runner.details[case_id])
        self.assertFalse((proof_path.parent / "oxide.mir.json").exists())
        self.assertTrue((proof_path.parent / "emit.log").exists())

    def test_emit_failure_without_shared_proof_still_bisects(self):
        self.has_shared_path = False
        self.execute()
        self.assertEqual(len(self.exports), 3)
        self.assertEqual(self.runner.results, {case_id: "emit_failed" for case_id in self.ids})
        self.assertEqual(list(self.runner.work.rglob("shared-failure.log")), [])

    def test_export_failure_never_uses_emit_shared_proof(self):
        self.failure_stage = "export"
        with patch.object(run, "shared_failure") as proof:
            self.execute()
        proof.assert_not_called()
        self.assertEqual(len(self.exports), 3)
        self.assertEqual(self.runner.results, {case_id: "export_failed" for case_id in self.ids})

    def test_timed_out_emit_falls_back_even_if_log_contains_a_named_error(self):
        self.timeout = True
        with patch.object(run, "shared_failure") as proof:
            self.execute()
        proof.assert_not_called()
        self.assertEqual(len(self.exports), 3)
        self.assertEqual(self.runner.results, {case_id: "emit_timeout" for case_id in self.ids})

    def test_proof_for_different_roots_cannot_block_the_requested_batch(self):
        with patch.object(run, "shared_failure", return_value={"roots": ["unrequested::root"]}):
            self.execute()
        self.assertEqual(len(self.exports), 3)
        self.assertEqual(self.runner.results, {case_id: "emit_failed" for case_id in self.ids})


if __name__ == "__main__":
    unittest.main()
