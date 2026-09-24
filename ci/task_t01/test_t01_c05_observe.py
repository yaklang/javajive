"""T01-C05 observe vs success: exit 0 + failed=9 is never pass/complete."""

from __future__ import annotations

import unittest

import common  # noqa: F401  # repo root on sys.path

from tools.milestone_ledger import classify_task_report


class TestTaskT01C05ObserveVersusSuccess(unittest.TestCase):
    def test_T01_C05_observe_rc0_failed_9_is_not_pass_or_complete(self) -> None:
        report = {
            "policy": "observe",
            "status": "complete",
            "failed": 9,
            "passed": 4,
            "summary": {"checks": 13, "failed": 9, "passed": 4},
            "observations": [{"case": "UnicodePair", "pass": False}] * 9
            + [{"case": "Baseline", "pass": True}] * 4,
        }
        outcome = classify_task_report(0, report)
        self.assertFalse(outcome.is_success, "T01-C05")
        self.assertIn(outcome.status, {"fail", "observe"}, "T01-C05")
        self.assertNotIn(outcome.status, {"pass", "complete"}, "T01-C05")
        self.assertEqual(outcome.failed, 9, "T01-C05")
        self.assertEqual(outcome.process_exit_code, 0, "T01-C05 exit 0 is not a semantic pass")
        self.assertTrue(
            any("exit 0" in reason for reason in outcome.reasons),
            "T01-C05 " + str(outcome.reasons),
        )

    def test_T01_C05_gate_policy_failed_is_fail(self) -> None:
        report = {"policy": "gate", "summary": {"failed": 9, "passed": 4, "checks": 13}}
        outcome = classify_task_report(0, report)
        self.assertEqual(outcome.status, "fail", "T01-C05")
        self.assertFalse(outcome.is_success, "T01-C05")
        self.assertNotEqual(outcome.status, "complete", "T01-C05")


if __name__ == "__main__":
    unittest.main()
