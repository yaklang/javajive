"""T29 helper: execution-event complete187 mapping is fail-closed."""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from tools.ci_scheduler.execution_map import (  # noqa: E402
    PACK_CASE_IDS,
    RequiredCommand,
    command_outcome,
    evaluate_inventory,
    mention_is_not_coverage,
    parse_execution_events,
)
from tools.ci_scheduler.workflow_inventory import BASELINE_CI_JOBS, required_coverage_snapshot  # noqa: E402


class TestExecutionEventMapping(unittest.TestCase):
    def test_pack_has_187_unique_ids(self) -> None:
        self.assertEqual(len(PACK_CASE_IDS), 187)
        self.assertEqual(len(set(PACK_CASE_IDS)), 187)

    def test_string_mention_is_not_coverage(self) -> None:
        log = (
            "inventory said T09-C06 PASS because the identifier appears here\n"
            "also TestT09_C06 truncated name is not the real test\n"
            "ok  \tgithub.com/yaklang/javajive/classparser\n"
        )
        events = parse_execution_events(log)
        self.assertFalse(any(ev.test_name == "TestT09_C06_APIIrreducibleDiagnostic" for ev in events))
        inv = evaluate_inventory(log)
        row = next(c for c in inv["cases"] if c["case_id"] == "T09-C06")
        self.assertNotEqual(row["status"], "PASS")
        self.assertFalse(row["pass"])
        self.assertTrue(mention_is_not_coverage(log, "T09-C06"))

    def test_truncated_go_name_is_not_a_match(self) -> None:
        required = RequiredCommand(kind="go_test", test_name="TestT09_C06_APIIrreducibleDiagnostic", package="./test/cross")
        log = "=== RUN   TestT09_C06\n--- PASS: TestT09_C06 (0.01s)\n"
        events = parse_execution_events(log)
        self.assertEqual(command_outcome(required, events), "NOT_RUN")
        full = (
            "=== RUN   TestT09_C06_APIIrreducibleDiagnostic\n"
            "--- PASS: TestT09_C06_APIIrreducibleDiagnostic (0.20s)\n"
        )
        self.assertEqual(command_outcome(required, parse_execution_events(full)), "PASS")

    def test_unrun_and_zero_match_are_fail_closed(self) -> None:
        log = "ok  \tgithub.com/yaklang/javajive/test/cross\t0.012s [no tests to run]\n"
        inv = evaluate_inventory(log)
        self.assertEqual(inv["counts"]["total"], 187)
        self.assertEqual(inv["counts"]["pass"], 0)
        self.assertFalse(inv["overall_pass"])
        missing = next(c for c in inv["cases"] if c["case_id"] == "T10-C01")
        self.assertIn(missing["status"], {"NOT_RUN", "NOT_MAPPED"})
        self.assertFalse(missing["pass"])

    def test_python_ok_event_counts_only_exact_method(self) -> None:
        log = (
            "test_T30_C01_host_canary_invisible_and_mount_inventory "
            "(ci.task_t30.test_t30_c01_credentials.TestTaskT30C01Credentials) ... ok\n"
        )
        inv = evaluate_inventory(log)
        row = next(c for c in inv["cases"] if c["case_id"] == "T30-C01")
        self.assertEqual(row["status"], "PASS")
        neighbor = next(c for c in inv["cases"] if c["case_id"] == "T30-C02")
        self.assertNotEqual(neighbor["status"], "PASS")

    def test_required_ci_jobs_unchanged(self) -> None:
        snap = required_coverage_snapshot(ROOT)
        for job in BASELINE_CI_JOBS:
            self.assertIn(job, snap["jobs"]["ci.yml"])


if __name__ == "__main__":
    unittest.main()
