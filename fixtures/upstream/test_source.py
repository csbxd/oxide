"""Protect upstream assertions and compiler-generated test discovery tokens."""

import unittest

from source import instrument_tests


CASE = "oxide_upstream_macros::case"


class SourceInstrumentationTests(unittest.TestCase):
    def test_only_the_test_attribute_changes(self):
        source = '#[test]\n#[should_panic(expected = "bad")]\nfn example() { panic!("bad"); }\n'
        self.assertEqual(instrument_tests(source), source.replace("#[test]", f"#[{CASE}]"))

    def test_attribute_trivia_and_line_numbers_are_preserved(self):
        source = "# /* before */ [\n // attribute\n test /* nested /* test */ comment */\n]\nfn ok() {}"
        expected = source.replace(" test /* nested", f" {CASE} /* nested")
        actual = instrument_tests(source)
        self.assertEqual(actual, expected)
        self.assertEqual(actual.count("\n"), source.count("\n"))

    def test_line_and_nested_block_comments_are_opaque(self):
        comments = """// #[test]
/// #[test]
//! #[test]
/* #[test] /* #[test] */ #[test] */
/** #[test] */
/*! #[test] */
"""
        self.assertEqual(instrument_tests(comments + "#[test] fn ok() {}"), comments + f"#[{CASE}] fn ok() {{}}")

    def test_strings_bytes_c_strings_and_raw_strings_are_opaque(self):
        literals = [
            r'"#[test]"',
            r'"escaped \" #[test] \\"',
            '"multiline\n#[test]\n"',
            r'b"#[test]"',
            r'c"#[test]"',
            r'r"#[test]"',
            r'r#"#[test] \\"#',
            r'r###"#[test] "# "## #[test]"###',
            r'br"#[test]"',
            r'br##"#[test] "#"##',
            r'cr#"#[test]"#',
        ]
        for literal in literals:
            with self.subTest(literal=literal):
                source = f"const TEXT: &str = {literal};\n#[test] fn ok() {{}}"
                expected = f"const TEXT: &str = {literal};\n#[{CASE}] fn ok() {{}}"
                self.assertEqual(instrument_tests(source), expected)

    def test_doc_attribute_literals_are_opaque(self):
        source = '#[doc = "#[test]"]\n#[doc = r##"#[test]"##]\n#[test] fn ok() {}'
        self.assertEqual(instrument_tests(source), source.rsplit("#[test]", 1)[0] + f"#[{CASE}] fn ok() {{}}")

    def test_character_and_byte_literals_do_not_start_strings_or_comments(self):
        characters = [r"'\''", r"'\\'", r"'\x27'", r"'\u{0027}'", "'\"'", "'/'", "'🦀'", r"b'\''", "b'\"'"]
        for literal in characters:
            with self.subTest(literal=literal):
                source = f"const C: char = {literal};\n#[test] fn ok() {{}}"
                self.assertEqual(instrument_tests(source), source.replace("#[test]", f"#[{CASE}]"))

    def test_lifetimes_and_labels_do_not_hide_attributes(self):
        source = """fn borrow<'a, 'b: 'a>(a: &'a str, b: &'b str) -> &'a str { a }
fn label() { 'outer: loop { break 'outer; } }
fn anonymous(_: &'_ str) {}
#[test] fn ok() {}
"""
        self.assertEqual(instrument_tests(source), source.replace("#[test]", f"#[{CASE}]"))

    def test_macro_definition_and_invocation_tokens_are_instrumented(self):
        source = """macro_rules! tests {
    ($($name:ident),*) => { $( #[test] fn $name() { assert!(true); } )* };
}
tests!(first, second);
test_runtime_and_compiletime! { #[test] fn third() {} }
"""
        self.assertEqual(instrument_tests(source), source.replace("#[test]", f"#[{CASE}]"))

    def test_unrelated_attributes_and_inner_attributes_are_unchanged(self):
        source = """#![test]
#[test_case]
#[test::attribute]
#[something::test]
#[test()]
#[test = "value"]
#[cfg(test)]
#[cfg_attr(test, ignore = "#[test]")]
#[cfg_attr(test, derive(test))]
fn example() {}
"""
        self.assertEqual(instrument_tests(source), source)

    def test_raw_test_identifier_is_instrumented(self):
        self.assertEqual(instrument_tests("#[r#test] fn ok() {}"), f"#[{CASE}] fn ok() {{}}")

    def test_conditional_test_attributes_fail_explicitly(self):
        attributes = [
            "#[cfg_attr(feature = \"x\", test)]",
            "#[cfg_attr(all(unix, feature = \"x\"), ignore, test,)]",
            "#[cfg_attr(unix, cfg_attr(test, test))]",
            "#[cfg_attr(\n unix, /* reason */ test\n)]",
            "#[cfg_attr(unix, r#test)]",
        ]
        for attribute in attributes:
            with self.subTest(attribute=attribute):
                with self.assertRaisesRegex(ValueError, r"cfg_attr\(\.\.\., test\).*line 2"):
                    instrument_tests("// header\n" + attribute + " fn conditional() {}")

    def test_conditional_ignore_and_nested_non_test_attributes_are_unchanged(self):
        source = """#[cfg_attr(not(panic = "unwind"), ignore = "test requires unwinding")]
#[cfg_attr(test, cfg_attr(unix, allow(unused)))]
#[cfg_attr(test, doc = "test", derive(test))]
#[test] fn ok() {}
"""
        self.assertEqual(instrument_tests(source), source.replace("#[test]", f"#[{CASE}]"))

    def test_empty_input_and_repeated_instrumentation(self):
        self.assertEqual(instrument_tests(""), "")
        once = instrument_tests("#[test] fn ok() {}")
        self.assertEqual(instrument_tests(once), once)

    def test_unterminated_opaque_regions_fail_instead_of_damaging_source(self):
        for source in ['"#[test]', 'br##"#[test]', "/* nested /* #[test] */"]:
            with self.subTest(source=source):
                with self.assertRaisesRegex(ValueError, "unterminated Rust"):
                    instrument_tests(source)


if __name__ == "__main__":
    unittest.main()
