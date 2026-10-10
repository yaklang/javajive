"""CLI: python3 -m tools.sandbox_worker selfcheck"""

from __future__ import annotations

import json
import sys
from pathlib import Path

from tools.evidence_paths import evidence_subdir

from .detect import detect
from .executor import SandboxWorker
from .policy import Limits


def _evidence_dir() -> Path:
    return evidence_subdir("t30")


def selfcheck() -> int:
    det = detect()
    worker = SandboxWorker()
    payload = {
        "detection": det.as_dict(),
        "selected": worker.backend,
        "fail_closed": worker.backend is None,
        "note": "subprocess+timeout is never a backend",
    }
    out = _evidence_dir() / "backend.json"
    out.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps(payload, indent=2, sort_keys=True))
    if worker.backend is None:
        obs = worker.run(__import__("tools.sandbox_worker.job", fromlist=["UntrustedJob"]).UntrustedJob(argv=["/bin/true"]))
        print("fail-closed observation:", obs.status, obs.reason)
        return 0
    return 0


def main(argv: list[str] | None = None) -> int:
    argv = list(sys.argv[1:] if argv is None else argv)
    if not argv or argv[0] in {"selfcheck", "detect"}:
        return selfcheck()
    print("usage: python3 -m tools.sandbox_worker selfcheck", file=sys.stderr)
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
