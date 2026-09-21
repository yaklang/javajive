"""Shared synthetic observation builders for T01 contract tests."""

from __future__ import annotations

import hashlib
import sys
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[2]
_TEST_DIR = Path(__file__).resolve().parent
for _path in (str(_TEST_DIR), str(ROOT)):
    if _path not in sys.path:
        sys.path.insert(0, _path)

from tools.milestone_ledger import parse_observation
from tools.milestone_ledger.constants import MILESTONE_SHA, PR_BASE_SHA

HARNESS = hashlib.sha256(b"t01-test-harness-v1").hexdigest()
LOCK = hashlib.sha256(b"t01-test-dep-lock-v1").hexdigest()
INPUT_A = hashlib.sha256(b"t01-input-A").hexdigest()
COMPILER = "javac 21.0.11"


def digest(blob: bytes) -> str:
    return hashlib.sha256(blob).hexdigest()


def observation_dict(
    *,
    sample: str = "CaseA",
    mode: str = "precision",
    debug: str = "debug",
    status: str = "pass",
    input_hash: str = INPUT_A,
    compiler_version: str = COMPILER,
    release: int = 8,
    output_target: str | None = None,
    dependency_lock_digest: str = LOCK,
    harness_digest: str = HARNESS,
    revision: str = PR_BASE_SHA,
    source: str = "synthetic",
    execution_evidence: Any = ...,
    historical_raw: dict[str, Any] | None = None,
    xfail: bool = False,
    skip: bool = False,
    notes: str = "",
    interface: str | None = None,
) -> dict[str, Any]:
    if execution_evidence is ...:
        execution_evidence = {"executed": True, "kind": "synthetic"} if status == "pass" else None
    payload: dict[str, Any] = {
        "sample": sample,
        "status": status,
        "source": source,
        "evidence": {
            "input_hash": input_hash,
            "compiler_version": compiler_version,
            "release": release,
            "debug": debug,
            "mode": mode,
            "output_target": output_target or mode,
            "dependency_lock_digest": dependency_lock_digest,
            "harness_digest": harness_digest,
            "revision": revision,
        },
        "execution_evidence": execution_evidence,
        "historical_raw": historical_raw,
        "xfail": xfail,
        "skip": skip,
        "notes": notes,
    }
    if interface is not None:
        payload["interface"] = interface
    return payload


def obs(**kwargs: Any):
    return parse_observation(observation_dict(**kwargs))


def eight_api_pass():
    samples = ["CaseA", "CaseB"]
    modes = ["precision", "compatibility"]
    debugs = ["debug", "nodebug"]
    rows = [
        obs(sample=sample, mode=mode, debug=debug, status="pass")
        for sample in samples
        for mode in modes
        for debug in debugs
    ]
    manifest = {
        "samples": samples,
        "modes": modes,
        "debugs": debugs,
        "interface": "api",
    }
    return rows, manifest


MILESTONE = MILESTONE_SHA
PR_BASE = PR_BASE_SHA
