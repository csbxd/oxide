"""Resource failures abort measurement and cannot weaken regression baselines."""

from contextlib import redirect_stdout
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import Mock, patch

from run import CommandControl, CommandFailure, InfrastructureFailure, command
from test_parallel import add_case, runner_fixture


DISK_FULL = "open('/dev/full', 'wb', buffering=0).write(b'one byte')"


class ResourceFailureTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.directory = Path(temporary.name)

    def execute(self, script, *, control=None, log=None):
        return command([sys.executable, "-c", script], cwd=self.directory,
                       env=os.environ.copy(), log=log or self.directory / "command.log",
                       stage="go-build", timeout=5, control=control)

    def test_real_enospc_in_child_aborts_and_kills_other_active_commands(self):
        control = CommandControl()
        peer = control.start([sys.executable, "-c", "import time; time.sleep(60)"],
                             stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        try:
            with self.assertRaisesRegex(InfrastructureFailure, "go-build.*no space left on device"):
                self.execute(DISK_FULL, control=control)
            self.assertTrue(control.cancelled.is_set())
            self.assertLess(peer.wait(timeout=5), 0, "resource failure did not stop the peer command")
        finally:
            control.cancel()
            peer.wait(timeout=5)
            control.finished(peer)

    def test_real_enospc_while_opening_the_log_never_starts_the_command(self):
        control = CommandControl()
        with patch("run.subprocess.Popen") as popen:
            with self.assertRaises(InfrastructureFailure):
                self.execute("raise SystemExit(0)", control=control, log=Path("/dev/full"))
        popen.assert_not_called()
        self.assertTrue(control.cancelled.is_set())

    def test_command_header_is_not_an_infrastructure_diagnostic(self):
        with self.assertRaises(CommandFailure):
            self.execute("unused = 'no space left on device'; raise SystemExit(1)")

    def test_successful_test_output_is_not_reclassified_as_infrastructure_failure(self):
        output = self.execute("print('no space left on device')")
        self.assertIn("no space left on device", output)

    def test_other_explicit_resource_diagnostics_abort_failed_commands(self):
        for message in ["disk quota exceeded", "too many open files", "cannot allocate memory",
                        "resource temporarily unavailable", "failed to create new OS thread"]:
            with self.subTest(message=message):
                with self.assertRaises(InfrastructureFailure):
                    self.execute(f"import sys; print({message!r}, file=sys.stderr); sys.exit(1)")

    def test_enospc_does_not_bisect_record_a_test_failure_or_save_a_baseline(self):
        runner = runner_fixture()
        runner.work = self.directory / "run"
        runner.work.mkdir()
        runner.env["CARGO_TARGET_DIR"] = str(self.directory / "cargo-target")
        runner.args.keep_generated = False
        runner.batch_number = 0
        ids = [add_case(runner, "old_success"), add_case(runner, "new_candidate")]
        for case_id in ids:
            runner.cases[case_id]["root"] = "coretests::" + case_id.split("/")[1]
        runner.base = {"schema": 1, "context": {}, "passed": [ids[0]], "discovered": ids}
        runner.basefile = self.directory / "baseline.json"
        runner.basefile.write_text(json.dumps(runner.base))
        before = runner.basefile.read_bytes()
        runner.prepare, runner.discover = Mock(), Mock()
        runner.native_check = Mock(return_value=True)
        runner.cargo_args = Mock(return_value=["cargo", "rustc"])
        exports = []

        def fail_export(args, **options):
            if options["stage"] == "oxide-build":
                return ""
            self.assertEqual(options["stage"], "export")
            exports.append(args)
            return command([sys.executable, "-c", DISK_FULL], control=runner.control, **options)

        runner.command = fail_export
        with self.assertRaises(InfrastructureFailure):
            runner.run()
        self.assertEqual(len(exports), 1)
        self.assertTrue(runner.control.cancelled.is_set())
        self.assertEqual(runner.results, {case_id: "not_run" for case_id in ids})
        self.assertEqual(runner.basefile.read_bytes(), before)


class StorageIsolationTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.directory = Path(temporary.name)

    def test_go_storage_overrides_ambient_tmp_without_changing_context(self):
        first, second = runner_fixture(), runner_fixture()
        for index, runner in enumerate((first, second)):
            runner.cache = self.directory / "cache"
            runner.work = self.directory / f"run-{index}"
            runner.context = {"target": "aarch64", "source": "pinned"}
            runner.env.update(GOCACHE="/tmp/global-go-cache", GOTMPDIR="/tmp/global-go-temp")
            runner.configure_go_storage()
            self.assertEqual(runner.context, {"target": "aarch64", "source": "pinned"})
            for variable in ("GOCACHE", "GOTMPDIR"):
                self.assertTrue(Path(runner.env[variable]).is_dir())
                self.assertTrue(Path(runner.env[variable]).is_relative_to(self.directory))
        self.assertEqual(first.env["GOCACHE"], second.env["GOCACHE"])
        self.assertNotEqual(first.env["GOTMPDIR"], second.env["GOTMPDIR"])

    def test_cleanup_only_removes_new_test_crate_directories(self):
        runner = runner_fixture()
        target = self.directory / "cargo-target"
        parent = target / runner.triple / "debug/build/coretests"
        oracle = parent / "native-oracle/out/native"
        oracle.parent.mkdir(parents=True)
        oracle.write_text("keep native executable")
        dependency = target / runner.triple / "debug/build/core/dependency/out/libcore.rlib"
        dependency.parent.mkdir(parents=True)
        dependency.write_text("keep dependency")
        diagnostic = self.directory / "run/batch-00001/export.log"
        diagnostic.parent.mkdir(parents=True)
        diagnostic.write_text("keep diagnostics")
        before = runner.test_artifact_snapshot("coretests", target)
        new = parent / "unique-batch/out/native"
        new.parent.mkdir(parents=True)
        new.write_text("new native output")
        external = self.directory / "external"
        external.mkdir()
        sentinel = external / "preserved"
        sentinel.write_text("do not follow symlink")
        (parent / "linked-directory").symlink_to(external, target_is_directory=True)
        runner.clean_test_artifacts(before)
        self.assertFalse((parent / "unique-batch").exists())
        self.assertTrue(oracle.is_file())
        self.assertTrue(dependency.is_file())
        self.assertTrue(diagnostic.is_file())
        self.assertTrue(sentinel.is_file())

    def test_batch_releases_its_native_output_before_emit_and_recursive_retry(self):
        runner = runner_fixture()
        runner.work = self.directory / "run"
        runner.work.mkdir()
        target = self.directory / "cargo-target"
        runner.env["CARGO_TARGET_DIR"] = str(target)
        parent = target / runner.triple / "debug/build/coretests"
        oracle = parent / "native-oracle"
        oracle.mkdir(parents=True)
        runner.args.keep_generated = False
        runner.batch_number = 0
        ids = [add_case(runner, "first"), add_case(runner, "second")]
        for case_id in ids:
            runner.cases[case_id]["root"] = "coretests::" + case_id.split("/")[1]
        runner.cargo_args = Mock(return_value=["cargo", "rustc"])
        exports = []

        def compile_then_fail(args, *, env, log, stage, **options):
            self.assertEqual({entry.name for entry in parent.iterdir()}, {"native-oracle"})
            if stage == "export":
                exports.append(args)
                native = parent / f"batch-{len(exports)}" / "out/native"
                native.parent.mkdir(parents=True)
                native.write_text("temporary native output")
                mir = Path(next(str(arg).removeprefix("--oxide-export=") for arg in args
                                if str(arg).startswith("--oxide-export=")))
                mir.write_text("{}")
                roots = [{"name": root} for root in env["OXIDE_ROOTS"].split(",")]
                mir.with_suffix(".api.json").write_text(json.dumps({"roots": roots}))
                return ""
            self.assertEqual(stage, "emit")
            Path(log).write_text("unsupported non-shared operation\n")
            raise CommandFailure(stage, Path(log))

        runner.command = compile_then_fail
        with redirect_stdout(io.StringIO()):
            runner.batch("coretests", ids)
        self.assertEqual(len(exports), 3)
        self.assertEqual({entry.name for entry in parent.iterdir()}, {"native-oracle"})
        self.assertEqual(runner.results, {case_id: "emit_failed" for case_id in ids})


if __name__ == "__main__":
    unittest.main()
