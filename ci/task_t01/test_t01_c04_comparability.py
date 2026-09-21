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

    def test_T01_C04_same_fail_status_different_rebuilt_stdout_not_equal(self) -> None:
        """Same ledger fail is not equality when rebuilt outputs differ."""
        axes = dict(
            sample="ParamAnnotation",
            compiler_version="javac 21.0.11",
            harness_digest=digest(b"h"),
            dependency_lock_digest=digest(b"l"),
            input_hash=digest(b"i"),
        )
        left = obs(
            status="fail",
            execution_evidence={
                "executed": True,
                "kind": "api-live",
                "original_run": {"stdout": "1\n", "stderr": "", "rc": 0},
                "rebuilt_run": {"stdout": "0\n", "stderr": "", "rc": 0},
                "failure": "behavior mismatch",
                "stdout_match": False,
            },
            **axes,
        )
        right = obs(
            status="fail",
            execution_evidence={
                "executed": True,
                "kind": "api-live",
                "original_run": {"stdout": "1\n", "stderr": "", "rc": 0},
                "rebuilt_run": {"stdout": "2\n", "stderr": "", "rc": 0},
                "failure": "behavior mismatch",
                "stdout_match": False,
            },
            **axes,
        )
        result = compare_anchors([left], [right], [right])
        row = result["candidate_vs_pr_base"]["cases"][0]
        self.assertEqual(left.status, right.status)
        self.assertEqual(row["verdict"], "status_match_observation_differs", row)
        self.assertTrue(row["comparable"])
        self.assertIsNot(row.get("equals"), True)
        self.assertFalse(bool(row.get("equality_conclusion")))
        same = obs(
            status="fail",
            execution_evidence={
                "executed": True,
                "kind": "api-live",
                "original_run": {"stdout": "1\n", "stderr": "", "rc": 0},
                "rebuilt_run": {"stdout": "0\n", "stderr": "", "rc": 0},
                "failure": "behavior mismatch",
                "stdout_match": False,
            },
            **axes,
        )
        same_row = compare_anchors([left], [same], [same])["candidate_vs_pr_base"]["cases"][0]
        self.assertEqual(same_row["verdict"], "equal")
        self.assertTrue(same_row.get("equals"))

    def test_T01_C04_same_behavior_different_decompiled_source_is_not_regression(self) -> None:
        axes = dict(
            sample="Fmt",
            compiler_version="javac 21.0.11",
            harness_digest=digest(b"h"),
            dependency_lock_digest=digest(b"l"),
            input_hash=digest(b"i"),
        )
        run = {
            "executed": True,
            "kind": "api-live",
            "original_run": {"stdout": "7\n", "stderr": "", "rc": 0},
            "rebuilt_run": {"stdout": "7\n", "stderr": "", "rc": 0},
            "failure": None,
            "stdout_match": True,
            "decompile_status": "ok",
        }
        a = obs(status="pass", execution_evidence={**run, "decompiled_source_sha256": "a" * 64}, **axes)
        b = obs(status="pass", execution_evidence={**run, "decompiled_source_sha256": "b" * 64}, **axes)
        row = compare_anchors([a], [b], [b])["candidate_vs_pr_base"]["cases"][0]
        self.assertEqual(row["verdict"], "equal", row)
        self.assertTrue(row["runtime_equal"])
        self.assertFalse(row["source_equal"])
        self.assertTrue(row.get("equals"))
        self.assertTrue(any("format change" in r for r in row["reasons"]))

    def test_T01_C04_missing_one_runtime_payload_not_equality(self) -> None:
        axes = dict(
            sample="Gap",
            compiler_version="javac 21.0.11",
            harness_digest=digest(b"h"),
            dependency_lock_digest=digest(b"l"),
            input_hash=digest(b"i"),
        )
        full = obs(
            status="fail",
            execution_evidence={
                "executed": True,
                "kind": "api-live",
                "original_run": {"stdout": "1\n", "stderr": "", "rc": 0},
                "rebuilt_run": {"stdout": "0\n", "stderr": "", "rc": 0},
                "failure": "behavior mismatch",
                "stdout_match": False,
            },
            **axes,
        )
        empty = obs(status="fail", execution_evidence=None, **axes)
        row = compare_anchors([full], [empty], [empty])["candidate_vs_pr_base"]["cases"][0]
        self.assertEqual(row["verdict"], "status_match_incomplete_observation", row)
        self.assertFalse(bool(row.get("equals")))
        self.assertFalse(bool(row.get("no_regression")))

    def test_T01_C04_different_stderr_or_stage_not_equal(self) -> None:
        axes = dict(
            sample="Stage",
            compiler_version="javac 21.0.11",
            harness_digest=digest(b"h"),
            dependency_lock_digest=digest(b"l"),
            input_hash=digest(b"i"),
        )
        a = obs(
            status="fail",
            execution_evidence={
                "executed": True,
                "kind": "api-live",
                "original_run": {"stdout": "", "stderr": "VerifyError", "rc": 1},
                "rebuilt_run": {"stdout": "", "stderr": "VerifyError", "rc": 1},
                "failure": "verify",
                "stdout_match": True,
                "failure_stage": "verify_fail",
            },
            **axes,
        )
        b = obs(
            status="fail",
            execution_evidence={
                "executed": True,
                "kind": "api-live",
                "original_run": {"stdout": "", "stderr": "boom", "rc": 1},
                "rebuilt_run": {"stdout": "", "stderr": "boom", "rc": 1},
                "failure": "run",
                "stdout_match": True,
                "failure_stage": "run_fail",
            },
            **axes,
        )
        row = compare_anchors([a], [b], [b])["candidate_vs_pr_base"]["cases"][0]
        self.assertEqual(row["verdict"], "status_match_observation_differs", row)
        self.assertFalse(bool(row.get("equals")))

    def test_T01_C04_gained_capability_without_payload_is_not_runtime_proof(self) -> None:
        axes = dict(
            sample="Gain",
            compiler_version="javac 21.0.11",
            harness_digest=digest(b"h"),
            dependency_lock_digest=digest(b"l"),
            input_hash=digest(b"i"),
        )
        cand = obs(status="pass", **axes)
        ref = obs(status="fail", **axes)
        row = compare_anchors([cand], [ref], [ref])["candidate_vs_pr_base"]["cases"][0]
        self.assertEqual(row["verdict"], "improvement", row)
        self.assertFalse(row.get("runtime_proof"), row)
        self.assertFalse(bool(row.get("equals")))

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
