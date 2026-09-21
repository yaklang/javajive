"""Failure evidence pack: input, output, classpath, trace, tools, failure class."""

from __future__ import annotations

import hashlib
import json
import platform
import subprocess
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any


VALID_CLASSES = (
    "invalid_input",
    "unsupported",
    "partial",
    "budget",
    "infra_error",
    "behavior",
    "generator_error",
    "pass",
)


@dataclass
class FailureEvidence:
    case_id: str
    failure_class: str
    input_sha256: str
    output: str
    classpath: list[str]
    trace: list[str]
    tool_versions: dict[str, str]
    compiler_identity: dict[str, Any] | None = None
    mode: str | None = None
    extra: dict[str, Any] = field(default_factory=dict)

    def validate(self) -> None:
        missing = []
        if not self.case_id:
            missing.append("case_id")
        if self.failure_class not in VALID_CLASSES:
            missing.append("failure_class")
        if not self.input_sha256 or len(self.input_sha256) != 64:
            missing.append("input_sha256")
        if self.output is None:
            missing.append("output")
        if not isinstance(self.classpath, list):
            missing.append("classpath")
        if not self.trace:
            missing.append("trace")
        if not self.tool_versions:
            missing.append("tool_versions")
        extra = self.extra or {}
        if extra.get("require_artifacts"):
            if not extra.get("input_source") and not extra.get("input_source_path"):
                missing.append("input_source")
            if not extra.get("input_bytes_sha256") and not extra.get("input_bytes_path"):
                missing.append("input_bytes")
        if missing:
            raise ValueError(f"incomplete evidence {self.case_id}: {missing}")

    def to_dict(self) -> dict[str, Any]:
        self.validate()
        return asdict(self)


def tool_versions() -> dict[str, str]:
    versions = {
        "python": platform.python_version(),
        "platform": platform.platform(),
        "machine": platform.machine(),
    }
    for tool in ("java", "javac"):
        try:
            proc = subprocess.run([tool, "-version"], capture_output=True, text=True, timeout=20)
            versions[tool] = ((proc.stdout or "") + (proc.stderr or "")).strip().splitlines()[0]
        except (OSError, subprocess.SubprocessError) as exc:
            versions[tool] = f"infra_error:{exc}"
    return versions


def classify_failure(stage: str, *, legal_input: bool, infra: bool, budget: bool, mismatch: bool) -> str:
    if infra:
        return "infra_error"
    if not legal_input:
        return "invalid_input"
    if budget:
        return "budget"
    if stage in {"generate", "morph"} and mismatch:
        return "generator_error"
    if mismatch:
        return "behavior"
    return "pass"


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def write_evidence(path: Path, items: list[FailureEvidence]) -> str:
    payload = [i.to_dict() for i in items]
    text = json.dumps(payload, indent=2, sort_keys=True) + "\n"
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")
    return sha256_bytes(text.encode())


def from_pipeline(case_id: str, pipe, versions: dict[str, str]) -> FailureEvidence:
    """Build evidence from a live pipeline result, including source and class bytes paths."""
    digest = pipe.original_class_sha256 or sha256_bytes(pipe.original_source.encode())
    return FailureEvidence(
        case_id=case_id,
        failure_class=pipe.failure_class,
        input_sha256=digest,
        output=pipe.rebuilt_stdout or pipe.decompiled_source[:2000] or (pipe.stages.get("compile").stderr if pipe.stages.get("compile") else ""),
        classpath=list(pipe.classpath),
        trace=list(pipe.stages.keys()) or ["pipeline"],
        tool_versions=versions,
        compiler_identity=None,
        mode=pipe.mode,
        extra={
            "require_artifacts": True,
            "input_source": pipe.original_source,
            "input_source_path": pipe.source_path,
            "input_bytes_path": pipe.input_bytes_path,
            "input_bytes_sha256": digest,
            "decompiled_source": pipe.decompiled_source,
            "stages": {k: v.to_dict() for k, v in pipe.stages.items()},
            "oracle_kind": pipe.oracle_kind,
            "work": pipe.work,
        },
    )
