"""T30-C01: host canary (fake token) is invisible; mount inventory has no home/token/docker.sock."""

from __future__ import annotations

import os
import tempfile
import unittest
from pathlib import Path

from harness import assert_inventory_clean, obs_dict, worker, write_evidence


class TestTaskT30C01Credentials(unittest.TestCase):
    def test_T30_C01_host_canary_invisible_and_mount_inventory(self) -> None:
        w = worker()
        home_canary = Path.home() / ".t30-fake-token-not-a-secret"
        other_dir = Path(tempfile.mkdtemp(prefix="t30-c01-host-"))
        other_canary = other_dir / "github_token"
        payload = "T30-FAKE-TOKEN-NOT-A-SECRET"
        home_canary.write_text(payload, encoding="utf-8")
        other_canary.write_text(payload, encoding="utf-8")
        os.chmod(home_canary, 0o600)
        try:
            if w.backend is None:
                obs = w.probe_canary(home_canary)
                self.assertEqual(obs.reason, "isolation_unavailable")
                self.assertFalse(obs.did_execute)
                self.assertNotIn(payload, obs.stdout)
                write_evidence(
                    "T30-C01",
                    {
                        "backend": None,
                        "fail_closed": True,
                        "observation": obs_dict(obs),
                    },
                )
                return

            obs = w.probe_canary(home_canary)
            text = obs.stdout + "\n" + obs.stderr
            self.assertTrue(obs.did_execute)
            self.assertIsNotNone(obs.policy_digest)
            self.assertEqual(len(obs.policy_digest or ""), 64)
            self.assertNotIn(payload, text)
            self.assertNotIn("CANARY_VISIBLE=1", text)
            self.assertIn("CANARY_VISIBLE=0", text)
            self.assertNotIn("FOUND:/var/run/docker.sock", text)
            self.assertNotIn("FOUND:/run/docker.sock", text)
            assert_inventory_clean(self, obs.mount_inventory)

            obs2 = w.probe_canary(other_canary)
            text2 = obs2.stdout + "\n" + obs2.stderr
            self.assertNotIn(payload, text2)
            self.assertNotIn("CANARY_VISIBLE=1", text2)

            write_evidence(
                "T30-C01",
                {
                    "backend": w.backend,
                    "policy_digest": obs.policy_digest,
                    "home_canary": str(home_canary),
                    "other_canary": str(other_canary),
                    "observation": obs_dict(obs),
                    "observation_other": obs_dict(obs2),
                    "mount_inventory": obs.mount_inventory,
                },
            )
        finally:
            try:
                home_canary.unlink()
            except OSError:
                pass
            try:
                other_canary.unlink()
                other_dir.rmdir()
            except OSError:
                pass


if __name__ == "__main__":
    unittest.main()
