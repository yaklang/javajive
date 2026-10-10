import json
import unittest

from support import ensure_evidence, analysis, EVIDENCE


class TestT32C06ReplayCommand(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.doc = ensure_evidence()

    def test_T32_C06_replay_command_and_stats_format(self):
        errors = analysis.check_c06(self.doc, EVIDENCE)
        self.assertEqual(errors, [], msg=errors)

    def test_T32_C06_machine_differences_explicit(self):
        mach = self.doc["machine"]
        tc = self.doc["toolchain"]
        self.assertTrue(mach.get("os"))
        self.assertTrue(mach.get("arch"))
        self.assertTrue(tc.get("go_version"))
        self.assertIn("difference", (mach.get("machine_difference_note") or "").lower())
        cmds = json.loads((EVIDENCE / "commands.json").read_text(encoding="utf-8"))
        self.assertTrue(cmds)
        acc = [c for c in cmds if c.get("phase") == "acceptance"]
        self.assertTrue(acc)
        self.assertTrue(acc[0]["argv"])
        self.assertIn("go", acc[0]["argv"][0])
        samples = (EVIDENCE / "samples.jsonl").read_text(encoding="utf-8").strip().splitlines()
        self.assertGreaterEqual(len(samples), 10)
        row = json.loads(samples[0])
        for key in ("group_id", "stage", "mode", "cache_key", "samples", "work", "input_sha256"):
            self.assertIn(key, row)
