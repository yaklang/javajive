"""T01-C02 dual ledger: historical fail is never a pass; promotion is reviewed."""

from __future__ import annotations

import unittest

from common import PR_BASE, digest, obs
from tools.milestone_ledger import CorrectnessLedger, DefectLedger, ReviewRecord
from tools.milestone_ledger.errors import ExpectationChangeError, PromotionError


class TestTaskT01C02DualLedgerPromotion(unittest.TestCase):
    def test_T01_C02_fail_then_pass_retains_defect_and_promotes_gate(self) -> None:
        input_hash = digest(b"T01-C02-same-input-hash")
        fail_obs = obs(
            sample="PromoteMe",
            mode="precision",
            debug="debug",
            status="fail",
            input_hash=input_hash,
            notes="T01-C02 historical fail",
        )
        pass_obs = obs(
            sample="PromoteMe",
            mode="precision",
            debug="debug",
            status="pass",
            input_hash=input_hash,
            notes="T01-C02 later pass on same hash",
        )
        defects = DefectLedger()
        correctness = CorrectnessLedger(defects)
        correctness.ingest(fail_obs)
        self.assertTrue(
            defects.has_historical_fail(fail_obs.case_id, input_hash),
            "T01-C02 defect ledger records the fail",
        )
        self.assertEqual(defects.as_pass_count(), 0, "T01-C02 historical fail is never a pass")
        self.assertNotEqual(defects.history(fail_obs.case_id, input_hash)[0]["status"], "pass", "T01-C02")
        self.assertFalse(correctness.is_gated(fail_obs.case_id, input_hash), "T01-C02 not yet gated")
        self.assertFalse(correctness.was_always_pass(fail_obs.case_id, input_hash), "T01-C02")

        with self.assertRaises(PromotionError) as silent:
            correctness.ingest(pass_obs)
        self.assertIn("review", str(silent.exception).lower(), "T01-C02 silent promotion refused")

        review = ReviewRecord(
            case_id=pass_obs.case_id,
            reviewer="independent-reviewer",
            action="promotion",
            from_status="fail",
            to_status="pass",
            reason="T01-C02 same input hash now has execution evidence of a real pass",
            timestamp="2026-09-21T00:00:00Z",
            revision=PR_BASE,
            input_hash=input_hash,
        )
        promoted = correctness.promote(pass_obs, review)
        self.assertEqual(promoted["action"], "promoted", "T01-C02")
        self.assertTrue(promoted["defect_history_retained"], "T01-C02")
        self.assertTrue(correctness.is_gated(pass_obs.case_id, input_hash), "T01-C02 correctness now gates it")
        self.assertTrue(
            defects.has_historical_fail(fail_obs.case_id, input_hash),
            "T01-C02 defect history retained after promotion",
        )
        self.assertGreaterEqual(len(defects.history(fail_obs.case_id, input_hash)), 1, "T01-C02")
        self.assertFalse(
            correctness.was_always_pass(pass_obs.case_id, input_hash),
            "T01-C02 querying was this always pass? is false",
        )
        self.assertEqual(correctness.evaluate(pass_obs), "pass", "T01-C02 current strict gate is a real pass")
        self.assertEqual(correctness.evaluate(fail_obs), "fail", "T01-C02 historical fail still evaluates fail")

    def test_T01_C02_expectation_change_requires_review(self) -> None:
        input_hash = digest(b"T01-C02-expectation")
        sample = obs(sample="ExpectMe", status="pass", input_hash=input_hash)
        correctness = CorrectnessLedger()
        correctness.ingest(sample)
        with self.assertRaises(ExpectationChangeError) as ctx:
            correctness.change_expectation(sample.case_id, input_hash, "fail", review=None)
        self.assertIn("review", str(ctx.exception).lower(), "T01-C02")


if __name__ == "__main__":
    unittest.main()
