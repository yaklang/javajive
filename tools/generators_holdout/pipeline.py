"""Real JavaJive semantic oracle: compile / verify / decompile / rebuild / run.

Never treats a source-feature regex as a reproduced defect. Every retained
reduction trial re-runs this pipeline. Infra (missing javac/go build) is
infra_error and cannot satisfy behavioral acceptance.
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import shutil
import subprocess
import tempfile
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any

from .identity import (
    CompilerIdentity,
    InfraError,
    classify_java_process,
    compile_sources,
    java_command,
    verify_and_run,
)

REPO = Path(__file__).resolve().parents[2]
PROBE_PKG = Path(__file__).resolve().parent / "probe"
_PROBE_BIN: Path | None = None

MODES = ("precision", "compatibility")


@dataclass
class StageRecord:
    name: str
    status: str
    rc: int | None = None
    stdout: str = ""
    stderr: str = ""
    argv: list[str] = field(default_factory=list)
    artifact: str | None = None

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class PipelineResult:
    class_name: str
    mode: str
    debug: str
    failure_class: str
    stages: dict[str, StageRecord]
    original_source: str
    original_class_sha256: str
    decompiled_source: str
    rebuilt_stdout: str
    original_stdout: str
    input_bytes_path: str | None
    source_path: str | None
    classpath: list[str]
    work: str
    decompile_status: str
    stub_methods: list[str]
    notes: str = ""
    oracle_kind: str = "javajive_live"  # never "source_feature_regex"

    def to_dict(self) -> dict[str, Any]:
        return {
            "class_name": self.class_name,
            "mode": self.mode,
            "debug": self.debug,
            "failure_class": self.failure_class,
            "stages": {k: v.to_dict() for k, v in self.stages.items()},
            "original_source": self.original_source,
            "original_class_sha256": self.original_class_sha256,
            "decompiled_source": self.decompiled_source,
            "rebuilt_stdout": self.rebuilt_stdout,
            "original_stdout": self.original_stdout,
            "input_bytes_path": self.input_bytes_path,
            "source_path": self.source_path,
            "classpath": list(self.classpath),
            "work": self.work,
            "decompile_status": self.decompile_status,
            "stub_methods": list(self.stub_methods),
            "notes": self.notes,
            "oracle_kind": self.oracle_kind,
        }


def _sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def extract_class_name(source: str) -> str:
    m = re.search(r"public class (\w+)", source)
    if not m:
        raise InfraError("source has no public class")
    return m.group(1)


def ensure_probe(work: Path) -> Path:
    global _PROBE_BIN
    dest = work / "t28-oracle-probe"
    if _PROBE_BIN is not None and _PROBE_BIN.is_file():
        return _PROBE_BIN
    env = os.environ.copy()
    env["CGO_ENABLED"] = "1"
    env["GOTOOLCHAIN"] = "local"
    env["GOFLAGS"] = "-ldflags=-linkmode=external"
    proc = subprocess.run(
        ["go", "build", "-ldflags=-linkmode=external", "-o", str(dest), str(PROBE_PKG)],
        cwd=REPO,
        env=env,
        capture_output=True,
        text=True,
        timeout=180,
    )
    if proc.returncode != 0 or not dest.is_file():
        raise InfraError(f"go build probe failed: {proc.stderr[-2000:]}")
    _PROBE_BIN = dest
    return dest


def decompile_class(class_file: Path, mode: str, work: Path) -> dict[str, Any]:
    probe = ensure_probe(work)
    env = os.environ.copy()
    env["CGO_ENABLED"] = "1"
    env["GOTOOLCHAIN"] = "local"
    proc = subprocess.run(
        [str(probe), "-mode", mode, str(class_file)],
        capture_output=True,
        text=True,
        timeout=30,
        env=env,
    )
    parsed: dict[str, Any] = {}
    if proc.stdout.strip():
        try:
            parsed = json.loads(proc.stdout)
        except json.JSONDecodeError:
            parsed = {"error": "probe_json_corrupt", "raw": proc.stdout[:2000]}
    result = parsed.get("result") or {}
    return {
        "rc": proc.returncode,
        "stderr": proc.stderr,
        "stdout": proc.stdout,
        "source": result.get("source") or "",
        "status": result.get("status") or ("infra_error" if proc.returncode == 2 else "behavior"),
        "stub_methods": result.get("stub_methods") or [],
        "error": parsed.get("error") or "",
        "input_hash": result.get("input_hash") or "",
        "mode": result.get("mode") or mode,
    }


def run_pipeline(
    source: str,
    identity: CompilerIdentity,
    work: Path,
    *,
    mode: str = "precision",
    debug: str = "nodebug",
    class_name: str | None = None,
) -> PipelineResult:
    work.mkdir(parents=True, exist_ok=True)
    artifacts = work / "artifacts"
    artifacts.mkdir(exist_ok=True)
    name = class_name or extract_class_name(source)
    src_path = artifacts / f"{name}.java"
    src_path.write_text(source, encoding="utf-8")
    stages: dict[str, StageRecord] = {}
    classpath: list[str] = []

    orig_dir = work / "original"
    compiled = compile_sources(identity, [src_path], orig_dir, debug=debug)
    stages["compile"] = StageRecord(
        "compile",
        "ok" if compiled["rc"] == 0 else "compile_fail",
        compiled["rc"],
        compiled.get("stdout") or "",
        compiled.get("stderr") or "",
        compiled.get("argv") or [],
        str(orig_dir),
    )
    if compiled["rc"] != 0:
        return PipelineResult(
            name, mode, debug, "invalid_input", stages, source, "", "", "", "",
            None, str(src_path), classpath, str(work), "", [],
            notes="original source did not compile",
        )

    class_file = orig_dir / f"{name}.class"
    if not class_file.is_file():
        stages["compile"].status = "compile_fail"
        return PipelineResult(
            name, mode, debug, "invalid_input", stages, source, "", "", "", "",
            None, str(src_path), classpath, str(work), "", [],
            notes="missing class file after compile",
        )
    class_bytes = class_file.read_bytes()
    bytes_path = artifacts / f"{name}.class"
    bytes_path.write_bytes(class_bytes)
    classpath.append(str(orig_dir))

    orig_run = verify_and_run(orig_dir, name, java_bin=java_command(identity))
    orig_stage = orig_run["stage"]
    stages["original_verify_run"] = StageRecord(
        "original_verify_run",
        orig_stage,
        orig_run.get("rc"),
        orig_run.get("stdout") or "",
        orig_run.get("stderr") or "",
        orig_run.get("argv") or [],
        str(class_file),
    )
    if orig_stage == "infra_error":
        return PipelineResult(
            name, mode, debug, "infra_error", stages, source, _sha(class_bytes),
            "", "", orig_run.get("stdout") or "", str(bytes_path), str(src_path),
            classpath, str(work), "", [], notes="original run infra",
        )
    if orig_stage != "run_ok":
        # Original fixture is not a legal runnable class — not a decompiler defect.
        mapped = {
            "verify_fail": "invalid_input",
            "linkage_error": "invalid_input",
            "run_fail": "invalid_input",
        }.get(orig_stage, "invalid_input")
        return PipelineResult(
            name, mode, debug, mapped, stages, source, _sha(class_bytes),
            "", "", orig_run.get("stdout") or "", str(bytes_path), str(src_path),
            classpath, str(work), "", [], notes=f"original class stage={orig_stage}",
        )

    try:
        dec = decompile_class(class_file, mode, work)
    except InfraError as exc:
        stages["decompile"] = StageRecord("decompile", "infra_error", 2, "", str(exc), [], None)
        return PipelineResult(
            name, mode, debug, "infra_error", stages, source, _sha(class_bytes),
            "", "", orig_run.get("stdout") or "", str(bytes_path), str(src_path),
            classpath, str(work), "", [], notes=str(exc),
        )
    stages["decompile"] = StageRecord(
        "decompile",
        dec["status"] if dec["rc"] in (0, 1) else "infra_error",
        dec["rc"],
        dec.get("source") or "",
        (dec.get("stderr") or "") + (dec.get("error") or ""),
        [],
        None,
    )
    decompiled = dec.get("source") or ""
    dec_path = artifacts / f"{name}.decompiled.{mode}.java"
    dec_path.write_text(decompiled, encoding="utf-8")
    stages["decompile"].artifact = str(dec_path)

    if dec["rc"] not in (0, 1) and not decompiled.strip():
        return PipelineResult(
            name, mode, debug, "infra_error", stages, source, _sha(class_bytes),
            decompiled, "", orig_run.get("stdout") or "", str(bytes_path), str(src_path),
            classpath, str(work), dec.get("status") or "", dec.get("stub_methods") or [],
            notes="probe failed",
        )
    if not decompiled.strip():
        return PipelineResult(
            name, mode, debug, "unsupported", stages, source, _sha(class_bytes),
            "", "", orig_run.get("stdout") or "", str(bytes_path), str(src_path),
            classpath, str(work), dec.get("status") or "unsupported",
            dec.get("stub_methods") or [], notes="empty decompile",
        )

    rebuilt_src_dir = work / f"rebuilt-{mode}"
    rebuilt_src_dir.mkdir(exist_ok=True)
    rebuilt_java = rebuilt_src_dir / f"{name}.java"
    rebuilt_java.write_text(decompiled, encoding="utf-8")
    rebuilt_classes = work / f"rebuilt-{mode}-classes"
    rebuilt = compile_sources(identity, [rebuilt_java], rebuilt_classes, debug=debug)
    stages["rebuild_compile"] = StageRecord(
        "rebuild_compile",
        "ok" if rebuilt["rc"] == 0 else "rebuild_compile_fail",
        rebuilt["rc"],
        rebuilt.get("stdout") or "",
        rebuilt.get("stderr") or "",
        rebuilt.get("argv") or [],
        str(rebuilt_classes),
    )
    if rebuilt["rc"] != 0:
        return PipelineResult(
            name, mode, debug, "behavior", stages, source, _sha(class_bytes),
            decompiled, "", orig_run.get("stdout") or "", str(bytes_path), str(src_path),
            classpath + [str(rebuilt_classes)], str(work), dec.get("status") or "",
            dec.get("stub_methods") or [], notes="decompiled source did not compile",
        )

    rebuilt_run = verify_and_run(rebuilt_classes, name, java_bin=java_command(identity))
    rstage = rebuilt_run["stage"]
    stages["rebuild_verify_run"] = StageRecord(
        "rebuild_verify_run",
        rstage,
        rebuilt_run.get("rc"),
        rebuilt_run.get("stdout") or "",
        rebuilt_run.get("stderr") or "",
        rebuilt_run.get("argv") or [],
        str(rebuilt_classes / f"{name}.class"),
    )
    if rstage == "infra_error":
        fc = "infra_error"
    elif rstage != "run_ok":
        fc = "behavior"
    elif rebuilt_run.get("stdout") != orig_run.get("stdout"):
        fc = "behavior"
        stages["stdout_compare"] = StageRecord(
            "stdout_compare", "mismatch", 1,
            rebuilt_run.get("stdout") or "", orig_run.get("stdout") or "", [], None,
        )
    else:
        fc = "pass"
        stages["stdout_compare"] = StageRecord(
            "stdout_compare", "equal", 0,
            rebuilt_run.get("stdout") or "", "", [], None,
        )

    if dec.get("stub_methods") and fc == "pass":
        # Stubs with matching stdout is still partial, not a silent pass.
        fc = "partial"
        notes = "stub methods present; not counted as full pass"
    else:
        notes = ""

    return PipelineResult(
        name, mode, debug, fc, stages, source, _sha(class_bytes),
        decompiled, rebuilt_run.get("stdout") or "", orig_run.get("stdout") or "",
        str(bytes_path), str(src_path), classpath + [str(rebuilt_classes)],
        str(work), dec.get("status") or "", dec.get("stub_methods") or [], notes=notes,
    )


class FaultInjectedOracle:
    """Separately labeled harness control. Not a JavaJive observation."""

    MARKER = "T28_INJECT_FAIL"

    def classify(self, source: str) -> str:
        if self.MARKER in source:
            return "harness_control_injected_mismatch"
        return "pass"
