"""T01 live Precision/Compatibility API replay: real compile/decompile/rebuild/run."""

from __future__ import annotations

import hashlib
import json
import re
import shutil
import tempfile
import unittest
from pathlib import Path

from common import ROOT
from tools.milestone_ledger.constants import HISTORICAL_CLI_THIRTEEN_FIELDS, SEED_SOURCE_SHA256
from tools.milestone_ledger.historical import load_historical_cli_report
from tools.milestone_ledger.live_replay import (
    DEFAULT_SAMPLES,
    EVIDENCE_DIR,
    LiveReplayInfraError,
    run_live_replay,
    write_live_replay_evidence,
)
from tools.milestone_ledger.observation import is_semantic_pass, json_equal, parse_observation
from tools.milestone_ledger.provisional import (
    PROVISIONAL_CANDIDATE_REVISION,
    is_fake_40_hex_candidate,
    reject_fake_candidate_sha,
)

_GIT_SHA = re.compile(r"^[0-9a-f]{40}$")
_REQUIRED_SAMPLES = ("Baseline", "UnicodePair")
_MODES = ("precision", "compatibility")
_DEBUGS = ("debug", "nodebug")
_MATRIX = 16


class TestTaskT01LiveApiReplay(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        if shutil.which("javac") is None or shutil.which("java") is None or shutil.which("go") is None:
            raise AssertionError("infra_error: javac/java/go required; live API replay must not skip")
        cls._tmp = tempfile.TemporaryDirectory(prefix="t01-live-api-")
        cls.work = Path(cls._tmp.name)
        try:
            cls.report = run_live_replay(
                repo=ROOT,
                work=cls.work,
                samples=DEFAULT_SAMPLES,
                modes=_MODES,
                debugs=_DEBUGS,
            )
        except LiveReplayInfraError as exc:
            raise AssertionError(f"infra_error: {exc}") from exc
        cls.observations = list(cls.report["observations"])
        cls.by_id = {obs.case_id: obs for obs in cls.observations}
        cls.evidence_dir = write_live_replay_evidence(cls.report, EVIDENCE_DIR)

    @classmethod
    def tearDownClass(cls) -> None:
        cls._tmp.cleanup()

    def _cells(self, sample: str):
        rows = [
            self.by_id[f"{sample}::api::{mode}::{debug}"]
            for mode in _MODES
            for debug in _DEBUGS
        ]
        self.assertEqual(len(rows), 4, sample)
        return rows

    def test_live_replay_does_not_skip_and_covers_matrix(self) -> None:
        self.assertEqual(len(self.observations), _MATRIX)
        seen = {(obs.sample, obs.evidence.mode, obs.evidence.debug) for obs in self.observations}
        expected = {(sample, mode, debug) for sample in DEFAULT_SAMPLES for mode in _MODES for debug in _DEBUGS}
        self.assertEqual(seen, expected)
        for sample in _REQUIRED_SAMPLES:
            for mode in _MODES:
                for debug in _DEBUGS:
                    self.assertIn(f"{sample}::api::{mode}::{debug}", self.by_id)
        for obs in self.observations:
            self.assertFalse(obs.skip)
            self.assertFalse(obs.xfail)
            self.assertNotEqual(obs.status, "not_run")
            self.assertNotEqual(obs.source, "api-not-executed")
            self.assertEqual(obs.source, "api-live")
            self.assertEqual(obs.interface, "api")
            self.assertIsNotNone(obs.execution_evidence)
            self.assertTrue(obs.execution_evidence["executed"])
            self.assertEqual(obs.execution_evidence["kind"], "api-live")
            self.assertNotIn(obs.status, {"not_run", "unknown"})

    def test_unicodepair_not_silent_pass_when_stdout_differs(self) -> None:
        for obs in self._cells("UnicodePair"):
            ee = obs.execution_evidence or {}
            original = ee.get("original_run") or {}
            rebuilt = ee.get("rebuilt_run") or {}
            self.assertEqual(original.get("stdout"), "65\n55357\n56832\n90\n", obs.case_id)
            if original.get("stdout") != rebuilt.get("stdout") or original.get("stderr") != rebuilt.get("stderr"):
                self.assertEqual(obs.status, "fail", obs.case_id)
                self.assertNotEqual(obs.status, "pass", obs.case_id)
                self.assertFalse(is_semantic_pass(obs), obs.case_id)
                self.assertFalse(ee.get("stdout_match"), obs.case_id)

    def test_baseline_pass_from_real_rebuild_run(self) -> None:
        for obs in self._cells("Baseline"):
            ee = obs.execution_evidence or {}
            original = ee.get("original_run") or {}
            rebuilt = ee.get("rebuilt_run") or {}
            compile_rec = ee.get("rebuilt_compile") or {}
            verify_rec = ee.get("rebuilt_verify") or {}
            self.assertEqual(original.get("stdout"), "7\n", obs.case_id)
            self.assertEqual(original.get("rc"), 0, obs.case_id)
            self.assertEqual(compile_rec.get("rc"), 0, json.dumps(compile_rec)[:500])
            self.assertEqual(verify_rec.get("rc"), 0, json.dumps(verify_rec)[:500])
            self.assertEqual(rebuilt.get("rc"), 0, obs.case_id)
            self.assertEqual(original.get("stdout"), rebuilt.get("stdout"), obs.case_id)
            self.assertEqual(original.get("stderr"), rebuilt.get("stderr"), obs.case_id)
            self.assertTrue(ee.get("stdout_match"), obs.case_id)
            self.assertTrue(ee.get("compile_ok"), obs.case_id)
            self.assertTrue(ee.get("verify_ok"), obs.case_id)
            self.assertFalse(ee.get("original_class_on_rebuilt_classpath"), obs.case_id)
            rebuilt_cp = list(ee.get("rebuilt_classpath") or [])
            original_dir = (ee.get("original_classpath") or [None])[0]
            self.assertNotIn(original_dir, rebuilt_cp)
            self.assertEqual(obs.status, "pass", obs.case_id)
            self.assertTrue(is_semantic_pass(obs), obs.case_id)
            self.assertIn("kind", ee)
            self.assertEqual(ee["kind"], "api-live")

    def test_provisional_sha_is_not_fake_40_hex(self) -> None:
        provisional = self.report["provisional"]
        self.assertEqual(provisional["candidate_revision"], PROVISIONAL_CANDIDATE_REVISION)
        self.assertEqual(provisional["candidate_revision"], "PROVISIONAL_UNCOMMITTED")
        self.assertFalse(is_fake_40_hex_candidate(provisional["candidate_revision"]))
        self.assertFalse(bool(_GIT_SHA.fullmatch(str(provisional["candidate_revision"]))))
        self.assertIsNone(provisional["immutable_candidate_sha"])
        self.assertTrue(_GIT_SHA.fullmatch(str(provisional["git_head"])))
        self.assertNotEqual(provisional["candidate_revision"], provisional["git_head"])
        self.assertEqual(reject_fake_candidate_sha(provisional["candidate_revision"]), "PROVISIONAL_UNCOMMITTED")
        with self.assertRaises(Exception) as ctx:
            reject_fake_candidate_sha("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
        self.assertIn("fake", str(ctx.exception).lower())
        for obs in self.observations:
            self.assertEqual(obs.extras["candidate_revision"], "PROVISIONAL_UNCOMMITTED")
            self.assertFalse(is_fake_40_hex_candidate(obs.extras["candidate_revision"]))
            self.assertEqual(obs.evidence.revision, provisional["git_head"])
            self.assertNotEqual(obs.extras["candidate_revision"], obs.evidence.revision)
            self.assertTrue(_GIT_SHA.fullmatch(obs.evidence.revision))
            self.assertFalse(obs.extras.get("immutable_candidate_sha_claimed"))

    def test_candidate_provisional_and_head_replay_rows_written(self) -> None:
        cand = json.loads((self.evidence_dir / "candidate_provisional.json").read_text(encoding="utf-8"))
        head = json.loads((self.evidence_dir / "head_replay.json").read_text(encoding="utf-8"))
        self.assertEqual(cand["candidate_revision"], "PROVISIONAL_UNCOMMITTED")
        self.assertEqual(head["candidate_revision"], "PROVISIONAL_UNCOMMITTED")
        self.assertEqual(cand["anchor_role"], "candidate-provisional")
        self.assertEqual(head["anchor_role"], "head_replay")
        self.assertTrue(cand["worktree_dirty"] in (True, False))
        self.assertEqual(len(cand["observations"]), _MATRIX)
        self.assertEqual(len(head["observations"]), _MATRIX)
        for row in cand["observations"] + head["observations"]:
            obs = parse_observation(row)
            self.assertEqual(obs.source, "api-live")
            self.assertEqual(obs.execution_evidence["kind"], "api-live")

    def test_historical_cli_thirteen_fields_remain_separate(self) -> None:
        loaded = load_historical_cli_report()
        self.assertGreaterEqual(len(loaded["cli"]), 4)
        originals = {row["case"]: row for row in loaded["originals"]}
        for obs in loaded["cli"]:
            original = originals[obs.sample]
            self.assertEqual(obs.evidence.mode, "compatibility-cli")
            self.assertEqual(obs.interface, "cli")
            self.assertEqual(obs.source, "historical-cli")
            for field in HISTORICAL_CLI_THIRTEEN_FIELDS:
                self.assertTrue(json_equal(original[field], obs.historical_raw[field]), field)
        live_ids = {obs.case_id for obs in self.observations}
        hist_ids = {obs.case_id for obs in loaded["cli"]}
        self.assertTrue(live_ids.isdisjoint(hist_ids))
        hist_file = json.loads((self.evidence_dir / "historical_cli.json").read_text(encoding="utf-8"))
        self.assertTrue(hist_file["separate"])
        self.assertEqual(hist_file["source"], "historical-cli")

    def test_mismatch_samples_are_not_silent_pass(self) -> None:
        for sample in ("ConcatProbe", "ParamAnnotation", "UnicodePair"):
            for obs in self._cells(sample):
                ee = obs.execution_evidence or {}
                original = ee.get("original_run") or {}
                rebuilt = ee.get("rebuilt_run") or {}
                if original.get("stdout") != rebuilt.get("stdout") or original.get("stderr") != rebuilt.get("stderr"):
                    self.assertNotEqual(obs.status, "pass", obs.case_id)
                    self.assertFalse(is_semantic_pass(obs), obs.case_id)

    def test_infra_error_never_counted_as_pass(self) -> None:
        for obs in self.observations:
            if obs.status == "infra_error":
                self.assertFalse(is_semantic_pass(obs), obs.case_id)
                self.assertNotEqual(obs.status, "pass")
        self.assertFalse(any(is_semantic_pass(o) and o.status != "pass" for o in self.observations))

    def test_reviewed_fixture_hashes_and_decompile_probe_used(self) -> None:
        for name, expected in SEED_SOURCE_SHA256.items():
            path = ROOT / "tools" / "milestone_ledger" / "fixtures" / f"{name}.java"
            got = hashlib.sha256(path.read_bytes()).hexdigest()
            self.assertEqual(got, expected, name)
        for obs in self.observations:
            ee = obs.execution_evidence or {}
            self.assertTrue(str(ee.get("probe") or "").endswith("t01-api-probe") or "t01-api-probe" in str(ee.get("probe")))
            self.assertIn("decompiled_source", ee)
            self.assertTrue((ee.get("original_compile") or {}).get("argv"))
            argv = (ee.get("original_compile") or {}).get("argv") or []
            self.assertIn("-proc:none", argv)
            self.assertIn("--release", argv)
            self.assertTrue("-g" in argv or "-g:none" in argv)


if __name__ == "__main__":
    unittest.main()
