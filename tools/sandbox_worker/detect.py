"""Detect a real isolation backend. Subprocess+timeout is never selected."""

from __future__ import annotations

import os
import platform
import shutil
import subprocess
from dataclasses import dataclass, field
from typing import Any


@dataclass
class BackendProbe:
    name: str
    available: bool
    detail: str
    binary: str | None = None
    extra: dict[str, Any] = field(default_factory=dict)


@dataclass
class Detection:
    selected: str | None
    probes: list[BackendProbe]
    os_name: str
    hostname: str

    def as_dict(self) -> dict[str, Any]:
        return {
            "selected": self.selected,
            "os_name": self.os_name,
            "hostname": self.hostname,
            "probes": [
                {
                    "name": p.name,
                    "available": p.available,
                    "detail": p.detail,
                    "binary": p.binary,
                    "extra": p.extra,
                }
                for p in self.probes
            ],
        }


def _run(argv: list[str], timeout: float = 20.0) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        argv,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        timeout=timeout,
        check=False,
    )


def _probe_container_engine(binary: str) -> BackendProbe:
    path = shutil.which(binary)
    if not path:
        return BackendProbe(binary, False, f"{binary} not on PATH")
    try:
        info = _run([path, "info"], timeout=25)
    except (OSError, subprocess.TimeoutExpired) as exc:
        return BackendProbe(binary, False, f"{binary} info failed: {exc}", path)
    if info.returncode != 0:
        err = (info.stderr or info.stdout or "").strip().splitlines()
        tail = err[-1] if err else f"exit {info.returncode}"
        return BackendProbe(binary, False, f"{binary} engine not usable: {tail}", path)
    version = ""
    try:
        ver = _run([path, "version", "--format", "{{.Server.Version}}"], timeout=15)
        version = (ver.stdout or "").strip()
    except (OSError, subprocess.TimeoutExpired):
        version = ""
    return BackendProbe(
        binary,
        True,
        f"{binary} engine up" + (f" version={version}" if version else ""),
        path,
        extra={"server_version": version},
    )


def _probe_bwrap() -> BackendProbe:
    path = shutil.which("bwrap")
    if not path:
        return BackendProbe("bwrap", False, "bwrap not on PATH")
    try:
        probe = _run(
            [
                path,
                "--die-with-parent",
                "--unshare-net",
                "--ro-bind",
                "/usr",
                "/usr",
                "--ro-bind",
                "/bin",
                "/bin",
                "--ro-bind-try",
                "/lib",
                "/lib",
                "--ro-bind-try",
                "/lib64",
                "/lib64",
                "--dev",
                "/dev",
                "--tmpfs",
                "/tmp",
                "/bin/true",
            ],
            timeout=15,
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        return BackendProbe("bwrap", False, f"bwrap probe failed: {exc}", path)
    if probe.returncode != 0:
        return BackendProbe("bwrap", False, (probe.stderr or probe.stdout or "bwrap probe failed")[:400], path)
    return BackendProbe("bwrap", True, "bwrap unshare-net probe ok", path)


def _probe_unshare() -> BackendProbe:
    if platform.system() != "Linux":
        return BackendProbe("unshare", False, "unshare backend is Linux-only")
    path = shutil.which("unshare")
    if not path:
        return BackendProbe("unshare", False, "unshare not on PATH")
    try:
        probe = _run(
            [path, "--user", "--map-root-user", "--net", "--mount", "--pid", "--fork", "true"],
            timeout=10,
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        return BackendProbe("unshare", False, f"unshare probe failed: {exc}", path)
    if probe.returncode != 0:
        return BackendProbe("unshare", False, (probe.stderr or probe.stdout or "unshare probe failed")[:400], path)
    return BackendProbe("unshare", True, "unshare user/net/mount/pid probe ok", path)


def _probe_seatbelt() -> BackendProbe:
    if platform.system() != "Darwin":
        return BackendProbe("sandbox-exec", False, "sandbox-exec backend is macOS-only")
    path = shutil.which("sandbox-exec") or "/usr/bin/sandbox-exec"
    if not os.path.isfile(path):
        return BackendProbe("sandbox-exec", False, "sandbox-exec binary missing")
    try:
        probe = _run([path, "-n", "no-network", "/usr/bin/true"], timeout=10)
    except (OSError, subprocess.TimeoutExpired) as exc:
        return BackendProbe("sandbox-exec", False, f"sandbox-exec probe failed: {exc}", path)
    if probe.returncode != 0:
        # Profile name no-network may be absent; binary still counts if -f works later.
        return BackendProbe(
            "sandbox-exec",
            True,
            "sandbox-exec present (named profile probe non-zero; custom profiles used)",
            path,
            extra={"named_profile_exit": probe.returncode},
        )
    return BackendProbe("sandbox-exec", True, "sandbox-exec no-network profile ok", path)


def detect() -> Detection:
    os_name = platform.system()
    probes = [
        _probe_container_engine("docker"),
        _probe_container_engine("podman"),
        _probe_bwrap(),
        _probe_unshare(),
        _probe_seatbelt(),
    ]
    selected = None
    by_name = {p.name: p for p in probes}
    if by_name["docker"].available:
        selected = "docker"
    elif by_name["podman"].available:
        selected = "podman"
    elif by_name["bwrap"].available:
        selected = "bwrap"
    elif by_name["unshare"].available:
        selected = "unshare"
    elif by_name["sandbox-exec"].available:
        selected = "sandbox-exec"
    return Detection(selected=selected, probes=probes, os_name=os_name, hostname=platform.node())


def detection_for_forced(name: str | None) -> Detection:
    det = detect()
    if not name:
        return det
    allowed = {p.name for p in det.probes if p.available}
    if name not in allowed:
        det.selected = None
        return det
    det.selected = name
    return det
