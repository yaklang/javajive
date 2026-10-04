"""Live toolchain / revision / digest probes. Missing tools are infra errors."""

from __future__ import annotations

import hashlib
import re
import subprocess
from pathlib import Path

from .constants import NEXT_STAGE_VERIFIER_SOURCE
from .errors import ObservationError

_SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
_GIT_SHA_RE = re.compile(r"^[0-9a-f]{40}$")
_JAVAC_RE = re.compile(r"^javac \d+(\.\d+)*(\s+\S.*)?$")


def sha256_hex(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def digest_file(path: Path | str) -> str:
    return sha256_hex(Path(path).read_bytes())


def require_sha256(name: str, value: str) -> str:
    text = str(value or "").strip().lower()
    if not _SHA256_RE.fullmatch(text):
        raise ObservationError(f"missing evidence: {name} must be a 64-hex sha256, got {value!r}")
    return text


def require_git_sha(name: str, value: str) -> str:
    text = str(value or "").strip().lower()
    if not _GIT_SHA_RE.fullmatch(text):
        raise ObservationError(f"missing evidence: {name} must be a 40-hex git SHA, got {value!r}")
    return text


def normalize_compiler_version(text: str) -> str:
    blob = str(text or "")
    if "--release" in blob and "javac " not in blob.split("--release", 1)[0]:
        raise ObservationError(
            "compiler_version must be real javac -version output, not --release"
        )
    for line in blob.splitlines():
        candidate = line.strip()
        if candidate.startswith("javac "):
            if "--release" in candidate:
                raise ObservationError(
                    "compiler_version must be real javac -version output, not --release"
                )
            if _JAVAC_RE.fullmatch(candidate):
                return candidate
    stripped = blob.strip()
    if stripped.isdigit() or stripped in {"8", "11", "17", "21"}:
        raise ObservationError(
            "compiler_version must be real javac -version output, not a --release / language level"
        )
    raise ObservationError(
        f"compiler_version must be real javac -version output, got {text!r}"
    )


def next_stage_harness_digest() -> str:
    return sha256_hex(NEXT_STAGE_VERIFIER_SOURCE.encode("utf-8"))


def probe_compiler(timeout: float = 20.0) -> str:
    try:
        proc = subprocess.run(
            ["javac", "-version"],
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
            timeout=timeout,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise ObservationError(f"infra_error: cannot probe javac: {exc}") from exc
    return normalize_compiler_version((proc.stdout or "") + (proc.stderr or ""))


def probe_revision(repo: Path | str, timeout: float = 20.0) -> str:
    try:
        proc = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            cwd=str(repo),
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise ObservationError(f"infra_error: cannot probe git revision: {exc}") from exc
    return require_git_sha("revision", (proc.stdout or "").strip())


def dependency_lock_digest(repo: Path | str) -> str:
    gosum = Path(repo) / "go.sum"
    if not gosum.is_file():
        raise ObservationError("missing evidence: go.sum for dependency lock digest")
    return digest_file(gosum)


def production_audit_script_digest(repo: Path | str) -> str:
    path = Path(repo) / "scripts" / "historical_jar_audit.py"
    if not path.is_file():
        raise ObservationError("missing production scripts/historical_jar_audit.py")
    return digest_file(path)
