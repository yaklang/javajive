"""T01 frozen SHA 31de113 live API replay: evidence contract, not provisional."""

from __future__ import annotations

import hashlib
import json
import os
import re
import subprocess
import unittest
from pathlib import Path

from common import ROOT
from tools.milestone_ledger.constants import PR_BASE_SHA, SEED_SOURCE_SHA256
from tools.milestone_ledger.frozen_replay import (
    ANCHOR_ROLE,
    EVIDENCE_DIR,
    FROZEN_SHA,
    run_frozen_replay,
)
from tools.milestone_ledger.live_replay import harness_digest
from tools.milestone_ledger.observation import is_semantic_pass, parse_observation
from tools.milestone_ledger.provisional import PROVISIONAL_CANDIDATE_REVISION

_GIT_SHA = re.compile(r"^[0-9a-f]{40}$")
_SHA256 = re.compile(r"^[0-9a-f]{64}$")
_MODES = ("precision", "compatibility")
_DEBUGS = ("debug", "nodebug")
_SAMPLES = ("Baseline", "UnicodePair", "ConcatProbe", "ParamAnnotation")
_MATRIX = 16
_HOME_LEAK = "/Users/v1ll4n"
_BEHAVIOR_FAIL_PREFIXES = (
    "behavior mismatch",
    "rebuilt compile failed",
    "rebuilt verify failed",
    "DecompileWithOptions error",
    "stdout/stderr mismatch",
)


def _load_json(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


class TestTaskT01Frozen31de113(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.evidence_dir = Path(EVIDENCE_DIR)
        try:
            # Reconstructs 31de113 and 81f8ef5 from git objects into unique temps.
            # Explicit JAVAJIVE_FROZEN_31DE113 / JAVAJIVE_PR_BASE_81F8EF5 are honored
            # read-only; missing objects are infra_error, never skipped.
            run_frozen_replay(dest=cls.evidence_dir, evidence_repo=ROOT)
        except Exception as exc:
            raise AssertionError(f"infra_error: frozen replay failed: {exc}") from exc
        obs_path = cls.evidence_dir / "observations.json"
        summary_path = cls.evidence_dir / "summary.json"
        if not obs_path.is_file() or not summary_path.is_file():
            raise AssertionError("infra_error: frozen replay did not write observations")
        cls.obs_doc = _load_json(obs_path)
        cls.summary = _load_json(summary_path)
        cls.frozen_doc = _load_json(cls.evidence_dir / "frozen.json")
        cls.probe_build = _load_json(cls.evidence_dir / "probe_build.json")
        cls.compare = _load_json(cls.evidence_dir / "compare_vs_pr_base_81f8ef5.json")
        cls.pr_base = _load_json(cls.evidence_dir / "pr_base_81f8ef5_live.json")
        cls.observations = [parse_observation(row) for row in cls.obs_doc["observations"]]
        cls.by_id = {obs.case_id: obs for obs in cls.observations}

    def _cells(self, sample: str):
        rows = [
            self.by_id[f"{sample}::api::{mode}::{debug}"]
            for mode in _MODES
            for debug in _DEBUGS
        ]
        self.assertEqual(len(rows), 4, sample)
        return rows

    def _refuse_pass_on_infra(self, obs) -> None:
        if obs.status == "infra_error":
            self.assertNotEqual(obs.status, "pass", obs.case_id)
            self.assertFalse(is_semantic_pass(obs), obs.case_id)

    def test_matrix_coverage_and_api_live_source(self) -> None:
        self.assertEqual(len(self.observations), _MATRIX)
        seen = {(obs.sample, obs.evidence.mode, obs.evidence.debug) for obs in self.observations}
        expected = {(sample, mode, debug) for sample in _SAMPLES for mode in _MODES for debug in _DEBUGS}
        self.assertEqual(seen, expected)
        for obs in self.observations:
            self.assertFalse(obs.skip, obs.case_id)
            self.assertFalse(obs.xfail, obs.case_id)
            self.assertNotEqual(obs.status, "not_run", obs.case_id)
            self.assertNotEqual(obs.source, "api-not-executed", obs.case_id)
            self.assertEqual(obs.source, "api-live", obs.case_id)
            self.assertEqual(obs.interface, "api", obs.case_id)
            self.assertIsNotNone(obs.execution_evidence, obs.case_id)
            self.assertTrue(obs.execution_evidence["executed"], obs.case_id)
            self.assertEqual(obs.execution_evidence["kind"], "api-live", obs.case_id)

    def test_candidate_revision_is_frozen_40_hex_not_provisional(self) -> None:
        self.assertTrue(_GIT_SHA.fullmatch(FROZEN_SHA))
        self.assertEqual(FROZEN_SHA, "31de113dded2fec7c34799776f43cca213aa1d77")
        self.assertEqual(self.summary["candidate_revision"], FROZEN_SHA)
        self.assertEqual(self.summary["anchor_role"], ANCHOR_ROLE)
        self.assertEqual(self.summary["anchor_role"], "frozen_31de113")
        self.assertTrue(self.summary["not_provisional"])
        self.assertTrue(self.summary["not_latest_integrated"])
        self.assertEqual(self.obs_doc["candidate_revision"], FROZEN_SHA)
        self.assertNotEqual(self.summary["candidate_revision"], PROVISIONAL_CANDIDATE_REVISION)
        self.assertNotEqual(self.summary["candidate_revision"], "PROVISIONAL_UNCOMMITTED")
        for obs in self.observations:
            self.assertEqual(obs.extras["candidate_revision"], FROZEN_SHA, obs.case_id)
            self.assertEqual(obs.evidence.revision, FROZEN_SHA, obs.case_id)
            self.assertEqual(obs.extras["anchor_role"], "frozen_31de113", obs.case_id)
            self.assertTrue(obs.extras["not_provisional"], obs.case_id)
            self.assertTrue(obs.extras["not_latest_integrated"], obs.case_id)
            self.assertEqual(obs.extras["immutable_candidate_sha"], FROZEN_SHA, obs.case_id)

    def test_does_not_claim_latest_integrated_or_evidence_checkpoint(self) -> None:
        head = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
        self.assertTrue(_GIT_SHA.fullmatch(head), head)
        self.assertEqual(self.frozen_doc["evidence_checkpoint_head"], head)
        self.assertFalse(self.frozen_doc["evidence_checkpoint_is_latest_integrated"])
        self.assertTrue(self.frozen_doc["not_latest_integrated"])
        self.assertNotEqual(self.summary["candidate_revision"], head)
        self.assertEqual(self.summary["candidate_revision"], FROZEN_SHA)
        self.assertIn("not latest integrated", json.dumps(self.summary).lower())
        self.assertIn("checkpoint", str(self.frozen_doc.get("claim") or "").lower())
        for obs in self.observations:
            notes = obs.notes.lower()
            self.assertIn("not latest integrated", notes, obs.case_id)
            self.assertIn("checkpoint", notes, obs.case_id)
            self.assertTrue(obs.extras.get("not_latest_integrated"), obs.case_id)
            self.assertFalse(obs.extras.get("evidence_checkpoint_is_latest_integrated"), obs.case_id)

    def test_baseline_not_infra_error(self) -> None:
        for obs in self._cells("Baseline"):
            self._refuse_pass_on_infra(obs)
            self.assertNotEqual(obs.status, "infra_error", obs.case_id)
            ee = obs.execution_evidence or {}
            original = ee.get("original_run") or {}
            rebuilt = ee.get("rebuilt_run") or {}
            compile_rec = ee.get("rebuilt_compile") or {}
            verify_rec = ee.get("rebuilt_verify") or {}
            self.assertEqual(original.get("stdout"), "7\n", obs.case_id)
            self.assertEqual(original.get("rc"), 0, obs.case_id)
            self.assertEqual(compile_rec.get("rc"), 0, obs.case_id)
            self.assertEqual(verify_rec.get("rc"), 0, obs.case_id)
            self.assertEqual(rebuilt.get("rc"), 0, obs.case_id)
            self.assertTrue(ee.get("stdout_match"), obs.case_id)
            self.assertTrue(ee.get("compile_ok"), obs.case_id)
            self.assertTrue(ee.get("verify_ok"), obs.case_id)
            self.assertFalse(ee.get("original_class_on_rebuilt_classpath"), obs.case_id)
            if obs.status == "infra_error":
                self.assertNotEqual(obs.status, "pass", obs.case_id)
                self.assertFalse(is_semantic_pass(obs), obs.case_id)
                continue
            self.assertEqual(obs.status, "pass", obs.case_id)
            self.assertTrue(is_semantic_pass(obs), obs.case_id)

    def test_unicodepair_fail_is_behavior_not_infra(self) -> None:
        for obs in self._cells("UnicodePair"):
            self._refuse_pass_on_infra(obs)
            self.assertNotEqual(obs.status, "infra_error", obs.case_id)
            ee = obs.execution_evidence or {}
            original = ee.get("original_run") or {}
            rebuilt = ee.get("rebuilt_run") or {}
            self.assertEqual(original.get("stdout"), "65\n55357\n56832\n90\n", obs.case_id)
            self.assertTrue(ee.get("executed"), obs.case_id)
            self.assertEqual(ee.get("kind"), "api-live", obs.case_id)
            if obs.status == "infra_error":
                self.assertNotEqual(obs.status, "pass", obs.case_id)
                self.assertFalse(is_semantic_pass(obs), obs.case_id)
                continue
            stdout_differs = original.get("stdout") != rebuilt.get("stdout") or original.get("stderr") != rebuilt.get(
                "stderr"
            )
            if stdout_differs or obs.status == "fail":
                self.assertEqual(obs.status, "fail", obs.case_id)
                self.assertNotEqual(obs.status, "pass", obs.case_id)
                self.assertFalse(is_semantic_pass(obs), obs.case_id)
                failure = str(ee.get("failure") or "")
                self.assertTrue(
                    any(failure.startswith(prefix) or prefix in failure for prefix in _BEHAVIOR_FAIL_PREFIXES),
                    f"{obs.case_id} fail must be behavior not infra: {failure!r}",
                )
                self.assertNotIn("infra_error", failure)
            else:
                self.assertEqual(obs.status, "pass", obs.case_id)
                self.assertTrue(ee.get("stdout_match"), obs.case_id)
                self.assertTrue(ee.get("compile_ok"), obs.case_id)
                self.assertTrue(ee.get("verify_ok"), obs.case_id)
                self.assertTrue(is_semantic_pass(obs), obs.case_id)

    def test_infra_error_never_counted_as_pass(self) -> None:
        for obs in self.observations:
            self._refuse_pass_on_infra(obs)
            if obs.status == "infra_error":
                self.assertFalse(is_semantic_pass(obs), obs.case_id)
                self.assertNotEqual(obs.status, "pass", obs.case_id)
            if is_semantic_pass(obs):
                self.assertEqual(obs.status, "pass", obs.case_id)
                self.assertNotEqual(obs.status, "infra_error", obs.case_id)
        self.assertEqual(self.summary["counts"]["infra_error"], 0)
        self.assertFalse(any(o.status == "infra_error" and o.status == "pass" for o in self.observations))

    def test_mismatch_samples_are_not_silent_pass(self) -> None:
        for sample in _SAMPLES:
            for obs in self._cells(sample):
                self._refuse_pass_on_infra(obs)
                ee = obs.execution_evidence or {}
                original = ee.get("original_run") or {}
                rebuilt = ee.get("rebuilt_run") or {}
                if original.get("stdout") != rebuilt.get("stdout") or original.get("stderr") != rebuilt.get("stderr"):
                    self.assertNotEqual(obs.status, "pass", obs.case_id)
                    self.assertFalse(is_semantic_pass(obs), obs.case_id)
                    self.assertNotEqual(obs.status, "infra_error", obs.case_id)

    def test_paramannotation_fail_is_behavior_when_present(self) -> None:
        for obs in self._cells("ParamAnnotation"):
            self._refuse_pass_on_infra(obs)
            self.assertNotEqual(obs.status, "infra_error", obs.case_id)
            ee = obs.execution_evidence or {}
            if obs.status == "fail":
                self.assertFalse(is_semantic_pass(obs), obs.case_id)
                failure = str(ee.get("failure") or "")
                self.assertTrue(
                    any(prefix in failure for prefix in _BEHAVIOR_FAIL_PREFIXES),
                    failure,
                )
                self.assertFalse(ee.get("stdout_match"), obs.case_id)
                original = (ee.get("original_run") or {}).get("stdout")
                rebuilt = (ee.get("rebuilt_run") or {}).get("stdout")
                self.assertEqual(original, "1\n", obs.case_id)
                self.assertEqual(rebuilt, "0\n", obs.case_id)

    def test_original_class_not_on_rebuilt_classpath_and_verify_all(self) -> None:
        for obs in self.observations:
            ee = obs.execution_evidence or {}
            self.assertFalse(ee.get("original_class_on_rebuilt_classpath"), obs.case_id)
            rebuilt_cp = list(ee.get("rebuilt_classpath") or [])
            original_dir = (ee.get("original_classpath") or [None])[0]
            self.assertNotIn(original_dir, rebuilt_cp, obs.case_id)
            for rec_name in ("original_run", "rebuilt_run", "original_verify", "rebuilt_verify"):
                rec = ee.get(rec_name) or {}
                argv = rec.get("argv") or []
                if argv:
                    self.assertIn("-Xverify:all", argv, f"{obs.case_id} {rec_name}")

    def test_probe_built_against_frozen_tree_replace(self) -> None:
        method = str(self.probe_build.get("method") or "")
        tree = str(self.probe_build.get("frozen_tree") or self.frozen_doc.get("frozen_tree") or "")
        self.assertTrue(tree, "probe must record the materialized frozen tree")
        self.assertIn("replace github.com/yaklang/javajive =>", method)
        self.assertIn(tree, method)
        self.assertIn("CGO_ENABLED=1", method)
        self.assertIn("GOTOOLCHAIN=local", method)
        self.assertIn("-ldflags=-linkmode=external", method)
        self.assertFalse(self.probe_build.get("writes_frozen_tree"))
        self.assertFalse(self.probe_build.get("writes_anchor_tree"))
        self.assertEqual(self.probe_build.get("cgo_enabled"), "1")
        probe = Path(self.probe_build.get("probe") or "")
        self.assertTrue(probe.is_file(), probe)
        for obs in self.observations:
            ee = obs.execution_evidence or {}
            self.assertEqual(str(ee.get("probe") or ""), str(probe), obs.case_id)

    def test_historical_81f8ef5_live_rows_are_separate_file(self) -> None:
        protocol = harness_digest()
        self.assertTrue(self.pr_base.get("separate"))
        self.assertEqual(self.pr_base.get("anchor_role"), "pr_base_81f8ef5_live")
        self.assertEqual(self.pr_base.get("git_head"), PR_BASE_SHA)
        self.assertEqual(self.pr_base.get("candidate_revision"), PR_BASE_SHA)
        self.assertNotEqual(self.pr_base.get("candidate_revision"), PROVISIONAL_CANDIDATE_REVISION)
        self.assertTrue(self.compare.get("separate"))
        self.assertEqual(self.compare.get("left_anchor"), "frozen_31de113")
        self.assertEqual(self.compare.get("right_anchor"), "pr_base_81f8ef5_live")
        self.assertTrue(self.compare.get("not_latest_integrated"))
        self.assertTrue(self.compare.get("same_protocol_harness_digest"), self.compare)
        self.assertEqual(self.compare.get("protocol_harness_digest"), protocol)
        self.assertEqual(self.compare.get("harness_digest_frozen"), protocol)
        self.assertEqual(self.compare.get("harness_digest_pr_base"), protocol)
        frozen_ids = {obs.case_id for obs in self.observations}
        pr_ids = {row["case_id"] for row in self.pr_base.get("observations") or []}
        self.assertEqual(frozen_ids, pr_ids)
        for obs in self.observations:
            self.assertEqual(obs.evidence.harness_digest, protocol, obs.case_id)
        for case in self.compare.get("cases") or []:
            reasons = " ".join(case.get("axis_reasons") or case.get("reasons") or [])
            self.assertNotIn("harness_digest differs", reasons, case.get("case_id"))
            if case.get("comparable"):
                same_status = case.get("frozen_status") == case.get("pr_base_status")
                if case.get("verdict") == "status_match_observation_differs":
                    self.assertTrue(same_status, case.get("case_id"))
                    self.assertFalse(bool(case.get("equals")), case.get("case_id"))
                    self.assertFalse(bool(case.get("equality_conclusion")), case.get("case_id"))
                    continue
                # Status match is necessary but not sufficient for equals.
                if bool(case.get("equals")):
                    self.assertTrue(same_status, case.get("case_id"))
                    frozen_obs = self.by_id[case["case_id"]]
                    pr_row = next(r for r in self.pr_base["observations"] if r["case_id"] == case["case_id"])
                    self.assertEqual(
                        (frozen_obs.execution_evidence or {}).get("original_run", {}).get("stdout"),
                        pr_row.get("original_stdout"),
                        case.get("case_id"),
                    )
                    self.assertEqual(
                        (frozen_obs.execution_evidence or {}).get("rebuilt_run", {}).get("stdout"),
                        pr_row.get("rebuilt_stdout"),
                        case.get("case_id"),
                    )
            else:
                self.assertIsNone(case.get("equality_conclusion"), case.get("case_id"))
                self.assertNotEqual(case.get("equals"), True, case.get("case_id"))
        pr_unicode = [row for row in self.pr_base["observations"] if row["sample"] == "UnicodePair"]
        self.assertTrue(pr_unicode)
        for row in pr_unicode:
            self.assertEqual(row["status"], "fail")
            self.assertIn("behavior mismatch", str(row.get("failure") or ""))
            self.assertEqual(row.get("harness_digest"), protocol)

    def test_evidence_written_outside_tracked_repo_dumps(self) -> None:
        tracked = (ROOT / "tools" / "milestone_ledger" / "evidence").resolve()
        dest = self.evidence_dir.resolve()
        self.assertFalse(str(dest).startswith(str(tracked) + "/"), dest)
        self.assertNotEqual(dest, tracked)

    def test_no_home_user_paths_in_evidence_json(self) -> None:
        for name in (
            "observations.json",
            "summary.json",
            "frozen.json",
            "toolchain.json",
            "probe_build.json",
            "pr_base_81f8ef5_live.json",
            "compare_vs_pr_base_81f8ef5.json",
            "frozen_31de113.json",
        ):
            text = (self.evidence_dir / name).read_text(encoding="utf-8")
            self.assertNotIn(_HOME_LEAK, text, name)

    def test_reviewed_fixture_hashes_and_temp_class_paths(self) -> None:
        for name, expected in SEED_SOURCE_SHA256.items():
            path = ROOT / "tools" / "milestone_ledger" / "fixtures" / f"{name}.java"
            got = hashlib.sha256(path.read_bytes()).hexdigest()
            self.assertEqual(got, expected, name)
        for obs in self.observations:
            ee = obs.execution_evidence or {}
            orig = str(ee.get("original_class_file") or "")
            rebuilt = str(ee.get("rebuilt_dir") or "")
            repo = str(ROOT.resolve())
            self.assertTrue(orig, obs.case_id)
            self.assertTrue(rebuilt, obs.case_id)
            self.assertFalse(str(Path(orig).resolve()).startswith(repo + os.sep), obs.case_id)
            self.assertFalse(str(Path(rebuilt).resolve()).startswith(repo + os.sep), obs.case_id)
            argv = (ee.get("original_compile") or {}).get("argv") or []
            self.assertIn("-proc:none", argv, obs.case_id)
            self.assertIn("--release", argv, obs.case_id)
            self.assertTrue("-g" in argv or "-g:none" in argv, obs.case_id)
            self.assertTrue(_SHA256.fullmatch(obs.evidence.input_hash), obs.case_id)

    def test_frozen_worktree_not_edited(self) -> None:
        self.assertFalse(self.summary["frozen_worktree_dirty"])
        self.assertFalse(self.frozen_doc["frozen_worktree_dirty"])
        tree = Path(self.frozen_doc["frozen_tree"])
        self.assertTrue(tree.is_dir(), tree)
        javajive = (tree / "javajive.go").read_text(encoding="utf-8")
        self.assertIn("func DecompileWithOptions(", javajive)
        for obs in self.observations:
            self.assertFalse(obs.extras.get("frozen_worktree_dirty"), obs.case_id)
            self.assertFalse(obs.extras.get("head_is_pr_base"), obs.case_id)

    def test_counts_never_tally_infra_as_pass(self) -> None:
        counts = self.summary["counts"]
        statuses = self.summary["statuses"]
        self.assertEqual(counts["live_api"], _MATRIX)
        self.assertEqual(sum(1 for s in statuses.values() if s == "pass"), counts["pass"])
        self.assertEqual(sum(1 for s in statuses.values() if s == "fail"), counts["fail"])
        self.assertEqual(sum(1 for s in statuses.values() if s == "infra_error"), counts["infra_error"])
        for case_id, status in statuses.items():
            if status == "infra_error":
                self.assertNotEqual(status, "pass", case_id)
            obs = self.by_id[case_id]
            if obs.status == "pass":
                self.assertTrue(is_semantic_pass(obs), case_id)
                self.assertNotEqual(obs.status, "infra_error", case_id)


if __name__ == "__main__":
    unittest.main()
