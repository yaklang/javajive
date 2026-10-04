"""Observation records for untrusted runs. Fail closed when isolation cannot be applied."""

from __future__ import annotations

from dataclasses import asdict, dataclass, field
from typing import Any

from .constants import (
    REASON_CAPABILITY_UNSUPPORTED,
    REASON_ISOLATION_UNAVAILABLE,
    STATUS_INFRA_ERROR,
    STATUS_UNSUPPORTED,
)


@dataclass
class Observation:
    status: str
    reason: str
    backend: str | None
    policy_digest: str | None
    policy: dict[str, Any] | None
    argv: list[str]
    exit_code: int | None
    stdout: str
    stderr: str
    timed_out: bool = False
    output_capped: bool = False
    artifact_bytes: int = 0
    artifact_capped: bool = False
    mount_inventory: dict[str, Any] = field(default_factory=dict)
    resource_limits: dict[str, Any] = field(default_factory=dict)
    network_class: str | None = None
    did_execute: bool = False
    leftover_host_pids: list[int] = field(default_factory=list)
    leftover_containers: list[str] = field(default_factory=list)
    compiler: str | None = None
    extra: dict[str, Any] = field(default_factory=dict)

    def as_dict(self) -> dict[str, Any]:
        return asdict(self)

    @classmethod
    def isolation_unavailable(cls, detail: str, *, os_name: str = "") -> "Observation":
        status = STATUS_UNSUPPORTED if os_name.lower().startswith("win") else STATUS_INFRA_ERROR
        return cls(
            status=status,
            reason=REASON_ISOLATION_UNAVAILABLE,
            backend=None,
            policy_digest=None,
            policy=None,
            argv=[],
            exit_code=None,
            stdout="",
            stderr=detail,
            did_execute=False,
            extra={"detail": detail, "os": os_name, "not_supported": True},
        )

    @classmethod
    def capability_unsupported(
        cls,
        detail: str,
        *,
        backend: str | None,
        missing: list[str],
        os_name: str = "",
        argv: list[str] | None = None,
    ) -> "Observation":
        return cls(
            status=STATUS_UNSUPPORTED,
            reason=REASON_CAPABILITY_UNSUPPORTED,
            backend=backend,
            policy_digest=None,
            policy=None,
            argv=list(argv or []),
            exit_code=None,
            stdout="",
            stderr=detail,
            did_execute=False,
            extra={
                "detail": detail,
                "os": os_name,
                "missing_capabilities": missing,
                "not_supported": True,
                "memory_enforced": False,
                "pids_enforced": False,
            },
        )
