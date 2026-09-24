"""CLI subcommands validate / compare / aggregate."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from common import MILESTONE, PR_BASE, ROOT, eight_api_pass, obs
from tools.milestone_ledger.cli import main
from tools.milestone_ledger.compare import compare_anchors


class TestTaskT01CLI(unittest.TestCase):
    def test_cli_validate_compare_aggregate(self) -> None:
        rows, manifest = eight_api_pass()
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            obs_path = tmp_path / "obs.json"
            man_path = tmp_path / "manifest.json"
            obs_path.write_text(json.dumps({"observations": [r.to_dict() for r in rows]}), encoding="utf-8")
            man_path.write_text(json.dumps(manifest), encoding="utf-8")
            self.assertEqual(main(["validate", "--observations", str(obs_path)]), 0)
            self.assertEqual(main(["aggregate", "--manifest", str(man_path), "--observations", str(obs_path)]), 0)

            dropped = rows[:-1]
            drop_path = tmp_path / "drop.json"
            drop_path.write_text(json.dumps({"observations": [r.to_dict() for r in dropped]}), encoding="utf-8")
            self.assertEqual(main(["aggregate", "--manifest", str(man_path), "--observations", str(drop_path)]), 1)

            cand = tmp_path / "cand.json"
            base = tmp_path / "base.json"
            mile = tmp_path / "mile.json"
            item = obs(sample="Cap", status="fail", revision=PR_BASE)
            mile_item = obs(sample="Cap", status="pass", revision=MILESTONE)
            for path, payload in (
                (cand, [item]),
                (base, [item]),
                (mile, [mile_item]),
            ):
                path.write_text(json.dumps({"observations": [p.to_dict() for p in payload]}), encoding="utf-8")
            rc = main(["compare", "--candidate", str(cand), "--pr-base", str(base), "--milestone", str(mile)])
            self.assertEqual(rc, 1)
            compared = compare_anchors([item], [item], [mile_item])
            self.assertEqual(compared["overall_verdict"], "long_term_drift")

    def test_cli_historical_validate(self) -> None:
        rc = main(["validate", "--historical"])
        self.assertEqual(rc, 0)

    def test_cli_module_and_main_py_entrypoints(self) -> None:
        module = subprocess.run(
            [sys.executable, "-m", "tools.milestone_ledger", "validate", "--historical"],
            cwd=ROOT,
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(module.returncode, 0, module.stderr)
        script = subprocess.run(
            [sys.executable, str(ROOT / "tools" / "milestone_ledger" / "__main__.py"), "validate", "--historical"],
            cwd=ROOT,
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(script.returncode, 0, script.stderr)

    def test_cli_corrupt_json_validate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "bad.json"
            path.write_text("{not json", encoding="utf-8")
            rc = main(["validate", "--observations", str(path)])
            self.assertNotEqual(rc, 0)


if __name__ == "__main__":
    unittest.main()
