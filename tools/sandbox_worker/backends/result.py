from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any


@dataclass
class BackendResult:
    exit_code: int | None
    stdout: bytes
    stderr: bytes
    timed_out: bool
    output_capped: bool
    killed: bool
    argv: list[str]
    mount_inventory: dict[str, Any]
    resource_limits: dict[str, Any]
    extra: dict[str, Any] = field(default_factory=dict)
    error: str | None = None
