"""Provisional candidate-revision schema for uncommitted live replay rows.

The Observation evidence.revision field remains a real git rev-parse HEAD
(40-hex) because the existing Evidence schema requires a git SHA. The candidate
identity is a separate field: candidate_revision="PROVISIONAL_UNCOMMITTED"
plus that HEAD and worktree_dirty. A synthetic 40-hex SHA is never minted as
the candidate revision. Final accepted evidence will replace this token with
an immutable candidate SHA supplied later.
"""

from __future__ import annotations

import re
import subprocess
from pathlib import Path
from typing import Any, Mapping

from .constants import MILESTONE_SHA, PR_BASE_SHA, SCHEMA_VERSION
from .errors import ObservationError
from .toolchain import probe_revision, require_git_sha

PROVISIONAL_CANDIDATE_REVISION = "PROVISIONAL_UNCOMMITTED"
_GIT_SHA_RE = re.compile(r"^[0-9a-f]{40}$")


class ProvisionalRevisionError(ObservationError):
    def __init__(self, message: str, **kwargs: Any) -> None:
        kwargs.setdefault("code", "provisional_revision")
        super().__init__(message, **kwargs)


def is_fake_40_hex_candidate(value: Any) -> bool:
    """True when a candidate_revision looks like an invented immutable SHA."""
    text = str(value or "").strip().lower()
    return bool(_GIT_SHA_RE.fullmatch(text))


def worktree_dirty(repo: Path | str, timeout: float = 20.0) -> bool:
    try:
        proc = subprocess.run(
            ["git", "status", "--porcelain"],
            cwd=str(repo),
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise ObservationError(f"infra_error: cannot probe git status: {exc}") from exc
    if proc.returncode != 0:
        raise ObservationError(
            f"infra_error: git status failed: {(proc.stderr or proc.stdout or '').strip()}"
        )
    return bool((proc.stdout or "").strip())


def describe_provisional(repo: Path | str) -> dict[str, Any]:
    head = probe_revision(repo)
    require_git_sha("git_head", head)
    dirty = worktree_dirty(repo)
    return {
        "schema_version": SCHEMA_VERSION,
        "candidate_revision": PROVISIONAL_CANDIDATE_REVISION,
        "git_head": head,
        "git_rev_parse_HEAD": head,
        "worktree_dirty": dirty,
        "pr_base_sha": PR_BASE_SHA,
        "milestone_sha": MILESTONE_SHA,
        "immutable_candidate_sha": None,
        "head_is_pr_base": head == PR_BASE_SHA,
        "claim": (
            "head_replay of git rev-parse HEAD; candidate_revision is "
            "PROVISIONAL_UNCOMMITTED; not an immutable candidate SHA"
        ),
    }


def reject_fake_candidate_sha(value: Any) -> str:
    text = str(value or "").strip()
    if is_fake_40_hex_candidate(text):
        raise ProvisionalRevisionError(
            "candidate_revision must not be a fake 40-hex SHA; "
            f"got {text!r}, want {PROVISIONAL_CANDIDATE_REVISION}"
        )
    if text != PROVISIONAL_CANDIDATE_REVISION:
        raise ProvisionalRevisionError(
            f"candidate_revision must be {PROVISIONAL_CANDIDATE_REVISION!r}, got {text!r}"
        )
    return text


def stamp_extras(extras: Mapping[str, Any] | None, provisional: Mapping[str, Any], *, role: str) -> dict[str, Any]:
    reject_fake_candidate_sha(provisional.get("candidate_revision"))
    out = dict(extras or {})
    out["candidate_revision"] = PROVISIONAL_CANDIDATE_REVISION
    out["git_head"] = str(provisional["git_head"])
    out["git_rev_parse_HEAD"] = str(provisional["git_head"])
    out["worktree_dirty"] = bool(provisional["worktree_dirty"])
    out["immutable_candidate_sha"] = None
    out["immutable_candidate_sha_claimed"] = False
    out["anchor_role"] = role
    out["head_is_pr_base"] = bool(provisional.get("head_is_pr_base"))
    return out


def wrap_provisional_ledger(
    observations: list[Mapping[str, Any]],
    *,
    role: str,
    provisional: Mapping[str, Any],
    extra: Mapping[str, Any] | None = None,
) -> dict[str, Any]:
    reject_fake_candidate_sha(provisional.get("candidate_revision"))
    payload = {
        "schema_version": SCHEMA_VERSION,
        "ledger": "t01-live-api-replay",
        "row_kind": "provisional",
        "anchor_role": role,
        "candidate_revision": PROVISIONAL_CANDIDATE_REVISION,
        "git_head": provisional["git_head"],
        "worktree_dirty": bool(provisional["worktree_dirty"]),
        "pr_base_sha": PR_BASE_SHA,
        "milestone_sha": MILESTONE_SHA,
        "immutable_candidate_sha": None,
        "historical_cli": (
            "separate compatibility-cli excerpt only; "
            "see tools/milestone_ledger/evidence/historical_cli_seeds.json"
        ),
        "observations": list(observations),
    }
    if extra:
        payload.update(dict(extra))
    return payload
