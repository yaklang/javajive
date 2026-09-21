import unittest

from support import ensure_evidence, analysis


class TestT32C05StubSkipCannotBePerfWin(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.doc = ensure_evidence()

    def test_T32_C05_faster_stub_is_correctness_regression(self):
        errors = analysis.check_c05(self.doc)
        self.assertEqual(errors, [], msg=errors)

    def test_T32_C05_skip_analysis_or_drop_output_not_perf_win(self):
        s = self.doc["semantic"]
        stub = s["faster_stub"]
        base = s["baseline"]
        self.assertTrue(stub.get("skipped_analysis"))
        self.assertGreater(stub.get("stub_method_count") or 0, 0)
        self.assertNotEqual(stub.get("output_sha256"), base.get("output_sha256"))
        self.assertTrue(s.get("correctness_regression"))
        self.assertFalse(s.get("recorded_as_perf_win"))
        self.assertEqual(s.get("classification"), "correctness_regression_not_perf_win")
        self.assertFalse(s.get("source_hash_used_as_semantic_fail"))
        if s.get("stub_is_faster"):
            self.assertFalse(s.get("recorded_as_perf_win"))
        note = (s.get("note") or "").lower()
        self.assertIn("hash", note)
        self.assertIn("not a semantic fail", note)

    def test_T32_C05_source_hash_inequality_is_not_semantic_fail(self):
        baseline = {"decompile_status": "complete", "output_sha256": "aaa", "stub_method_count": 0}
        candidate = {"decompile_status": "complete", "output_sha256": "bbb", "stub_method_count": 0}
        r = analysis.classify_semantic(baseline, candidate)
        self.assertEqual(r["classification"], "uncertain_source_diff_not_semantic_fail")
        self.assertFalse(r["correctness_regression"])
        self.assertFalse(r["recorded_as_perf_win"])
        self.assertFalse(r["source_hash_used_as_semantic_fail"])
        r2 = analysis.classify_semantic(
            baseline,
            candidate,
            oracle={"ran": True, "stdout_equal": True},
        )
        self.assertEqual(r2["classification"], "eligible_for_perf_compare")
        self.assertFalse(r2["correctness_regression"])
