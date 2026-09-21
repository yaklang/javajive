"""Materialize immutable git commits into unique temp checkouts.

A clean clone/CI job reconstructs 31de113 and 81f8ef5 from objects already in
the repository. Missing objects are infra_error (never skipped). Caller-supplied
paths are verified and never written. No shared /private/tmp/javajive-* default.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
from pathlib import Path

from .errors import ObservationError
from .provisional import worktree_dirty
from .toolchain import probe_revision


class AnchorCheckoutError(ObservationError):
    def __init__(self, message: str, **kwargs) -> None:
        kwargs.setdefault("code", "infra_error")
        super().__init__(message, **kwargs)


ENV_FROZEN = "JAVAJIVE_FROZEN_31DE113"
ENV_PR_BASE = "JAVAJIVE_PR_BASE_81F8EF5"
ENV_FETCH = "JAVAJIVE_FETCH_ANCHORS"

_PROTECTED: set[Path] = set()


def unique_scratch(prefix: str) -> Path:
    return Path(tempfile.mkdtemp(prefix=prefix))


def protect_tree(tree: Path) -> Path:
    resolved = Path(tree).resolve()
    _PROTECTED.add(resolved)
    return resolved


def protected_trees() -> list[Path]:
    return sorted(_PROTECTED)


def refuse_write_inside_protected(path: Path) -> None:
    resolved = Path(path).resolve()
    for tree in list(_PROTECTED):
        try:
            resolved.relative_to(tree)
        except ValueError:
            continue
        raise AnchorCheckoutError(
            f"infra_error: refusing to write inside protected tree {tree}: {path}"
        )


def git_common_dir(repo: Path) -> Path:
    proc = subprocess.run(
        ["git", "-C", str(repo), "rev-parse", "--git-common-dir"],
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        raise AnchorCheckoutError(
            f"infra_error: not a git repository: {repo}: {(proc.stderr or proc.stdout or '').strip()}"
        )
    raw = Path(proc.stdout.strip())
    if not raw.is_absolute():
        raw = (Path(repo) / raw).resolve()
    return raw


def commit_exists(repo: Path, sha: str) -> bool:
    proc = subprocess.run(
        ["git", "-C", str(repo), "cat-file", "-t", sha],
        capture_output=True,
        text=True,
        check=False,
    )
    return proc.returncode == 0 and proc.stdout.strip() == "commit"


def require_commit(repo: Path, sha: str) -> None:
    if commit_exists(repo, sha):
        return
    if os.environ.get(ENV_FETCH) == "1":
        fetch = subprocess.run(
            ["git", "-C", str(repo), "fetch", "--no-tags", "origin", sha],
            capture_output=True,
            text=True,
            check=False,
        )
        if fetch.returncode == 0 and commit_exists(repo, sha):
            return
        detail = (fetch.stderr or fetch.stdout or "").strip()
        raise AnchorCheckoutError(
            f"infra_error: commit {sha} is not in this clone and fetch failed. "
            f"Unavailable evidence is not skipped. {detail}"
        )
    raise AnchorCheckoutError(
        f"infra_error: commit {sha} is not in this clone. "
        f"Fetch the object (`git fetch origin {sha}`) before replay; "
        "unavailable evidence is not skipped."
    )


def verify_explicit_tree(path: Path, sha: str) -> Path:
    if not path.is_dir():
        raise AnchorCheckoutError(
            f"infra_error: explicit tree missing: {path} "
            "(unavailable evidence is not skipped)"
        )
    head = probe_revision(path)
    if head != sha:
        raise AnchorCheckoutError(
            f"infra_error: explicit tree HEAD {head} != required {sha} ({path})"
        )
    if worktree_dirty(path):
        raise AnchorCheckoutError(
            f"infra_error: explicit tree is dirty; refusing to use or mutate {path}"
        )
    javajive = path / "javajive.go"
    if not javajive.is_file():
        raise AnchorCheckoutError(f"infra_error: javajive.go missing in explicit tree {path}")
    text = javajive.read_text(encoding="utf-8")
    if "func DecompileWithOptions(" not in text:
        raise AnchorCheckoutError(
            f"infra_error: explicit tree lacks DecompileWithOptions: {path}"
        )
    return protect_tree(path)


def materialize_commit(
    sha: str,
    *,
    source_repo: Path,
    dest: Path | None = None,
) -> Path:
    """Checkout `sha` into a unique directory using repo objects. Never mutates other trees."""
    require_commit(source_repo, sha)
    dest = Path(dest) if dest is not None else unique_scratch(f"javajive-{sha[:12]}-")
    refuse_write_inside_protected(dest)
    if dest.exists() and any(dest.iterdir()):
        raise AnchorCheckoutError(f"infra_error: materialize dest is not empty: {dest}")
    if dest.exists():
        dest.rmdir()
    common = git_common_dir(source_repo)
    clone = subprocess.run(
        ["git", "clone", "--shared", "--no-checkout", "--quiet", str(common), str(dest)],
        capture_output=True,
        text=True,
        check=False,
    )
    if clone.returncode != 0:
        shutil.rmtree(dest, ignore_errors=True)
        clone = subprocess.run(
            ["git", "clone", "--no-checkout", "--quiet", str(common), str(dest)],
            capture_output=True,
            text=True,
            check=False,
        )
    if clone.returncode != 0:
        raise AnchorCheckoutError(
            "infra_error: git clone for anchor "
            f"{sha} failed: {(clone.stderr or clone.stdout or '').strip()}"
        )
    checkout = subprocess.run(
        ["git", "-C", str(dest), "checkout", "--detach", sha],
        capture_output=True,
        text=True,
        check=False,
    )
    if checkout.returncode != 0:
        raise AnchorCheckoutError(
            "infra_error: git checkout --detach "
            f"{sha} failed: {(checkout.stderr or checkout.stdout or '').strip()}"
        )
    verified = verify_explicit_tree(dest, sha)
    return verified


def resolve_anchor_tree(
    sha: str,
    *,
    source_repo: Path,
    explicit: Path | str | None = None,
    env_key: str | None = None,
) -> Path:
    """Honor an explicit path (env or argument) or materialize from git objects."""
    chosen: Path | None = None
    if explicit is not None:
        chosen = Path(explicit)
    elif env_key:
        raw = os.environ.get(env_key)
        if raw:
            chosen = Path(raw)
    if chosen is not None:
        return verify_explicit_tree(chosen, sha)
    return materialize_commit(sha, source_repo=source_repo)
