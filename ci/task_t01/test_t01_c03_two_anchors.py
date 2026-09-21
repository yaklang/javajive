"""T01-C03 two-anchor drift: equal-to-base is not sufficient if milestone lost a capability."""

from __future__ import annotations

import unittest

from common import MILESTONE, PR_BASE, digest, obs
from tools.milestone_ledger import compare_anchors


class TestTaskT01C03TwoAnchorCumulativeDrift(unittest.TestCase):
    def test_T01_C03_base_equals_candidate_but_milestone_capability_lost(self) -> None:
        input_hash = digest(b"T01-C03-capability")
        kwargs = dict(
            sample="EmojiRoundTrip",
            mode="precision",
            debug="debug",
            input_hash=input_hash,
            compiler_version="javac 21.0.11",
            harness_digest=digest(b"T01-C03-harness"),
            dependency_lock_digest=digest(b"T01-C03-lock"),
        )
        milestone = obs(status="pass", revision=MILESTONE, notes="T01-C03 milestone had the capability", **kwargs)
        pr_base = obs(status="fail", revision=PR_BASE, notes="T01-C03 PR base lost it", **kwargs)
        candidate = obs(status="fail", revision=PR_BASE, notes="T01-C03 candidate matches base", **kwargs)
        self.assertEqual(pr_base.status, candidate.status, "T01-C03")
        self.assertEqual(pr_base.evidence.revision, candidate.evidence.revision, "T01-C03")

        stable = dict(
            sample="BaselineStable",
            mode="compatibility",
            debug="nodebug",
            input_hash=digest(b"T01-C03-stable"),
            compiler_version="javac 21.0.11",
            harness_digest=kwargs["harness_digest"],
            dependency_lock_digest=kwargs["dependency_lock_digest"],
        )
        mile_stable = obs(status="pass", revision=MILESTONE, **stable)
        base_stable = obs(status="pass", revision=PR_BASE, **stable)
        cand_stable = obs(status="pass", revision=PR_BASE, **stable)

        result = compare_anchors(
            [candidate, cand_stable],
            [pr_base, base_stable],
            [milestone, mile_stable],
        )
        self.assertTrue(result["equal_to_pr_base"], "T01-C03 candidate equals PR base")
        self.assertTrue(result["long_term_drift"], "T01-C03 long-term drift versus milestone")
        self.assertTrue(
            result["no_regression_vs_base_only_insufficient"],
            "T01-C03 no-regression versus base only is insufficient",
        )
        self.assertEqual(result["overall_verdict"], "long_term_drift", "T01-C03")
        self.assertNotEqual(result["overall_verdict"], "equal", "T01-C03")
        self.assertNotEqual(result["overall_verdict"], "equal_to_pr_base", "T01-C03 overall is not no-change")
        mile_rows = {row["case_id"]: row for row in result["candidate_vs_milestone"]["cases"]}
        self.assertEqual(mile_rows[candidate.case_id]["verdict"], "drift", "T01-C03")
        self.assertFalse(mile_rows[candidate.case_id]["no_regression"], "T01-C03")
        base_rows = {row["case_id"]: row for row in result["candidate_vs_pr_base"]["cases"]}
        self.assertEqual(base_rows[candidate.case_id]["verdict"], "equal", "T01-C03 vs base only looks unchanged")


if __name__ == "__main__":
    unittest.main()
