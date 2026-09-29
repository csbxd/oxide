"""Check Cargo argument preservation and actual exec/exit propagation."""

import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import unittest

from export_wrapper import metadata_export_arguments


WRAPPER = Path(__file__).with_name("export_wrapper.py")


class ExportArgumentsTests(unittest.TestCase):
    def test_non_export_cargo_invocations_are_unchanged(self):
        for arguments in (
            ["/pinned/bin/rustc", "-vV"],
            ["/pinned/bin/rustc", "--print", "file-names", "--emit=dep-info,link"],
            ["/pinned/bin/rustc", "--emit", "dep-info,link", "dependency.rs"],
            ["/pinned/bin/rustc", "--emit"],
            ["/pinned/bin/rustc", "--oxide-export-other=value", "--emit=link"],
            ["/pinned/bin/rustc", "/source/--oxide-export=file.rs", "--emit=link"],
        ):
            with self.subTest(arguments=arguments):
                self.assertEqual(metadata_export_arguments(arguments), arguments)

    def test_duplicate_equals_and_separate_emit_modes_are_removed(self):
        arguments = ["/pinned/bin/rustc", "--emit=dep-info,link", "--test",
                     "--emit", "metadata", "--emit=asm", "--oxide-export=/new/mir.json"]
        self.assertEqual(metadata_export_arguments(arguments),
                         ["/pinned/bin/rustc", "--test", "--oxide-export=/new/mir.json", "--emit=metadata"])

    def test_all_other_arguments_and_their_order_are_preserved(self):
        preserved = ["/pinned/bin/rustc", "--crate-name", "coretests", "source with spaces.rs",
                     "--edition=2024", "--test", "--target", "aarch64-unknown-linux-gnu",
                     "-C", "panic=unwind", "-Coverflow-checks=yes", "-Copt-level=0",
                     "-Zmir-opt-level=0", "-Zalways-encode-mir", "--cfg", "test",
                     "--extern", "alloc=/path with spaces/liballoc.rlib", "--out-dir", "/output dir",
                     "--error-format=json", "--emit-other=value", "--oxide-export=/new/mir.json"]
        arguments = preserved[:5] + ["--emit=dep-info,link"] + preserved[5:]
        self.assertEqual(metadata_export_arguments(arguments), preserved + ["--emit=metadata"])

    def test_export_without_emit_still_requests_metadata(self):
        arguments = ["rustc", "--oxide-export=/new/mir.json", "input.rs"]
        self.assertEqual(metadata_export_arguments(arguments), arguments + ["--emit=metadata"])

    def test_input_arguments_are_not_mutated(self):
        arguments = ["rustc", "--emit=link", "--oxide-export=/new/mir.json"]
        before = list(arguments)
        metadata_export_arguments(arguments)
        self.assertEqual(arguments, before)

    def test_malformed_separate_emit_cannot_swallow_another_flag(self):
        for arguments in (
            ["rustc", "--oxide-export=/new/mir.json", "--emit"],
            ["rustc", "--emit", "--crate-name", "test", "--oxide-export=/new/mir.json"],
        ):
            with self.subTest(arguments=arguments), self.assertRaisesRegex(ValueError, "--emit requires a value"):
                metadata_export_arguments(arguments)


class ExportExecTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="oxide-export-wrapper-")
        self.addCleanup(self.temporary.cleanup)
        self.frontend = Path(self.temporary.name) / "frontend"
        self.frontend.write_text(
            f"#!{sys.executable}\n"
            "import json, os, signal, sys\n"
            "print(json.dumps({'pid': os.getpid(), 'argv': sys.argv}), flush=True)\n"
            "if os.environ.get('FAKE_FRONTEND_SIGNAL'):\n"
            "    os.kill(os.getpid(), signal.SIGTERM)\n"
            "sys.exit(int(os.environ.get('FAKE_FRONTEND_EXIT', '0')))\n"
        )
        self.frontend.chmod(0o755)
        self.env = os.environ | {"OXIDE_UPSTREAM_FRONTEND": str(self.frontend)}
        self.env.pop("FAKE_FRONTEND_EXIT", None)
        self.env.pop("FAKE_FRONTEND_SIGNAL", None)

    def invoke(self, arguments, **environment):
        process = subprocess.Popen([sys.executable, str(WRAPPER), *arguments],
                                   env=self.env | environment, text=True,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        stdout, stderr = process.communicate(timeout=10)
        return process, stdout, stderr

    def test_exec_preserves_pid_and_non_export_arguments(self):
        arguments = ["/pinned/rustc", "--crate-name", "dep", "--emit", "dep-info,link"]
        process, stdout, stderr = self.invoke(arguments)
        self.assertEqual(process.returncode, 0, stderr)
        record = json.loads(stdout)
        self.assertEqual(record["pid"], process.pid)
        self.assertEqual(record["argv"], [str(self.frontend), *arguments])

    def test_real_export_exec_passes_only_one_emit_mode(self):
        arguments = ["/pinned/rustc", "--emit=dep-info,link", "source.rs", "--emit", "metadata",
                     "--oxide-export=/new/mir.json", "-Coverflow-checks=yes"]
        process, stdout, stderr = self.invoke(arguments)
        self.assertEqual(process.returncode, 0, stderr)
        self.assertEqual(json.loads(stdout)["argv"],
                         [str(self.frontend), "/pinned/rustc", "source.rs", "--oxide-export=/new/mir.json",
                          "-Coverflow-checks=yes", "--emit=metadata"])

    def test_frontend_error_exit_code_is_not_hidden(self):
        for arguments in (["/pinned/rustc", "-vV"],
                          ["/pinned/rustc", "--emit=link", "--oxide-export=/new/mir.json"]):
            with self.subTest(arguments=arguments):
                process, _, _ = self.invoke(arguments, FAKE_FRONTEND_EXIT="37")
                self.assertEqual(process.returncode, 37)

    @unittest.skipUnless(os.name == "posix", "POSIX process signal semantics")
    def test_frontend_signal_is_preserved(self):
        process, _, _ = self.invoke(["/pinned/rustc", "--oxide-export=/new/mir.json"], FAKE_FRONTEND_SIGNAL="1")
        self.assertEqual(process.returncode, -signal.SIGTERM)

    def test_missing_frontend_setting_fails(self):
        process, _, stderr = self.invoke(["/pinned/rustc", "-vV"], OXIDE_UPSTREAM_FRONTEND="")
        self.assertNotEqual(process.returncode, 0)
        self.assertIn("OXIDE_UPSTREAM_FRONTEND is required", stderr)

    def test_exec_failure_cannot_report_success(self):
        process, _, stderr = self.invoke(["/pinned/rustc", "-vV"],
                                        OXIDE_UPSTREAM_FRONTEND=str(self.frontend.with_name("missing")))
        self.assertNotEqual(process.returncode, 0)
        self.assertIn("upstream export wrapper:", stderr)

    def test_malformed_export_emit_does_not_call_frontend(self):
        process, stdout, stderr = self.invoke(["/pinned/rustc", "--oxide-export=/new/mir.json", "--emit"])
        self.assertNotEqual(process.returncode, 0)
        self.assertEqual(stdout, "")
        self.assertIn("--emit requires a value", stderr)


if __name__ == "__main__":
    unittest.main()
