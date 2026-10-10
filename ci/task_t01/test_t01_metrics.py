"""T01-M01/M02/M03 metric contracts."""

from __future__ import annotations

import unittest

from common import eight_api_pass, obs
from tools.milestone_ledger import aggregate
from tools.milestone_ledger.constants import REQUIRED_EVIDENCE_FIELDS


class TestTaskT01Metrics(unittest.TestCase):
    def test_T01_M01_selected_inventory_coverage_100_zero_duplicates_missing(self) -> None:
        rows, manifest = eight_api_pass()
        result = aggregate(manifest, rows, strict=True)
        observed = result.metrics["T01-M01"]["observed"]
        self.assertEqual(observed["coverage"], 1.0)
        self.assertEqual(observed["duplicates"], 0)
        self.assertEqual(observed["missing"], 0)
        self.assertEqual(observed["denominator"], 8)
        self.assertEqual(result.metrics["T01-M01"]["verdict"], "pass")

    def test_T01_M02_every_result_has_input_toolchain_mode_harness_revision(self) -> None:
        rows, manifest = eight_api_pass()
        result = aggregate(manifest, rows, strict=True)
        self.assertEqual(result.metrics["T01-M02"]["verdict"], "pass")
        self.assertEqual(result.metrics["T01-M02"]["observed"]["missing_evidence"], 0)
        for item in rows:
            evidence = item.evidence.to_dict()
            for key in REQUIRED_EVIDENCE_FIELDS:
                self.assertTrue(evidence.get(key), f"T01-M02 missing {key} on {item.case_id}")
            self.assertTrue(evidence["input_hash"])
            self.assertTrue(str(evidence["compiler_version"]).startswith("javac "))
            self.assertTrue(evidence["mode"])
            self.assertTrue(evidence["harness_digest"])
            self.assertEqual(len(evidence["revision"]), 40)

    def test_T01_M03_fail_unsupported_infra_error_not_counted_as_pass(self) -> None:
        rows, manifest = eight_api_pass()
        mutated = [
            obs(
                sample=item.sample,
                mode=item.evidence.mode,
                debug=item.evidence.debug,
                status="fail" if i == 0 else "unsupported" if i == 1 else "infra_error" if i == 2 else item.status,
            )
            for i, item in enumerate(rows)
        ]
        result = aggregate(manifest, mutated, strict=False)
        self.assertEqual(result.metrics["T01-M03"]["observed"]["miscounted_as_pass"], 0)
        self.assertEqual(result.metrics["T01-M03"]["verdict"], "pass")
        for cid in mutated[0].case_id, mutated[1].case_id, mutated[2].case_id:
            self.assertNotIn(cid, result.pass_ids)
        naive = sum(1 for item in mutated if item.status in {"pass", "fail", "unsupported", "infra_error"} and item.status != "fail")
        # Explicit: semantic fail/unsupported/infra_error are not in pass_ids.
        self.assertTrue(all(item.status != "pass" or item.case_id in result.pass_ids for item in mutated))
        self.assertEqual(len(result.pass_ids), 5)
        self.assertNotEqual(len(result.pass_ids), naive)


if __name__ == "__main__":
    unittest.main()
