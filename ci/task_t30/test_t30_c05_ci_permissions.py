"""T30-C05: workflow permissions, persist-credentials, and 40-char action SHA pins. Missing policy fails closed."""

from __future__ import annotations

import unittest
from pathlib import Path

from harness import REPO_ROOT, write_evidence
from tools.sandbox_worker.ci_policy import classify_uses, review_ok, review_workflow_text, review_workflows_dir


class TestTaskT30C05CiPermissions(unittest.TestCase):
    def test_T30_C05_pins_permissions_and_fail_closed(self) -> None:
        wf_dir = REPO_ROOT / ".github" / "workflows"
        reviews = review_workflows_dir(wf_dir)
        ok, errors = review_ok(reviews)
        payload = {
            "workflows": [
                {
                    "path": r.path,
                    "ok": r.ok,
                    "errors": r.errors,
                    "uses": r.uses,
                    "pinned_uses": r.pinned_uses,
                    "unpinned_uses": r.unpinned_uses,
                    "permissions_present": r.permissions_present,
                    "has_pull_request": r.has_pull_request,
                    "has_pull_request_target": r.has_pull_request_target,
                    "checkout_steps": r.checkout_steps,
                    "checkout_persist_false": r.checkout_persist_false,
                }
                for r in reviews
            ],
            "errors": errors,
        }
        write_evidence("T30-C05", payload)
        self.assertTrue(reviews, "no workflows reviewed")
        names = {Path(r.path).name for r in reviews}
        self.assertIn("ci.yml", names)
        self.assertIn("deploy-pages.yml", names)
        self.assertTrue(ok, "workflow policy errors:\n" + "\n".join(errors))
        for r in reviews:
            self.assertTrue(r.permissions_present, r.path)
            self.assertFalse(r.has_pull_request_target, r.path)
            self.assertEqual(r.unpinned_uses, 0, r.uses)
            self.assertEqual(r.checkout_steps, r.checkout_persist_false, r.path)
            for uses in r.uses:
                kind = classify_uses(uses)
                self.assertIn(kind, {"pinned_sha", "pinned_digest", "local"}, uses)
                if kind == "pinned_sha":
                    sha = uses.rsplit("@", 1)[1]
                    self.assertEqual(len(sha), 40)

        missing = """
name: bad
on:
  pull_request:
jobs:
  t:
    runs-on: ubuntu-22.04
    steps:
      - uses: actions/checkout@v4
"""
        bad = review_workflow_text(missing, "<missing-policy>")
        self.assertFalse(bad.ok)
        self.assertTrue(any("permissions" in e or "unpinned" in e or "persist-credentials" in e for e in bad.errors))

        prt = """
name: bad2
on: pull_request_target
permissions:
  contents: write
jobs:
  t:
    runs-on: ubuntu-22.04
    steps:
      - uses: actions/checkout@11d5960a326750d5838078e36cf38b85af677262
"""
        bad2 = review_workflow_text(prt, "<prt>")
        self.assertFalse(bad2.ok)


if __name__ == "__main__":
    unittest.main()
