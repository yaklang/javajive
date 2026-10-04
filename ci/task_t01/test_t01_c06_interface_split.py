"""T01-C06 interface split: preserve historical CLI 13-check fields; Precision stays unknown."""

from __future__ import annotations

import hashlib
import json
import unittest
from pathlib import Path

from common import ROOT, observation_dict
from tools.milestone_ledger import parse_observation
from tools.milestone_ledger.constants import HISTORICAL_CLI_THIRTEEN_FIELDS, SEED_SOURCE_SHA256
from tools.milestone_ledger.errors import ObservationError
from tools.milestone_ledger.historical import load_historical_cli_report, package_historical_cli_path
from tools.milestone_ledger.observation import canonical_json, json_equal


class TestTaskT01C06InterfaceModeSeparation(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.loaded = load_historical_cli_report()

    def test_T01_C06_round_trip_historical_cli_thirteen_fields(self) -> None:
        self.assertGreaterEqual(len(self.loaded["cli"]), 4, "T01-C06")
        originals = {row["case"]: row for row in self.loaded["originals"]}
        for obs in self.loaded["cli"]:
            original = originals[obs.sample]
            self.assertEqual(obs.evidence.mode, "compatibility-cli", "T01-C06")
            self.assertEqual(obs.interface, "cli", "T01-C06")
            self.assertIsNotNone(obs.historical_raw, "T01-C06")
            for field in HISTORICAL_CLI_THIRTEEN_FIELDS:
                self.assertIn(field, obs.historical_raw, f"T01-C06 missing {field}")
                self.assertTrue(
                    json_equal(original[field], obs.historical_raw[field]),
                    f"T01-C06 field {field} for {obs.sample} is not JSON-equal",
                )
                self.assertEqual(
                    canonical_json(original[field]),
                    canonical_json(obs.historical_raw[field]),
                    f"T01-C06 byte/JSON equality failed for {obs.sample}.{field}",
                )
            for key, value in original.items():
                self.assertTrue(
                    json_equal(value, obs.historical_raw[key]),
                    f"T01-C06 extra historical key mutated: {obs.sample}.{key}",
                )
            self.assertEqual(obs.evidence.compiler_version, "javac 21.0.11", "T01-C06 real javac, not --release")
            self.assertNotIn("--release", obs.evidence.compiler_version, "T01-C06")

        baseline = next(o for o in self.loaded["cli"] if o.sample == "Baseline")
        self.assertEqual(baseline.status, "pass", "T01-C06")
        unicode_pair = next(o for o in self.loaded["cli"] if o.sample == "UnicodePair")
        self.assertEqual(unicode_pair.status, "fail", "T01-C06 historical fail is not a pass")
        self.assertEqual(unicode_pair.historical_raw["pass"], False, "T01-C06")

    def test_T01_C06_precision_unknown_when_not_executed(self) -> None:
        precision = [
            o
            for o in self.loaded["api_placeholders"]
            if o.evidence.mode == "precision"
        ]
        self.assertTrue(precision, "T01-C06")
        for obs in precision:
            self.assertIn(obs.status, {"not_run", "unknown"}, "T01-C06 Precision stays unknown")
            self.assertNotIn(obs.status, {"pass", "fail"}, "T01-C06 do not invent Precision pass/fail")
            self.assertEqual(obs.source, "api-not-executed", "T01-C06")
            self.assertIsNone(obs.historical_raw, "T01-C06 do not copy CLI results onto Precision")

    def test_T01_C06_mutate_precision_pass_without_evidence_rejected(self) -> None:
        target = next(
            o
            for o in self.loaded["api_placeholders"]
            if o.sample == "Baseline" and o.evidence.mode == "precision"
        )
        mutated = target.to_dict()
        mutated["status"] = "pass"
        with self.assertRaises(ObservationError) as ctx:
            parse_observation(mutated)
        self.assertIn("evidence", str(ctx.exception).lower(), "T01-C06")

        mutated_source = dict(mutated)
        mutated_source["source"] = "synthetic"
        mutated_source["execution_evidence"] = None
        with self.assertRaises(ObservationError) as ctx2:
            parse_observation(mutated_source)
        self.assertIn("evidence", str(ctx2.exception).lower(), "T01-C06")

        cli = next(o for o in self.loaded["cli"] if o.sample == "Baseline")
        stolen = target.to_dict()
        stolen["status"] = "pass"
        stolen["source"] = "synthetic"
        stolen["historical_raw"] = cli.historical_raw
        stolen["execution_evidence"] = {"executed": True, "kind": "synthetic"}
        with self.assertRaises(ObservationError) as ctx3:
            parse_observation(stolen)
        self.assertIn("historical", str(ctx3.exception).lower(), "T01-C06 CLI record cannot become Precision pass")

        fail_cli = next(o for o in self.loaded["cli"] if o.sample == "UnicodePair")
        flipped = fail_cli.to_dict()
        flipped["status"] = "pass"
        with self.assertRaises(ObservationError) as ctx4:
            parse_observation(flipped)
        self.assertIn("never a pass", str(ctx4.exception).lower(), "T01-C06")

    def test_T01_C06_reviewed_fixtures_match_seed_hashes(self) -> None:
        fixtures = ROOT / "tools" / "milestone_ledger" / "fixtures"
        for name, expected in SEED_SOURCE_SHA256.items():
            path = fixtures / f"{name}.java"
            got = hashlib.sha256(path.read_bytes()).hexdigest()
            self.assertEqual(got, expected, f"T01-C06 fixture hash {name}")
        report_path = package_historical_cli_path()
        self.assertTrue(report_path.is_file(), "T01-C06")
        data = json.loads(report_path.read_text(encoding="utf-8"))
        self.assertEqual(
            {row["case"] for row in data["observations"]},
            set(SEED_SOURCE_SHA256),
            "T01-C06",
        )


if __name__ == "__main__":
    unittest.main()
