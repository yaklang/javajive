import unittest

from support import ensure_evidence, analysis


class TestT32C04NoiseSelfTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.doc = ensure_evidence()

    def test_T32_C04_noise_vs_2x_regression(self):
        errors = analysis.check_c04(self.doc)
        self.assertEqual(errors, [], msg=errors)

    def test_T32_C04_uncertain_not_called_improvement(self):
        n = self.doc["noise"]
        self.assertFalse(n.get("jitter_called_improvement"))
        self.assertNotEqual(n.get("jitter_verdict"), "improvement")
        self.assertTrue(n.get("clear_2x_regression_detected"))
        self.assertEqual(n.get("regression_verdict"), "clear_regression")
