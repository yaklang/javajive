"""Mount allowlist: never home, tokens, or docker.sock."""

from __future__ import annotations

import os
from pathlib import Path

from .constants import DOCKER_SOCK_PATHS, TOKEN_BASENAMES


def host_home() -> Path:
    return Path(os.path.expanduser("~")).resolve()


def docker_sock_candidates() -> list[str]:
    found = list(DOCKER_SOCK_PATHS)
    home = host_home()
    found.append(str(home / ".docker" / "run" / "docker.sock"))
    found.append(str(home / ".orbstack" / "run" / "docker.sock"))
    return found


def is_forbidden_host_path(path: Path) -> str | None:
    try:
        resolved = path.resolve()
    except OSError:
        resolved = path
    text = str(resolved)
    home = host_home()
    if text == str(home) or text.startswith(str(home) + os.sep):
        return "home"
    name = resolved.name
    if name in TOKEN_BASENAMES or "docker.sock" in text:
        return "token_or_docker_sock"
    parts = set(resolved.parts)
    if ".ssh" in parts or ".gnupg" in parts or ".aws" in parts or ".docker" in parts:
        return "credential_dir"
    return None


def denied_host_paths() -> list[str]:
    items = [str(host_home())]
    items.extend(docker_sock_candidates())
    items.append(str(host_home() / ".ssh"))
    items.append(str(host_home() / ".git-credentials"))
    items.append(str(host_home() / ".netrc"))
    # unique, stable
    out: list[str] = []
    seen: set[str] = set()
    for item in items:
        if item not in seen:
            seen.add(item)
            out.append(item)
    return out


def assert_mounts_allowed(binds: list[tuple[str, str, str]]) -> None:
    """binds: list of (host_path, container_path, mode)."""
    for host, dest, _mode in binds:
        reason = is_forbidden_host_path(Path(host))
        if reason:
            raise ValueError(f"refusing to mount {host} -> {dest}: {reason}")
        if "docker.sock" in dest or dest.rstrip("/").endswith("docker.sock"):
            raise ValueError(f"refusing docker.sock destination {dest}")


def inventory(
    *,
    backend: str,
    binds: list[tuple[str, str, str]],
    tmpfs: list[str],
    extra: dict | None = None,
) -> dict:
    sources = [h for h, _, _ in binds]
    dests = [d for _, d, _ in binds]
    modes = {d: m for _, d, m in binds}
    writable = [d for d, m in modes.items() if "rw" in m and "ro" not in m]
    readonly = [d for d, m in modes.items() if "ro" in m]
    blob_l = " ".join(sources + dests).lower()
    return {
        "backend": backend,
        "host_bind_sources": sources,
        "container_destinations": dests,
        "writable_mounts": writable,
        "readonly_mounts": readonly,
        "tmpfs": list(tmpfs),
        "host_home_mounted": False,
        "docker_sock_mounted": any("docker.sock" in (s + d).lower() for s, d in zip(sources, dests)),
        "token_paths_mounted": any(is_forbidden_host_path(Path(s)) == "token_or_docker_sock" for s in sources),
        "denied_host_paths": denied_host_paths(),
        "home_in_sources": False,
        "docker_sock_in_sources": any("docker.sock" in s for s in sources),
        "extra": extra or {},
    }
