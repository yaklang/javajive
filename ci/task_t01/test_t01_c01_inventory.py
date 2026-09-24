"""T01-C01 inventory conservation: missing IDs are named; counts cannot hide swaps."""

from __future__ import annotations

import unittest

from common import eight_api_pass, observation_dict, obs
from tools.milestone_ledger import aggregate
from tools.milestone_ledger.errors import InventoryError


class TestTaskT01C01InventoryConservation(unittest.TestCase):
    def test_T01_C01_drop_one_names_missing_id(self) -> None:
        rows, manifest = eight_api_pass()
        self.assertEqual(len(rows), 8, "T01-C01 expected 2 cases × 2 modes × 2 debug = 8")
        dropped = rows[5]
        remaining = rows[:5] + rows[6:]
        result = aggregate(manifest, remaining, strict=False)
        self.assertFalse(result.ok, "T01-C01")
        self.assertIn(dropped.case_id, result.missing_ids, "T01-C01 missing ID must be named")
        self.assertTrue(
            any(f"MISSING ID: {dropped.case_id}" in err for err in result.errors),
            "T01-C01 aggregator must name MISSING ID " + dropped.case_id,
        )
        with self.assertRaises(InventoryError) as ctx:
            aggregate(manifest, remaining, strict=True)
        self.assertIn(dropped.case_id, ctx.exception.missing_ids, "T01-C01")
        self.assertIn(dropped.case_id, str(ctx.exception), "T01-C01")

    def test_T01_C01_duplicate_id_fails(self) -> None:
        rows, manifest = eight_api_pass()
        duplicated = rows + [rows[0]]
        result = aggregate(manifest, duplicated, strict=False)
        self.assertFalse(result.ok, "T01-C01")
        self.assertIn(rows[0].case_id, result.duplicate_ids, "T01-C01 duplicate ID must be named")
        self.assertTrue(
            any(f"DUPLICATE ID: {rows[0].case_id}" in err for err in result.errors),
            "T01-C01",
        )
        self.assertEqual(result.metrics["T01-M01"]["verdict"], "fail", "T01-C01")

    def test_T01_C01_count_equal_swapped_id_fails(self) -> None:
        rows, manifest = eight_api_pass()
        victim = rows[-1]
        swapped = rows[:-1] + [obs(sample="Gamma", mode=victim.evidence.mode, debug=victim.evidence.debug)]
        self.assertEqual(len(swapped), 8, "T01-C01 equal counts must not hide a swap")
        result = aggregate(manifest, swapped, strict=False)
        self.assertFalse(result.ok, "T01-C01")
        self.assertIn(victim.case_id, result.missing_ids, "T01-C01 swapped-out ID is missing")
        self.assertTrue(
            any(f"MISSING ID: {victim.case_id}" in err for err in result.errors),
            "T01-C01 equal count still names MISSING ID",
        )
        self.assertTrue(result.unexpected_ids, "T01-C01")

    def test_T01_C01_corrupt_json_fails_aggregation(self) -> None:
        _, manifest = eight_api_pass()
        result = aggregate(manifest, "{not json", strict=False)
        self.assertFalse(result.ok, "T01-C01")
        self.assertTrue(any("CORRUPT_JSON" in err for err in result.errors), "T01-C01 corrupt JSON")

    def test_T01_C01_empty_ledger_fails(self) -> None:
        rows, manifest = eight_api_pass()
        result = aggregate(manifest, [], strict=False)
        self.assertFalse(result.ok, "T01-C01")
        self.assertEqual(sorted(result.missing_ids), sorted(o.case_id for o in rows), "T01-C01")
        self.assertTrue(any("EMPTY_RESULTS" in err for err in result.errors), "T01-C01")
        for cid in result.missing_ids:
            self.assertTrue(any(f"MISSING ID: {cid}" in err for err in result.errors), "T01-C01")

    def test_T01_C01_missing_mode_fails(self) -> None:
        rows, manifest = eight_api_pass()
        broken = observation_dict(sample="CaseA", mode="precision", debug="debug")
        del broken["evidence"]["mode"]
        payload = [broken] + [row.to_dict() for row in rows[1:]]
        result = aggregate(manifest, payload, strict=False)
        self.assertFalse(result.ok, "T01-C01")
        self.assertTrue(any("MISSING_MODE" in err for err in result.errors), "T01-C01 missing mode")

    def test_T01_C01_xfail_counted_as_pass_fails(self) -> None:
        rows, manifest = eight_api_pass()
        bad = observation_dict(sample="CaseA", mode="precision", debug="debug", status="pass", xfail=True)
        result = aggregate(manifest, [bad] + [row.to_dict() for row in rows[1:]], strict=False)
        self.assertFalse(result.ok, "T01-C01")
        blob = " ".join(result.errors).lower()
        self.assertTrue("xfail" in blob or "pass" in blob, "T01-C01 xfail must not count as pass")
        self.assertNotIn("CaseA::api::precision::debug", result.pass_ids, "T01-C01")

    def test_T01_C01_full_eight_ok(self) -> None:
        rows, manifest = eight_api_pass()
        result = aggregate(manifest, rows, strict=True)
        self.assertTrue(result.ok, "T01-C01")
        self.assertEqual(result.missing_ids, [], "T01-C01")
        self.assertEqual(result.duplicate_ids, [], "T01-C01")
        self.assertEqual(result.metrics["T01-M01"]["observed"]["coverage"], 1.0, "T01-C01")


if __name__ == "__main__":
    unittest.main()
