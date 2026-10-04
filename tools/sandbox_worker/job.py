"""Untrusted job description. Callers cannot request network or host mounts."""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path

from .policy import Limits


@dataclass
class UntrustedJob:
    argv: list[str]
    input_files: dict[str, bytes] = field(default_factory=dict)
    env: dict[str, str] = field(default_factory=dict)
    limits: Limits = field(default_factory=Limits)
    name: str = "job"
    workdir: str = "/work"
    need_java: bool = False
    need_python: bool = False
    marker: str = ""
    allow_network: bool = False  # policy rejects True

    def validate(self) -> str | None:
        if self.allow_network:
            return "untrusted jobs cannot request network"
        if not self.argv or not all(isinstance(x, str) and x for x in self.argv):
            return "argv must be a non-empty list of strings"
        if any("\x00" in x for x in self.argv):
            return "nul byte in argv"
        for rel in self.input_files:
            if rel.startswith("/") or ".." in Path(rel).parts:
                return f"illegal input name {rel}"
        for key, val in self.env.items():
            if not key.isidentifier() or len(val) > 4096:
                return f"illegal env {key}"
            if key in {"HOME", "DOCKER_HOST", "SSH_AUTH_SOCK", "GITHUB_TOKEN", "GH_TOKEN", "AWS_SECRET_ACCESS_KEY"}:
                return f"env {key} is not passable into the sandbox"
        return None
