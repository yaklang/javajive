import unittest

from support import SESSION_NONCE, SESSION_STARTED, ensure_evidence, analysis, EVIDENCE


class TestT32C02ColdWarm(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.doc = ensure_evidence()

    def test_T32_C02_cold_vs_warm_labels_and_cache_keys(self):
        errors = analysis.check_c02(self.doc)
        self.assertEqual(errors, [], msg=errors)

    def test_T32_C02_evidence_refreshed_this_session(self):
        self.assertEqual(self.doc.get("collect_nonce"), SESSION_NONCE)
        nonce_file = (EVIDENCE / "collect-nonce.txt").read_text(encoding="utf-8").strip()
        self.assertEqual(nonce_file, SESSION_NONCE)
        self.assertGreaterEqual(self.doc.get("collected_at_unix") or 0, int(SESSION_STARTED) - 5)
        mtime = (EVIDENCE / "summary.json").stat().st_mtime
        self.assertGreaterEqual(mtime, SESSION_STARTED - 2)

    def test_T32_C02_does_not_pick_best_trial_as_improvement(self):
        for g in self.doc["groups"]:
            if g.get("family_id") != "tiny_real/reviewed" or g.get("stage") != "e2e_kernel":
                continue
            samples = [s["ns"] for s in g.get("samples") or [] if s.get("ok") and s.get("role") == "measured"]
            if len(samples) < 2:
                continue
            best = min(samples)
            reported_mean = g.get("mean_ns")
            self.assertGreaterEqual(len(samples), 10)
            self.assertGreater(reported_mean, 0)
            self.assertEqual(g.get("best_trial_ns_not_used_as_result"), best)
            if max(samples) != min(samples):
                self.assertNotEqual(reported_mean, best)
