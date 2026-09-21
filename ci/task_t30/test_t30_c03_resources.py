"""T30-C03: small budget limits fork/output/memory; no leftover host process tree."""

from __future__ import annotations

import unittest

from harness import obs_dict, worker, write_evidence
from tools.sandbox_worker.policy import Limits


class TestTaskT30C03Resources(unittest.TestCase):
    def test_T30_C03_limits_or_kill_and_no_leftover_children(self) -> None:
        w = worker()
        limits = Limits(
            timeout_seconds=6,
            memory_bytes=32 * 1024 * 1024,
            pids=8,
            output_bytes=16 * 1024,
            fsize_bytes=32 * 1024,
            artifact_bytes=32 * 1024,
            cpus="0.25",
            cpu_seconds=4,
        )
        if w.backend is None:
            obs = w.probe_resources(limits)
            self.assertEqual(obs.reason, "isolation_unavailable")
            self.assertFalse(obs.did_execute)
            write_evidence(
                "T30-C03",
                {"backend": None, "fail_closed": True, "observation": obs_dict(obs)},
            )
            return

        obs = w.probe_resources(limits)
        self.assertTrue(obs.did_execute)
        self.assertIsNotNone(obs.policy_digest)
        self.assertTrue(obs.resource_limits)
        self.assertIn("memory_bytes", obs.resource_limits)
        self.assertEqual(obs.resource_limits.get("memory_bytes"), limits.memory_bytes)
        self.assertEqual(obs.resource_limits.get("pids"), limits.pids)
        # Limited or predictably terminated — must not silently survive the full abuse.
        limited = (
            obs.timed_out
            or obs.output_capped
            or bool(obs.extra.get("killed"))
            or obs.status in {"timeout", "resource_limit", "blocked"}
            or (obs.exit_code not in (0, None))
        )
        self.assertTrue(limited, f"worker was not limited: status={obs.status} exit={obs.exit_code} out={obs.stdout[-400:]}")
        self.assertEqual(obs.leftover_host_pids, [])
        self.assertEqual(obs.leftover_containers, [])
        write_evidence(
            "T30-C03",
            {
                "backend": w.backend,
                "policy_digest": obs.policy_digest,
                "resource_limits": obs.resource_limits,
                "leftover_host_pids": obs.leftover_host_pids,
                "leftover_containers": obs.leftover_containers,
                "timed_out": obs.timed_out,
                "output_capped": obs.output_capped,
                "observation": obs_dict(obs),
            },
        )


if __name__ == "__main__":
    unittest.main()
