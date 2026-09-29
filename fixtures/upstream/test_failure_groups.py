"""Diagnostic grouping never supplies pass/fail decisions or drops roots."""

from pathlib import Path
import tempfile
import unittest

from failure_groups import failure_groups, named_failure_groups


SOURCE = """package upstream
func f0() {
    bad := 1 / 0
    _ = bad
}
func f1() {
    callback := oxide.FunctionPointer(f0)
    _ = callback
}
func f2() {}
// First translates suite::first.
func First() {
    f1()
}
// Second translates suite::second.
func Second() {
    f2()
}
// Third translates suite::third.
func Third() {
    f1()
}
"""


class FailureGroupsTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.directory = Path(temporary.name)
        self.generated = self.directory / "go"
        self.generated.mkdir()
        self.source = self.generated / "oxide_gen_00000.go"
        self.source.write_text(SOURCE)
        self.log = self.directory / "go-build.log"
        self.log.write_text("./oxide_gen_00000.go:3:14: invalid operation: division by zero\n")
        self.roots = ["suite::third", "suite::second", "suite::first"]

    def test_partitions_in_original_order_including_function_pointer_reference(self):
        self.assertEqual(failure_groups(self.generated, self.log, self.roots),
                         [["suite::third", "suite::first"], ["suite::second"]])

    def test_retains_every_root_exactly_once(self):
        groups = failure_groups(self.generated, self.log, self.roots)
        self.assertCountEqual([root for group in groups for root in group], self.roots)

    def test_repeated_diagnostics_and_absolute_paths(self):
        self.log.write_text(f"{self.source}:3:14: division by zero\n"
                            "./oxide_gen_00000.go:4:10: another diagnostic\n")
        self.assertEqual(failure_groups(self.generated, self.log, self.roots),
                         [["suite::third", "suite::first"], ["suite::second"]])

    def test_references_across_generated_files(self):
        self.source.write_text(SOURCE.replace("func f1() {\n    callback := oxide.FunctionPointer(f0)\n    _ = callback\n}\n", ""))
        (self.generated / "oxide_gen_00001.go").write_text("package upstream\nfunc f1() {\n    f0()\n}\n")
        self.assertEqual(failure_groups(self.generated, self.log, self.roots),
                         [["suite::third", "suite::first"], ["suite::second"]])

    def test_all_suspected_roots_fall_back(self):
        self.assertIsNone(failure_groups(self.generated, self.log, ["suite::first", "suite::third"]))

    def test_unconnected_diagnostic_falls_back(self):
        self.source.write_text(SOURCE + "func unrelated() {\n    bad := 1 / 0\n}\n")
        self.log.write_text(f"./oxide_gen_00000.go:{len(SOURCE.splitlines()) + 2}:14: division by zero\n")
        self.assertIsNone(failure_groups(self.generated, self.log, self.roots))

    def test_unrecognized_diagnostic_location_falls_back(self):
        for log in ["runtime out of memory\n", "upstream_test.go:3:10: unknown test\n",
                    "./oxide_gen_00000.go:1:10: package error\n",
                    "./oxide_gen_99999.go:3:10: missing file\n"]:
            with self.subTest(log=log):
                self.log.write_text(log)
                self.assertIsNone(failure_groups(self.generated, self.log, self.roots))

    def test_missing_or_duplicate_roots_fall_back(self):
        for roots in [[], ["suite::first"], ["suite::first", "suite::first"],
                      ["suite::first", "suite::missing"]]:
            with self.subTest(roots=roots):
                self.assertIsNone(failure_groups(self.generated, self.log, roots))

    def test_unterminated_generated_function_falls_back(self):
        self.source.write_text(SOURCE + "func incomplete() {\n")
        self.assertIsNone(failure_groups(self.generated, self.log, self.roots))

    def test_missing_log_falls_back(self):
        self.log.unlink()
        self.assertIsNone(failure_groups(self.generated, self.log, self.roots))


class NamedFailureGroupsTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.log = Path(temporary.name) / "emit.log"
        self.roots = ["coretests::future::__oxide_upstream_test_a",
                      "coretests::future::__oxide_upstream_test_ab",
                      "coretests::future::__oxide_upstream_test_other"]

    def test_nested_item_partitions_without_go_sources(self):
        self.log.write_text("$ ['oxide', 'emit']\n"
                            "coretests::future::test_a::async_fn::{closure#0}: missing discriminant\n")
        self.assertEqual(named_failure_groups(self.log, self.roots),
                         [[self.roots[0]], self.roots[1:]])

    def test_similarly_prefixed_tests_do_not_match(self):
        self.log.write_text("coretests::future::test_ab: unsupported lowering\n")
        self.assertEqual(named_failure_groups(self.log, self.roots),
                         [[self.roots[1]], [self.roots[0], self.roots[2]]])

    def test_longer_crate_or_module_paths_do_not_match(self):
        for name in ["othercoretests::future::test_a", "other::coretests::future::test_a",
                     "coretests::otherfuture::test_a", "coretests::future::test_abc"]:
            with self.subTest(name=name):
                self.log.write_text(name + ": unsupported lowering\n")
                self.assertIsNone(named_failure_groups(self.log, self.roots))

    def test_command_header_is_not_a_diagnostic(self):
        self.log.write_text("$ ['rustc', 'coretests::future::test_a']\n"
                            "coretests::future::test_ab: unsupported lowering\n")
        self.assertEqual(named_failure_groups(self.log, self.roots),
                         [[self.roots[1]], [self.roots[0], self.roots[2]]])

    def test_diagnostic_without_command_header_is_preserved(self):
        self.log.write_text("oxide-rs: error in <coretests::future::test_a::Value as Trait>\n")
        self.assertEqual(named_failure_groups(self.log, self.roots),
                         [[self.roots[0]], self.roots[1:]])

    def test_multiple_diagnostics_preserve_order_and_every_root(self):
        self.log.write_text("coretests::future::test_other: error\ncoretests::future::test_a: error\n")
        self.assertEqual(named_failure_groups(self.log, self.roots),
                         [[self.roots[0], self.roots[2]], [self.roots[1]]])

    def test_all_or_no_suspects_fall_back(self):
        for text in ["unrelated failure", "\n".join(root.replace("__oxide_upstream_", "") for root in self.roots)]:
            with self.subTest(text=text):
                self.log.write_text(text)
                self.assertIsNone(named_failure_groups(self.log, self.roots))

    def test_missing_log_or_invalid_root_set_falls_back(self):
        self.assertIsNone(named_failure_groups(self.log, self.roots))
        self.log.write_text("coretests::future::test_a: error\n")
        for roots in [[], self.roots[:1], [self.roots[0], self.roots[0]],
                      [self.roots[0], "coretests::future::unadapted"]]:
            with self.subTest(roots=roots):
                self.assertIsNone(named_failure_groups(self.log, roots))


if __name__ == "__main__":
    unittest.main()
