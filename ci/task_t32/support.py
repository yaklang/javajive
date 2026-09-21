"""Load T32 evidence, generating it with the Go harness if needed.

Cold/warm numbers are collected in this process. Stale committed summary.json
is not reused unless T32_FORCE_REFRESH=0.
"""
from __future__ import annotations

import os
import subprocess
import sys
import time
import uuid
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
if str(REPO) not in sys.path:
    sys.path.insert(0, str(REPO))
from tools.evidence_paths import evidence_root, evidence_subdir  # noqa: E402

EVIDENCE = evidence_subdir("t32")
ANALYSIS = REPO / "tools" / "performance_baseline"
if str(ANALYSIS) not in sys.path:
    sys.path.insert(0, str(ANALYSIS))

import analysis  # noqa: E402

SESSION_STARTED = time.time()
SESSION_NONCE = os.environ.setdefault("T32_SESSION_NONCE", uuid.uuid4().hex)

_DOC = None


def force_refresh() -> bool:
    # Recollect unless the caller explicitly sets T32_FORCE_REFRESH=0.
    return os.environ.get("T32_FORCE_REFRESH", "1") != "0"


def ensure_evidence() -> dict:
    global _DOC
    if _DOC is not None:
        return _DOC
    summary = EVIDENCE / "summary.json"
    if force_refresh() or not summary.is_file():
        run_go_harness()
    doc = analysis.load_summary(EVIDENCE)
    assert_fresh(doc, summary)
    _DOC = doc
    return doc


def assert_fresh(doc: dict, summary: Path) -> None:
    nonce = (doc.get("collect_nonce") or "").strip()
    nonce_file = EVIDENCE / "collect-nonce.txt"
    file_nonce = nonce_file.read_text(encoding="utf-8").strip() if nonce_file.is_file() else ""
    if nonce != SESSION_NONCE and file_nonce != SESSION_NONCE:
        raise AssertionError(
            f"stale evidence: collect_nonce={nonce!r} file={file_nonce!r} session={SESSION_NONCE!r}"
        )
    mtime = summary.stat().st_mtime
    if mtime < SESSION_STARTED - 2:
        raise AssertionError(
            f"summary.json mtime {mtime} older than test session start {SESSION_STARTED}"
        )
    collected = int(doc.get("collected_at_unix") or 0)
    if collected and collected < int(SESSION_STARTED) - 5:
        raise AssertionError(f"collected_at_unix {collected} older than session {SESSION_STARTED}")


def run_go_harness() -> None:
    env = os.environ.copy()
    env["CGO_ENABLED"] = "1"
    env["GOTOOLCHAIN"] = "local"
    env["T32_LDFLAGS"] = "-linkmode=external"
    env["T32_FORCE_REFRESH"] = "1"
    env["T32_COLLECT"] = "1"
    env["T32_SESSION_NONCE"] = SESSION_NONCE
    # Python contract tests collect a reduced-but-real set (all kernel stages on
    # tiny_real, e2e N/2N/4N for scale kinds, cold e2e). Full ≥10-repeat evidence
    # is produced by Go TestTaskT32CollectEvidence with T32_REDUCED unset.
    env["T32_REDUCED"] = env.get("T32_REDUCED", "1")
    env["JAVAJIVE_EVIDENCE_DIR"] = str(evidence_root())
    argv = [
        "go",
        "test",
        "./tools/performance_baseline",
        "-count=1",
        "-timeout=25m",
        "-ldflags=-linkmode=external",
        "-run",
        "^TestTaskT32PythonCollect$",
    ]
    r = subprocess.run(argv, cwd=REPO, env=env, capture_output=True, text=True)
    log = EVIDENCE
    log.mkdir(parents=True, exist_ok=True)
    (log / "go-collect.log").write_text(r.stdout + "\n" + r.stderr, encoding="utf-8")
    (log / "python-session.txt").write_text(
        f"nonce={SESSION_NONCE}\nstarted={SESSION_STARTED}\nreduced={env['T32_REDUCED']}\n",
        encoding="utf-8",
    )
    if r.returncode != 0:
        raise RuntimeError(f"go harness failed: {r.stderr[-4000:]}\n{r.stdout[-4000:]}")
