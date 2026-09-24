"""JAVAJIVE_EVIDENCE_DIR: tests must not mutate tracked repo evidence dumps."""

from __future__ import annotations

import os
import tempfile
from pathlib import Path

_ENV = "JAVAJIVE_EVIDENCE_DIR"


def evidence_root() -> Path:
    raw = os.environ.get(_ENV)
    if raw:
        root = Path(raw)
        root.mkdir(parents=True, exist_ok=True)
        return root
    root = Path(tempfile.mkdtemp(prefix="javajive-evidence-"))
    os.environ[_ENV] = str(root)
    return root


def evidence_subdir(*parts: str) -> Path:
    path = evidence_root().joinpath(*parts)
    path.mkdir(parents=True, exist_ok=True)
    return path
