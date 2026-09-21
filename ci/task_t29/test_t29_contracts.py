"""Executable T29 contracts. Names include T29-Cxx."""

from __future__ import annotations

import json
import os
import subprocess
import sys
import time
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from tools.ci_scheduler.cache import accept_baseline_artifact, cache_key
from tools.ci_scheduler.envcheck import require_go_matches, require_jdk
from tools.ci_scheduler.gates import CaseObservation, evaluate_local_gates, promote_capability
from tools.ci_scheduler.schedule import compare_schedules
from tools.ci_scheduler.shard import ShardError, TestItem, expand_manifest, shard_items
from tools.ci_scheduler.workflow_inventory import BASELINE_CI_JOBS, action_pins, required_coverage_snapshot

from tools.evidence_paths import evidence_subdir

EVIDENCE = evidence_subdir("t29")


class T29Contracts(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        EVIDENCE.mkdir(parents=True, exist_ok=True)

    def test_T29_C01_shard_inventory_is_conserved(self) -> None:
        """T29-C01: long names, determinism reps covered exactly; duplicate names are errors."""
        long_name = "Test" + ("VeryLongIdent" * 40) + "Suffix"
        items = [
            TestItem(long_name, duration_ns=5_000_000_000, repetitions=1),
            TestItem("TestDecompileSyntaxRegression", duration_ns=8_000_000_000, repetitions=1),
            TestItem("TestRegressionSeedsAreDeterministic", duration_ns=12_000_000_000, repetitions=8),
            TestItem("TestTiny", duration_ns=10_000_000, repetitions=1),
        ]
        expanded = expand_manifest(items)
        self.assertEqual(len(expanded), 1 + 1 + 8 + 1)
        self.assertIn(long_name, expanded)
        self.assertEqual(sum(1 for n in expanded if n.startswith("TestRegressionSeedsAreDeterministic")), 8)
        shards = shard_items(items, 4)
        covered = [n for s in shards for n in s.items]
        self.assertEqual(sorted(covered), sorted(expanded))
        self.assertEqual(len(covered), len(set(covered)))
        self.assertTrue(all(s.items for s in shards))
        with self.assertRaises(ShardError):
            expand_manifest(items + [TestItem("TestTiny", duration_ns=1)])
        with self.assertRaises(ShardError):
            shard_items(items, 0)
        (EVIDENCE / "t29_c01_shards.json").write_text(
            json.dumps({"expanded": expanded, "shards": [s.to_dict() for s in shards]}, indent=2) + "\n"
        )

    def test_T29_C02_duration_schedule_uses_measured_times(self) -> None:
        """T29-C02: real go test durations vs round-robin; report max-shard/total; no CI-time promise."""
        env = os.environ.copy()
        env["CGO_ENABLED"] = "1"
        env["GOTOOLCHAIN"] = "local"
        packages = ["./internal/log", "./internal/funk", "./internal/memfile", "./internal/utils"]
        measured: list[TestItem] = []
        raw_runs = []
        for pkg in packages:
            t0 = time.perf_counter_ns()
            proc = subprocess.run(
                ["go", "test", pkg, "-count=1", "-timeout=60s", "-ldflags=-linkmode=external"],
                cwd=ROOT,
                env=env,
                capture_output=True,
                text=True,
                timeout=90,
            )
            elapsed = time.perf_counter_ns() - t0
            raw_runs.append(
                {
                    "package": pkg,
                    "rc": proc.returncode,
                    "ns": elapsed,
                    "stdout_tail": proc.stdout[-500:],
                    "stderr_tail": proc.stderr[-500:],
                }
            )
            self.assertEqual(proc.returncode, 0, msg=f"{pkg} failed: {proc.stdout}\n{proc.stderr}")
            measured.append(TestItem(pkg, duration_ns=elapsed, repetitions=1))
        # Inject a long synthetic sibling using the max measured duration * 8 so packing has work to do.
        measured.append(
            TestItem(
                "TestSyntheticLongDeterminism",
                duration_ns=max(it.duration_ns for it in measured) * 8,
                repetitions=4,
            )
        )
        report = compare_schedules(measured, 3)
        self.assertTrue(report["measured"])
        self.assertIsNone(report["improvement_claimed_percent"])
        self.assertGreater(report["packed"]["max_shard_ns"], 0)
        self.assertGreater(report["round_robin"]["max_shard_ns"], 0)
        self.assertEqual(report["n_expanded"], sum(i.repetitions for i in measured))
        (EVIDENCE / "t29_c02_schedule.json").write_text(
            json.dumps({"raw_runs": raw_runs, "report": report}, indent=2) + "\n"
        )

    def test_T29_C03_local_gate_does_not_hide_unrepaired_failures(self) -> None:
        """T29-C03: repairing T02/T03 still reports T04 fail; overall is not all-pass."""
        obs = [
            CaseObservation("T02-C01", "T02", "pass", gate=True),
            CaseObservation("T03-C01", "T03", "pass", gate=True),
            CaseObservation("T04-C01", "T04", "fail", gate=False),
            CaseObservation("T04-C02", "T04", "fail", gate=False),
        ]
        report = evaluate_local_gates(obs, {"T02", "T03"})
        self.assertTrue(report.local_gate_pass)
        self.assertFalse(report.overall_pass)
        self.assertIn("T04-C01", report.reported_failures)
        self.assertIn("T04-C02", report.reported_failures)
        self.assertEqual(report.hidden_failures, [])
        self.assertEqual(report.local_failures, [])
        broken_local = evaluate_local_gates(
            [CaseObservation("T02-C01", "T02", "fail", True), CaseObservation("T04-C01", "T04", "fail", False)],
            {"T02"},
        )
        self.assertFalse(broken_local.local_gate_pass)
        observe = evaluate_local_gates(
            [CaseObservation("T20-C01", "T20", "observe", False)],
            {"T20"},
        )
        self.assertFalse(observe.overall_pass)
        self.assertIn("T20-C01", observe.observe_counted_as_pass)
        (EVIDENCE / "t29_c03_gates.json").write_text(json.dumps(report.to_dict(), indent=2) + "\n")

    def test_T29_C04_pr_baseline_cache_is_untrusted(self) -> None:
        """T29-C04: PR-forged baseline or harness change with reused key is incomparable."""
        expected = cache_key(
            revision="81f8ef55603419bf737f10aa36e582f94619b35d",
            harness_digest="abc123",
            compiler="javac 21.0.2",
            flags="-ldflags=-linkmode=external",
            deps_digest="lock1",
            mode="compatibility",
            policy_digest="sandbox-v1",
        )
        pr = accept_baseline_artifact(
            expected_key=expected,
            artifact_key=expected,
            artifact_source="pull_request",
            trusted_revisions={"81f8ef55603419bf737f10aa36e582f94619b35d"},
            artifact_revision="81f8ef55603419bf737f10aa36e582f94619b35d",
            harness_changed=False,
        )
        self.assertFalse(pr.trusted)
        self.assertFalse(pr.comparable)
        self.assertEqual(pr.reason, "pr_supplied_baseline_untrusted")
        reused = accept_baseline_artifact(
            expected_key=expected,
            artifact_key=expected,
            artifact_source="ci_main",
            trusted_revisions={"81f8ef55603419bf737f10aa36e582f94619b35d"},
            artifact_revision="81f8ef55603419bf737f10aa36e582f94619b35d",
            harness_changed=True,
        )
        self.assertEqual(reused.reason, "harness_changed_but_key_reused")
        self.assertEqual(reused.to_dict()["status"], "incomparable")
        good = accept_baseline_artifact(
            expected_key=expected,
            artifact_key=expected,
            artifact_source="ci_main",
            trusted_revisions={"81f8ef55603419bf737f10aa36e582f94619b35d"},
            artifact_revision="81f8ef55603419bf737f10aa36e582f94619b35d",
            harness_changed=False,
        )
        self.assertTrue(good.trusted)
        (EVIDENCE / "t29_c04_cache.json").write_text(
            json.dumps({"expected": expected, "pr": pr.to_dict(), "reused": reused.to_dict()}, indent=2) + "\n"
        )

    def test_T29_C05_missing_jdk_or_zero_matches_are_infra_error(self) -> None:
        """T29-C05: JDK 17 running a JDK21 case, or go test regex zero-match, is infra_error not skip/pass."""
        low = require_jdk(21, "17.0.11")
        self.assertEqual(low.status, "infra_error")
        self.assertFalse(low.job_green)
        missing = require_jdk(21, None)
        self.assertEqual(missing.status, "infra_error")
        ok = require_jdk(17, "21.0.2")
        self.assertEqual(ok.status, "pass")
        self.assertTrue(ok.job_green)
        zero = require_go_matches(0, "^TestDoesNotExist$")
        self.assertEqual(zero.status, "infra_error")
        self.assertFalse(zero.job_green)
        # Prove a real go test -list zero-match on this repo.
        env = os.environ.copy()
        env["CGO_ENABLED"] = "1"
        env["GOTOOLCHAIN"] = "local"
        proc = subprocess.run(
            ["go", "test", "./internal/log", "-list", "^TestThisNameIsNotARealTest$", "-ldflags=-linkmode=external"],
            cwd=ROOT,
            env=env,
            capture_output=True,
            text=True,
            timeout=60,
        )
        listed = [ln for ln in proc.stdout.splitlines() if ln.startswith("Test")]
        self.assertEqual(listed, [])
        self.assertEqual(require_go_matches(len(listed), "^TestThisNameIsNotARealTest$").status, "infra_error")
        (EVIDENCE / "t29_c05_env.json").write_text(
            json.dumps(
                {
                    "jdk17": low.status,
                    "zero_match": zero.status,
                    "go_list_stdout": proc.stdout,
                    "go_list_stderr": proc.stderr,
                },
                indent=2,
            )
            + "\n"
        )

    def test_T29_C06_observe_to_gate_requires_review(self) -> None:
        """T29-C06: Record observe→pass needs review; rollback or missing case fails."""
        inventory = {"T20-C01", "T20-C02"}
        no_review = promote_capability(
            "T20-C01", from_status="observe", to_status="pass", review_id=None, inventory=inventory
        )
        self.assertFalse(no_review["ok"])
        self.assertEqual(no_review["reason"], "promotion_requires_review")
        ok = promote_capability(
            "T20-C01", from_status="observe", to_status="pass", review_id="review-t20-1", inventory=inventory
        )
        self.assertTrue(ok["ok"])
        rollback = promote_capability(
            "T20-C01", from_status="pass", to_status="observe", review_id=None, inventory=inventory
        )
        self.assertFalse(rollback["ok"])
        missing = promote_capability(
            "T20-C99", from_status="observe", to_status="pass", review_id="review-x", inventory=inventory
        )
        self.assertEqual(missing["reason"], "missing_case")
        snap = required_coverage_snapshot(ROOT)
        for job in BASELINE_CI_JOBS:
            self.assertIn(job, snap["jobs"]["ci.yml"], msg="T29 must not drop required CI jobs")
        gates = ROOT / ".github" / "workflows" / "task-gates.yml"
        self.assertTrue(gates.is_file(), "T29 adds task-gates.yml without replacing ci.yml")
        pins = action_pins(gates)
        self.assertTrue(pins)
        self.assertTrue(all(row["pinned"] for row in pins), msg=pins)
        text = gates.read_text(encoding="utf-8")
        self.assertIn("JAVA8_HOME", text)
        self.assertIn("fetch_ecj.py", text)
        self.assertIn("java-version: '8'", text)
        (EVIDENCE / "t29_c06_upgrade.json").write_text(
            json.dumps({"no_review": no_review, "ok": ok, "rollback": rollback, "ci_jobs": snap, "task_gates_pins": pins}, indent=2) + "\n"
        )


if __name__ == "__main__":
    unittest.main()
