"""The classparser suite runs only when its build inputs or fixtures changed."""

import unittest
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[2]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from tools.ci_scheduler.scope import changed_files, decide, requires_classparser


DEPS = {"classparser", "classparser/decompiler/core", "internal/codec", "internal/filesys"}


class ScopeDecision(unittest.TestCase):
    def test_dex2jar_only_diff_skips_the_suite(self):
        self.assertFalse(requires_classparser(["dex2jar/lower/method.go", "inspect/analyze.go"], DEPS))

    def test_empty_or_unknown_diff_fails_closed(self):
        self.assertTrue(requires_classparser([], DEPS))
        self.assertEqual(changed_files("", "HEAD"), [])
        self.assertEqual(changed_files("0000000", "HEAD"), [])

    def test_classparser_and_dependency_files_require_the_suite(self):
        for path in ("classparser/code_analyser.go", "classparser/decompiler/core/opcode.go",
                     "internal/codec/decode.go", "internal/filesys/fs.go"):
            with self.subTest(path=path):
                self.assertTrue(requires_classparser([path], DEPS))

    def test_shared_build_and_fixture_inputs_require_the_suite(self):
        for path in ("go.mod", "go.sum", ".github/workflows/ci.yml",
                     "tools/ci_scheduler/scope.py", "ci/task_t29/source-duration-hints.json",
                     "test/cross/testdata/historical-jars.lock.json"):
            with self.subTest(path=path):
                self.assertTrue(requires_classparser([path], DEPS))

    def test_directory_membership_uses_path_components(self):
        self.assertFalse(requires_classparser(["classparser2/foo.go", "internal/codecx/foo.go"], DEPS))
        self.assertFalse(requires_classparser(["docs/classparser.md"], DEPS))

    def test_manual_dispatch_and_full_label_force_the_suite(self):
        self.assertTrue(decide("workflow_dispatch", "", "HEAD", ""))
        self.assertTrue(decide("pull_request", "", "HEAD", "ci/full"))
        self.assertTrue(decide("pull_request", "", "HEAD", "triage,ci/full"))

    def test_dex2jar_pull_request_with_real_diff_decides_by_closure(self):
        # An empty base is unknown, so the decision must fail closed.
        self.assertTrue(decide("pull_request", "0000000000000000000000000000000000000000", "HEAD", ""))


if __name__ == "__main__":
    unittest.main()
