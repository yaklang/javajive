"""Artifact-root confinement: reject .., symlink-out, and oversize writes."""

from __future__ import annotations

import os
import stat
from pathlib import Path

from .constants import REASON_PATH_ESCAPE


class ArtifactEscape(ValueError):
    def __init__(self, message: str, path: str = "") -> None:
        super().__init__(message)
        self.reason = REASON_PATH_ESCAPE
        self.path = path


def _is_rel_to(path: Path, root: Path) -> bool:
    try:
        path.relative_to(root)
        return True
    except ValueError:
        return False


def normalize_root(root: Path) -> Path:
    root = Path(root)
    if not root.exists():
        root.mkdir(parents=True, exist_ok=True)
    if root.is_symlink():
        raise ArtifactEscape("artifact root must not be a symlink", str(root))
    resolved = root.resolve()
    if not resolved.is_dir():
        raise ArtifactEscape("artifact root is not a directory", str(root))
    os.chmod(resolved, 0o1777)
    return resolved


def reject_relative_escape(rel: str) -> None:
    if rel is None:
        raise ArtifactEscape("missing artifact path")
    text = str(rel).replace("\\", "/")
    if text.startswith("/") or text.startswith("~") or (len(text) >= 2 and text[1] == ":"):
        raise ArtifactEscape("absolute artifact path rejected", text)
    parts = [p for p in text.split("/") if p not in ("", ".")]
    if any(p == ".." for p in parts):
        raise ArtifactEscape("parent-segment artifact path rejected", text)
    if not parts:
        raise ArtifactEscape("empty artifact path", text)


def confined_path(root: Path, rel: str, *, follow_final: bool = False) -> Path:
    reject_relative_escape(rel)
    root = Path(root).resolve()
    current = root
    parts = [p for p in Path(rel).parts if p not in ("", ".")]
    for i, part in enumerate(parts):
        current = current / part
        try:
            st = os.lstat(current)
        except FileNotFoundError:
            if i != len(parts) - 1:
                # intermediate missing: still require the would-be path under root
                if not _is_rel_to(current.resolve() if current.parent.exists() else current, root):
                    raise ArtifactEscape("artifact path escaped root", str(current)) from None
            break
        if stat.S_ISLNK(st.st_mode):
            target = Path(os.path.realpath(current))
            if not _is_rel_to(target, root):
                raise ArtifactEscape("symlink escaped artifact root", str(current))
            if not follow_final and i == len(parts) - 1:
                raise ArtifactEscape("refusing to write through symlink", str(current))
            current = target
    resolved_parent = current.parent.resolve()
    if not _is_rel_to(resolved_parent, root) and resolved_parent != root:
        raise ArtifactEscape("artifact parent escaped root", str(current))
    candidate = resolved_parent / current.name
    if not _is_rel_to(candidate if candidate.exists() else resolved_parent, root) and resolved_parent != root:
        raise ArtifactEscape("artifact path escaped root", str(candidate))
    return candidate


def write_bytes(root: Path, rel: str, data: bytes, *, cap: int) -> Path:
    dest = confined_path(root, rel)
    dest.parent.mkdir(parents=True, exist_ok=True)
    if len(data) > cap:
        raise ArtifactEscape(f"payload exceeds byte cap {cap}", rel)
    used = directory_size(root)
    if used + len(data) > cap:
        raise ArtifactEscape(f"artifact root exceeds byte cap {cap}", rel)
    flags = os.O_WRONLY | os.O_CREAT | os.O_TRUNC
    if hasattr(os, "O_NOFOLLOW"):
        flags |= os.O_NOFOLLOW
    fd = os.open(dest, flags, 0o644)
    try:
        os.write(fd, data)
    finally:
        os.close(fd)
    return dest


def directory_size(root: Path) -> int:
    total = 0
    root = Path(root)
    if not root.exists():
        return 0
    for dirpath, dirnames, filenames in os.walk(root, followlinks=False):
        # do not walk symlink directories
        dirnames[:] = [d for d in dirnames if not os.path.islink(os.path.join(dirpath, d))]
        for name in filenames:
            p = os.path.join(dirpath, name)
            if os.path.islink(p):
                continue
            try:
                total += os.path.getsize(p)
            except OSError:
                continue
    return total


def enforce_cap(root: Path, cap: int) -> tuple[int, bool]:
    """Delete overflow files (largest first) until under cap. Returns (size, capped)."""
    root = Path(root)
    files: list[tuple[int, Path]] = []
    if root.exists():
        for dirpath, dirnames, filenames in os.walk(root, followlinks=False):
            dirnames[:] = [d for d in dirnames if not os.path.islink(os.path.join(dirpath, d))]
            for name in filenames:
                p = Path(dirpath) / name
                if p.is_symlink():
                    try:
                        p.unlink()
                    except OSError:
                        pass
                    continue
                try:
                    files.append((p.stat().st_size, p))
                except OSError:
                    continue
    total = sum(sz for sz, _ in files)
    capped = False
    if total <= cap:
        return total, False
    files.sort(reverse=True)
    for sz, p in files:
        if total <= cap:
            break
        try:
            p.unlink()
            total -= sz
            capped = True
        except OSError:
            continue
    return max(total, 0), True


def host_files_outside(root: Path, parent: Path) -> list[Path]:
    """Return files in parent that are not inside root (escape witnesses)."""
    root = root.resolve()
    parent = parent.resolve()
    found: list[Path] = []
    if not parent.exists():
        return found
    for dirpath, dirnames, filenames in os.walk(parent, followlinks=False):
        dirnames[:] = [d for d in dirnames if not os.path.islink(os.path.join(dirpath, d))]
        for name in filenames:
            p = (Path(dirpath) / name).resolve()
            if not _is_rel_to(p, root):
                found.append(p)
    return found
