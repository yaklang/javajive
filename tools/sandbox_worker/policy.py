"""Canonical isolation policy and digest. A digest is recorded on every observation."""

from __future__ import annotations

import hashlib
import json
from dataclasses import asdict, dataclass, field
from typing import Any


def canonical_json(obj: Any) -> str:
    return json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=True)


def sha256_text(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


@dataclass(frozen=True)
class Limits:
    memory_bytes: int = 256 * 1024 * 1024
    pids: int = 64
    cpu_seconds: int = 30
    cpus: str = "0.5"
    output_bytes: int = 256 * 1024
    artifact_bytes: int = 256 * 1024
    fsize_bytes: int = 256 * 1024
    timeout_seconds: float = 30.0
    nofile: int = 256

    def as_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class IsolationPolicy:
    version: str
    backend: str
    network: str
    user: str
    cap_drop: list[str]
    read_only_root: bool
    no_new_privileges: bool
    home_visible: bool
    docker_sock_visible: bool
    tokens_visible: bool
    javac_proc: str
    kill_process_group_on_timeout: bool
    limits: dict[str, Any]
    extra: dict[str, Any] = field(default_factory=dict)

    def digest_material(self) -> dict[str, Any]:
        return {
            "backend": self.backend,
            "cap_drop": list(self.cap_drop),
            "docker_sock_visible": self.docker_sock_visible,
            "home_visible": self.home_visible,
            "javac_proc": self.javac_proc,
            "kill_process_group_on_timeout": self.kill_process_group_on_timeout,
            "limits": self.limits,
            "network": self.network,
            "no_new_privileges": self.no_new_privileges,
            "read_only_root": self.read_only_root,
            "tokens_visible": self.tokens_visible,
            "user": self.user,
            "version": self.version,
            "extra": self.extra,
        }

    def digest(self) -> str:
        return sha256_text(canonical_json(self.digest_material()))

    def as_dict(self) -> dict[str, Any]:
        material = self.digest_material()
        material["digest"] = self.digest()
        return material


def make_policy(*, backend: str, limits: Limits, extra: dict[str, Any] | None = None) -> IsolationPolicy:
    from .constants import POLICY_VERSION, SANDBOX_USER

    return IsolationPolicy(
        version=POLICY_VERSION,
        backend=backend,
        network="none",
        user=SANDBOX_USER,
        cap_drop=["ALL"],
        read_only_root=True,
        no_new_privileges=True,
        home_visible=False,
        docker_sock_visible=False,
        tokens_visible=False,
        javac_proc="none",
        kill_process_group_on_timeout=True,
        limits=limits.as_dict(),
        extra=extra or {},
    )
