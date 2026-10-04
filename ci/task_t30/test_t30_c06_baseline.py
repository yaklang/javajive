"""T30-C06: reviewed Baseline.java compiles and runs inside the worker; no false-positive deny-all."""

from __future__ import annotations

import hashlib
import unittest
from pathlib import Path

from harness import REPO_ROOT, obs_dict, worker, write_evidence

BASELINE = REPO_ROOT / "tools" / "sandbox_worker" / "fixtures" / "Baseline.java"


class TestTaskT30C06BaselineControl(unittest.TestCase):
    def test_T30_C06_baseline_compile_verify_run_in_worker(self) -> None:
        self.assertTrue(BASELINE.is_file())
        src = BASELINE.read_text(encoding="utf-8")
        self.assertIn("class Baseline", src)
        digest = hashlib.sha256(BASELINE.read_bytes()).hexdigest()
        w = worker()
        if w.backend is None:
            obs = w.compile_and_run_java(BASELINE)
            self.assertEqual(obs.reason, "isolation_unavailable")
            self.assertFalse(obs.did_execute)
            write_evidence(
                "T30-C06",
                {
                    "backend": None,
                    "fail_closed": True,
                    "host_limitation": True,
                    "baseline_sha256": digest,
                    "observation": obs_dict(obs),
                },
            )
            return

        if w.backend in {"docker", "podman"}:
            w.ensure_toolchain_image()
        obs = w.compile_and_run_java(BASELINE)
        text = (obs.stdout + "\n" + obs.stderr).strip()
        write_evidence(
            "T30-C06",
            {
                "backend": w.backend,
                "policy_digest": obs.policy_digest,
                "baseline_sha256": digest,
                "compiler": obs.compiler,
                "observation": obs_dict(obs),
            },
        )
        self.assertTrue(obs.did_execute)
        self.assertIsNotNone(obs.policy_digest)
        self.assertEqual(obs.status, "ok", text)
        self.assertEqual(obs.exit_code, 0, text)
        self.assertIn("\n7", "\n" + obs.stdout.replace("\r\n", "\n"))
        self.assertNotIn("-processor", " ".join(obs.argv))
        # Policy records javac -proc:none
        self.assertEqual((obs.policy or {}).get("javac_proc"), "none")


if __name__ == "__main__":
    unittest.main()
