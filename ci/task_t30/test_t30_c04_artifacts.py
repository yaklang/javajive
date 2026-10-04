"""T30-C04: ../, symlink-out, and huge logs cannot escape the artifact root; byte cap enforced."""

from __future__ import annotations

import os
import tempfile
import unittest
from pathlib import Path

from harness import obs_dict, worker, write_evidence
from tools.sandbox_worker.artifacts import ArtifactEscape, directory_size, write_bytes
from tools.sandbox_worker.policy import Limits


class TestTaskT30C04ArtifactEscape(unittest.TestCase):
    def test_T30_C04_path_escape_symlink_and_byte_cap(self) -> None:
        root = Path(tempfile.mkdtemp(prefix="t30-c04-art-"))
        parent = root.parent
        sentinel = parent / "t30-c04-outside-sentinel.txt"
        sentinel.write_text("UNCHANGED", encoding="utf-8")
        try:
            with self.assertRaises(ArtifactEscape):
                write_bytes(root, "../escape.txt", b"pwned", cap=4096)
            with self.assertRaises(ArtifactEscape):
                write_bytes(root, "/tmp/escape.txt", b"pwned", cap=4096)
            ok = write_bytes(root, "inside.txt", b"ok", cap=4096)
            self.assertTrue(ok.is_file())
            self.assertEqual(ok.read_bytes(), b"ok")

            # symlink pointing outside
            link = root / "outlink"
            try:
                link.symlink_to(sentinel)
            except OSError:
                link = None
            if link is not None:
                with self.assertRaises(ArtifactEscape):
                    write_bytes(root, "outlink", b"pwned", cap=4096)
            self.assertEqual(sentinel.read_text(encoding="utf-8"), "UNCHANGED")

            # byte cap on host-mediated write
            with self.assertRaises(ArtifactEscape):
                write_bytes(root, "huge.txt", b"x" * 8192, cap=1024)

            w = worker()
            limits = Limits(
                timeout_seconds=8,
                memory_bytes=64 * 1024 * 1024,
                pids=16,
                output_bytes=32 * 1024,
                fsize_bytes=16 * 1024,
                artifact_bytes=16 * 1024,
            )
            if w.backend is None:
                obs = w.probe_artifact_escape(limits)
                self.assertEqual(obs.reason, "isolation_unavailable")
                self.assertFalse(obs.did_execute)
                write_evidence(
                    "T30-C04",
                    {"backend": None, "fail_closed": True, "observation": obs_dict(obs)},
                )
                return

            obs = w.probe_artifact_escape(limits)
            self.assertTrue(obs.did_execute)
            self.assertIsNotNone(obs.policy_digest)
            stage = Path(obs.extra["stage"])
            art = Path(obs.extra["artifact_root"])
            self.assertFalse((stage / "escape.txt").exists(), "dot-dot write escaped artifact root")
            self.assertEqual(sentinel.read_text(encoding="utf-8"), "UNCHANGED")
            # byte cap actually applied
            self.assertLessEqual(obs.artifact_bytes, limits.artifact_bytes)
            self.assertLessEqual(directory_size(art), limits.artifact_bytes)
            huge = art / "huge.bin"
            if huge.exists():
                self.assertLessEqual(huge.stat().st_size, limits.fsize_bytes + 4096)
            write_evidence(
                "T30-C04",
                {
                    "backend": w.backend,
                    "policy_digest": obs.policy_digest,
                    "sentinel_unchanged": True,
                    "escape_exists": (stage / "escape.txt").exists(),
                    "artifact_bytes": obs.artifact_bytes,
                    "artifact_cap": limits.artifact_bytes,
                    "artifact_capped": obs.artifact_capped,
                    "observation": obs_dict(obs),
                },
            )
        finally:
            try:
                sentinel.unlink()
            except OSError:
                pass
            for dirpath, dirnames, filenames in os.walk(root, topdown=False):
                for name in filenames:
                    try:
                        os.unlink(os.path.join(dirpath, name))
                    except OSError:
                        pass
                for name in dirnames:
                    try:
                        os.rmdir(os.path.join(dirpath, name))
                    except OSError:
                        pass
            try:
                os.rmdir(root)
            except OSError:
                pass


if __name__ == "__main__":
    unittest.main()
