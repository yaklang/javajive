import unittest

from support import ensure_evidence, analysis, EVIDENCE


class TestT32C01StageIsolation(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.doc = ensure_evidence()

    def test_T32_C01_stage_isolation_same_class_per_stage_vs_e2e(self):
        errors = analysis.check_c01(self.doc)
        self.assertEqual(errors, [], msg=errors)

    def test_T32_C01_honest_stage_labels(self):
        stages = {g.get("stage") for g in self.doc["groups"]}
        for st, marker in analysis.HONEST_STAGE_MARKERS.items():
            self.assertIn(st, stages, f"missing honest stage id {st}")
            self.assertIn(marker, st)
        for forbidden in analysis.FORBIDDEN_STAGE_IDS:
            self.assertNotIn(forbidden, stages)
        inv = {e["id"]: e for e in self.doc.get("stage_inventory") or []}
        self.assertIn("cfg_opcode_only", inv)
        self.assertTrue(inv["cfg_opcode_only"].get("opcode_only"))
        timed = (inv["cfg_opcode_only"].get("timed") or "").lower()
        self.assertNotIn("semanticcfg", timed.replace(" ", ""))
        self.assertTrue(inv["dataflow_stacksim_opcode_only"].get("opcode_only"))
        self.assertIn("calcopcodestackinfo", (inv["dataflow_stacksim_opcode_only"].get("timed") or "").lower())
        self.assertTrue(inv["region_and_statements_combined"].get("combined"))
        self.assertTrue(inv["render_class_dump_reruns_decompile"].get("reruns_decompile"))
        self.assertFalse(self.doc.get("pending_t24_t25_t26"))

    def test_T32_C01_javac_not_added_into_decompiler_kernel(self):
        found = False
        for g in self.doc["groups"]:
            self.assertFalse(
                g.get("kernel_includes_javac"),
                f"{g.get('group_id')} mixed javac into kernel",
            )
            if g.get("family_id") == "tiny_real/reviewed" and g.get("stage") == "e2e_kernel":
                found = True
                notes = ((g.get("work") or {}).get("notes") or "").lower()
                self.assertIn("never", notes)
                self.assertIn("javac", notes)
        self.assertTrue(found, "missing tiny_real e2e kernel group")
        split = " ".join(self.doc.get("stage_split_notes") or []).lower()
        self.assertIn("javac", split)
        self.assertIn("never", split)
        self.assertTrue((EVIDENCE / "summary.json").is_file())
        self.assertTrue((EVIDENCE / "stage-inventory.json").is_file())
