import unittest

from support import ensure_evidence, analysis


class TestT32C03ScaleCurve(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.doc = ensure_evidence()

    def test_T32_C03_scale_curve_hashes_and_node_edge_counts(self):
        errors = analysis.check_c03(self.doc)
        self.assertEqual(errors, [], msg=errors)

    def test_T32_C03_not_just_mean_time(self):
        for kind in analysis.SCALE_KINDS:
            rows = []
            for lab in ("N", "2N", "4N"):
                matches = [
                    g
                    for g in self.doc["groups"]
                    if g.get("family_id") == f"{kind}/{lab}"
                    and g.get("stage") == "e2e_kernel"
                    and g.get("mode") == "warm"
                ]
                self.assertTrue(matches, f"missing {kind}/{lab}")
                g = matches[0]
                w = g.get("work") or {}
                self.assertTrue(w.get("input_sha256"))
                self.assertGreater(w.get("bytecode_bytes") or 0, 0)
                self.assertTrue(
                    (w.get("cfg_nodes") or 0) > 0
                    or (w.get("opcode_count") or 0) > 0
                    or (w.get("cfg_edges") or 0) > 0
                    or (w.get("nodes") or 0) > 0
                )
                self.assertIn("nodes", w)
                self.assertIn("edges", w)
                self.assertIn("def_merge", w)
                self.assertIn("region_roots", w)
                self.assertFalse(w.get("pending_t24_t25_t26"))
                rows.append(w)
            self.assertNotEqual(rows[0]["input_sha256"], rows[1]["input_sha256"])
            self.assertNotEqual(rows[1]["input_sha256"], rows[2]["input_sha256"])
