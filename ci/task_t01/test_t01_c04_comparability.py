"""T01-C04: same name but different JDK/harness/lock/hash is incomparable, never equal."""

from __future__ import annotations

import unittest

from common import PR_BASE, digest, obs
from tools.milestone_ledger import compare_anchors, comparability_reasons


def _pair(**kwargs):
    left = obs(sample="SameName", mode="precision", debug="debug", status="pass", revision=PR_BASE, **kwargs)
    return left


class TestTaskT01C04ComparabilityRejection(unittest.TestCase):
    def _assert_incomparable(self, candidate, reference, axis: str) -> None:
        reasons = comparability_reasons(candidate, reference)
        self.assertTrue(any(axis in item for item in reasons), f"T01-C04 {axis}")
        result = compare_anchors([candidate], [reference], [reference])
        for side in ("candidate_vs_pr_base", "candidate_vs_milestone"):
            row = result[side]["cases"][0]
            self.assertEqual(row["verdict"], "incomparable", f"T01-C04 {axis} {side}")
            self.assertIsNot(row.get("equals"), True, f"T01-C04 {axis} must not emit equality")
            self.assertIsNot(row.get("no_regression"), True, f"T01-C04 {axis} must not emit no-regression")
            self.assertIsNone(row.get("equality_conclusion"), f"T01-C04 {axis}")
            self.assertIsNone(row.get("no_regression_conclusion"), f"T01-C04 {axis}")
        self.assertEqual(result["overall_verdict"], "incomparable", "T01-C04")
        self.assertNotEqual(result["overall_verdict"], "equal", "T01-C04")

    def test_T01_C04_different_javac_version_incomparable(self) -> None:
        a = obs(
            sample="SameName",
            compiler_version="javac 21.0.11",
            harness_digest=digest(b"h"),
            dependency_lock_digest=digest(b"l"),
            input_hash=digest(b"i"),
        )
        b = obs(
            sample="SameName",
            compiler_version="javac 17.0.14",
            harness_digest=digest(b"h"),
            dependency_lock_digest=digest(b"l"),
            input_hash=digest(b"i"),
        )
        self._assert_incomparable(a, b, "compiler_version")

    def test_T01_C04_different_harness_digest_incomparable(self) -> None:
        a = obs(
            sample="SameName",
            compiler_version="javac 21.0.11",
            harness_digest=digest(b"harness-a"),
            dependency_lock_digest=digest(b"l"),
            input_hash=digest(b"i"),
        )
        b = obs(
            sample="SameName",
            compiler_version="javac 21.0.11",
            harness_digest=digest(b"harness-b"),
            dependency_lock_digest=digest(b"l"),
            input_hash=digest(b"i"),
        )
        self._assert_incomparable(a, b, "harness_digest")

    def test_T01_C04_different_input_hash_incomparable(self) -> None:
        a = obs(
            sample="SameName",
            compiler_version="javac 21.0.11",
            harness_digest=digest(b"h"),
            dependency_lock_digest=digest(b"l"),
            input_hash=digest(b"input-a"),
        )
        b = obs(
            sample="SameName",
            compiler_version="javac 21.0.11",
            harness_digest=digest(b"h"),
            dependency_lock_digest=digest(b"l"),
            input_hash=digest(b"input-b"),
        )
        self._assert_incomparable(a, b, "input_hash")

    def test_T01_C04_different_dependency_lock_incomparable(self) -> None:
        a = obs(
            sample="SameName",
            compiler_version="javac 21.0.11",
            harness_digest=digest(b"h"),
            dependency_lock_digest=digest(b"lock-a"),
            input_hash=digest(b"i"),
        )
        b = obs(
            sample="SameName",
            compiler_version="javac 21.0.11",
            harness_digest=digest(b"h"),
            dependency_lock_digest=digest(b"lock-b"),
            input_hash=digest(b"i"),
        )
        self._assert_incomparable(a, b, "dependency_lock_digest")

    def test_T01_C04_broken_run_is_infra_error_not_equal(self) -> None:
        a = obs(sample="SameName", status="infra_error")
        b = obs(sample="SameName", status="pass")
        result = compare_anchors([a], [b], [b])
        row = result["candidate_vs_pr_base"]["cases"][0]
        self.assertEqual(row["verdict"], "infra_error", "T01-C04")
        self.assertIsNot(row.get("equals"), True, "T01-C04")
        self.assertEqual(result["overall_verdict"], "infra_error", "T01-C04")


if __name__ == "__main__":
    unittest.main()
