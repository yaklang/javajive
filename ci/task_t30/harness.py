"""Shared helpers for T30 contract tests. Test method names live in test_*.py."""

from __future__ import annotations

import json
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
if str(REPO_ROOT) not in sys.path:
    sys.path.insert(0, str(REPO_ROOT))

from tools.evidence_paths import evidence_subdir  # noqa: E402
from tools.sandbox_worker.executor import SandboxWorker  # noqa: E402
from tools.sandbox_worker.policy import Limits  # noqa: E402

EVIDENCE_DIR = evidence_subdir("t30")


def worker() -> SandboxWorker:
    return SandboxWorker()


def write_evidence(case_id: str, payload: dict) -> Path:
    EVIDENCE_DIR.mkdir(parents=True, exist_ok=True)
    path = EVIDENCE_DIR / f"{case_id}.json"
    path.write_text(json.dumps(payload, indent=2, sort_keys=True, default=str) + "\n", encoding="utf-8")
    return path


def obs_dict(obs) -> dict:
    return obs.as_dict()


def assert_inventory_clean(test, inv: dict) -> None:
    sources = inv.get("host_bind_sources") or []
    dests = inv.get("container_destinations") or []
    blob = " ".join(sources + dests).lower()
    test.assertFalse(inv.get("host_home_mounted"), "home must not be mounted")
    test.assertFalse(inv.get("docker_sock_mounted"), "docker.sock must not be mounted")
    test.assertNotIn("docker.sock", blob)
    home = str(Path.home().resolve()).lower()
    for src in sources:
        test.assertFalse(str(Path(src).resolve()).lower().startswith(home + "/"), f"mounted home path {src}")
        test.assertNotEqual(str(Path(src).resolve()).lower(), home)
    test.assertTrue(inv.get("denied_host_paths"), "denied path inventory required")
    denied = " ".join(inv.get("denied_host_paths") or [])
    test.assertIn(str(Path.home()), denied)
    test.assertTrue(any("docker.sock" in p for p in inv.get("denied_host_paths") or []))
