"""T01: reconstruct 31de113/81f8ef5 from git objects; fail closed if missing."""

from __future__ import annotations

import os
import tempfile
import unittest
from pathlib import Path

from common import ROOT
from tools.milestone_ledger.anchor_checkout import (
    ENV_FROZEN,
    AnchorCheckoutError,
    materialize_commit,
    require_commit,
    resolve_anchor_tree,
    verify_explicit_tree,
)
from tools.milestone_ledger.constants import PR_BASE_SHA as CONST_PR_BASE
from tools.milestone_ledger.frozen_replay import FROZEN_SHA
from tools.milestone_ledger.toolchain import probe_revision

_MISSING = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"


class TestAnchorCheckout(unittest.TestCase):
    def test_require_commit_fail_closed_when_object_missing(self) -> None:
        with self.assertRaises(AnchorCheckoutError) as ctx:
            require_commit(ROOT, _MISSING)
        self.assertIn("not in this clone", str(ctx.exception))
        self.assertIn("not skipped", str(ctx.exception))

    def test_explicit_missing_path_is_infra_not_skip(self) -> None:
        missing = Path(tempfile.gettempdir()) / "javajive-no-such-tree-does-not-exist"
        with self.assertRaises(AnchorCheckoutError) as ctx:
            verify_explicit_tree(missing, FROZEN_SHA)
        self.assertIn("explicit tree missing", str(ctx.exception))
        self.assertIn("not skipped", str(ctx.exception))

    def test_materialize_31de113_and_81f8ef5_from_repo_objects(self) -> None:
        frozen = materialize_commit(FROZEN_SHA, source_repo=ROOT)
        base = materialize_commit(CONST_PR_BASE, source_repo=ROOT)
        self.addCleanup(lambda: None)
        self.assertEqual(probe_revision(frozen), FROZEN_SHA)
        self.assertEqual(probe_revision(base), CONST_PR_BASE)
        self.assertTrue((frozen / "javajive.go").is_file())
        self.assertTrue((base / "javajive.go").is_file())
        self.assertNotEqual(frozen.resolve(), base.resolve())
        marker = frozen / "MUST_NOT_WRITE"
        # Caller must not treat this as a writable shared tree.
        self.assertFalse(marker.is_file())

    def test_explicit_wrong_sha_is_error(self) -> None:
        tree = materialize_commit(CONST_PR_BASE, source_repo=ROOT)
        with self.assertRaises(AnchorCheckoutError) as ctx:
            verify_explicit_tree(tree, FROZEN_SHA)
        self.assertIn("HEAD", str(ctx.exception))
        self.assertIn(FROZEN_SHA, str(ctx.exception))

    def test_env_explicit_is_honored_read_only(self) -> None:
        tree = materialize_commit(FROZEN_SHA, source_repo=ROOT)
        old = os.environ.get(ENV_FROZEN)
        os.environ[ENV_FROZEN] = str(tree)
        try:
            resolved = resolve_anchor_tree(FROZEN_SHA, source_repo=ROOT, env_key=ENV_FROZEN)
            self.assertEqual(resolved, tree.resolve())
            self.assertEqual(probe_revision(resolved), FROZEN_SHA)
        finally:
            if old is None:
                os.environ.pop(ENV_FROZEN, None)
            else:
                os.environ[ENV_FROZEN] = old


if __name__ == "__main__":
    unittest.main()
