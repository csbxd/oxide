"""Only explicit common support dependencies may block an entire batch."""

import copy
import json
from pathlib import Path
import tempfile
import unittest

from shared_failure import shared_failure


def function(symbol, name=None, calls=None, asserts=None):
    return {"symbol": symbol, "name": name or symbol, "calls": calls or {}, "assert_calls": asserts or {}}


class SharedFailureTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.directory = Path(temporary.name)
        self.mir = self.directory / "oxide.mir.json"
        self.log = self.directory / "emit.log"
        self.log.write_text("$ ['oxide', 'emit']\nallocator::bad: external/intrinsic body requires lowering (Item)\n")
        self.program = {
            "process_exit_symbol": "exit",
            "functions": [function("root1"), function("root2"), function("exit", calls={"2": "shared"}),
                          function("shared", asserts={"7": {"symbol": "bad"}}), function("bad", "allocator::bad")],
            "roots": [{"name": "suite::first", "symbol": "root1", "params": [], "return": 10},
                      {"name": "suite::second", "symbol": "root2", "params": [], "return": 10}],
            "types": [{"id": 10, "kind": "u8", "size": 1, "sized": True}],
            "api_types": [{"id": 10, "kind": "u8", "size": 1, "sized": True, "display_symbol": "shared"}],
        }

    def proof(self):
        self.mir.write_text(json.dumps(self.program))
        return shared_failure(self.mir, self.log)

    def test_process_exit_path_records_each_direct_call_and_assert_edge(self):
        proof = self.proof()
        self.assertEqual(proof["anchor"], {"kind": "process_exit_symbol", "symbol": "exit"})
        self.assertEqual(proof["failed_function"], {"name": "allocator::bad", "symbol": "bad"})
        self.assertEqual([node["symbol"] for node in proof["path"]], ["exit", "shared", "bad"])
        self.assertEqual(proof["edges"], [
            {"kind": "calls", "block": "2", "caller": "exit", "callee": "shared"},
            {"kind": "assert_calls", "block": "7", "caller": "shared", "callee": "bad"},
        ])
        self.assertEqual(proof["roots"], ["suite::first", "suite::second"])

    def test_proof_edges_can_be_checked_against_original_metadata(self):
        proof = self.proof()
        by_symbol = {item["symbol"]: item for item in self.program["functions"]}
        for edge in proof["edges"]:
            original = by_symbol[edge["caller"]][edge["kind"]][edge["block"]]
            self.assertEqual(original if edge["kind"] == "calls" else original["symbol"], edge["callee"])

    def test_shared_u8_return_display_is_an_anchor_without_process_exit(self):
        self.program["process_exit_symbol"] = ""
        proof = self.proof()
        self.assertEqual(proof["anchor"], {"kind": "shared_u8_return_api", "type": 10,
                                           "member": "display_symbol", "symbol": "shared"})

    def test_shared_u8_return_debug_format_is_an_anchor(self):
        self.program["process_exit_symbol"] = ""
        api = self.program["api_types"][0]
        del api["display_symbol"]
        api["debug"] = {"format_symbol": "shared"}
        self.assertEqual(self.proof()["anchor"]["member"], "debug.format_symbol")

    def test_api_anchor_requires_every_root_to_have_the_same_unit_to_u8_signature(self):
        self.program["process_exit_symbol"] = ""
        original = copy.deepcopy(self.program)
        for change in [{"params": [10]}, {"return": 11}, {"params": None}]:
            with self.subTest(change=change):
                self.program = copy.deepcopy(original)
                self.program["roots"][1].update(change)
                self.assertIsNone(self.proof())

    def test_other_public_types_and_unsupported_api_members_do_not_prove_shared_support(self):
        self.program["process_exit_symbol"] = ""
        original = copy.deepcopy(self.program)
        for change in [{"id": 11}, {"kind": "u16"}, {"size": 2}, {"sized": False},
                       {"display_symbol": "", "invented_symbol": "shared"}]:
            with self.subTest(change=change):
                self.program = copy.deepcopy(original)
                self.program["api_types"][0].update(change)
                self.assertIsNone(self.proof())

    def test_process_exit_proof_is_independent_of_root_signature(self):
        self.program["roots"][1]["params"] = [10]
        self.assertEqual(self.proof()["anchor"]["kind"], "process_exit_symbol")

    def test_function_name_must_resolve_uniquely(self):
        self.program["functions"].append(function("bad2", "allocator::bad"))
        self.assertIsNone(self.proof())

    def test_multiple_named_diagnostics_are_not_conflated(self):
        self.log.write_text("allocator::bad: error\nshared: another error\n")
        self.assertIsNone(self.proof())

    def test_command_header_and_unknown_function_names_do_not_count(self):
        for diagnostic in ["$ ['allocator::bad: error']\nunrelated error\n", "unknown::bad: error\n"]:
            with self.subTest(diagnostic=diagnostic):
                self.log.write_text(diagnostic)
                self.assertIsNone(self.proof())

    def test_indirect_calls_and_allocation_references_are_not_edges(self):
        shared = self.program["functions"][3]
        shared["assert_calls"] = {}
        shared["calls"] = {"0": "<indirect>"}
        shared["body"] = {"constant": {"allocation": 99}}
        self.program["allocations"] = [{"id": 99, "function": "bad"}]
        self.assertIsNone(self.proof())

    def test_reachability_from_only_a_test_root_cannot_block_the_batch(self):
        self.program["process_exit_symbol"] = ""
        self.program["api_types"] = []
        self.program["functions"][0]["calls"] = {"0": "bad"}
        self.assertIsNone(self.proof())

    def test_cycles_terminate_without_inventing_reachability(self):
        self.program["functions"][3]["assert_calls"] = {}
        self.program["functions"][3]["calls"] = {"0": "exit"}
        self.assertIsNone(self.proof())

    def test_empty_roots_or_duplicate_function_symbols_are_rejected(self):
        original = copy.deepcopy(self.program)
        self.program["roots"] = []
        self.assertIsNone(self.proof())
        self.program = original
        self.program["functions"].append(function("bad", "other::name"))
        self.assertIsNone(self.proof())

    def test_missing_or_malformed_inputs_fall_back(self):
        self.assertIsNone(shared_failure(self.mir, self.log))
        self.mir.write_text("not JSON")
        self.assertIsNone(shared_failure(self.mir, self.log))


if __name__ == "__main__":
    unittest.main()
