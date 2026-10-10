import unittest

from support import ensure_evidence, analysis


class TestT32Metrics(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.doc = ensure_evidence()

    def test_T32_M01_each_group_at_least_10_valid_repeats(self):
        r = analysis.check_m01(self.doc)
        self.assertEqual(r["verdict"], "pass", r)

    def test_T32_M02_stages_have_time_alloc_rss_work_counts(self):
        r = analysis.check_m02(self.doc)
        self.assertEqual(r["verdict"], "pass", r)

    def test_T32_M03_no_uncalibrated_percent_gate_or_masked_semantic(self):
        r = analysis.check_m03(self.doc)
        self.assertEqual(r["verdict"], "pass", r)
        self.assertEqual(self.doc.get("uncalibrated_hard_percentage_gates"), 0)
        self.assertIsNone(self.doc.get("claimed_speedup_percent"))
        self.assertIsNone(self.doc.get("speedup_percent"))
        self.assertFalse(self.doc.get("pending_t24_t25_t26"))
