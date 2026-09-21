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
            "debug_used": self.debug,
            "decompiled_source_sha256": _sha(self.decompiled_source.encode()) if self.decompiled_source else "",
        }


def _sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def extract_class_name(source: str) -> str:
    m = re.search(r"public class (\w+)", source)
    if not m:
        raise InfraError("source has no public class")
    return m.group(1)


def extract_binary_name(source: str) -> str:
    pkg = re.search(r"^\s*package\s+([\w.]+)\s*;", source, re.M)
    cls = extract_class_name(source)
    return f"{pkg.group(1)}.{cls}" if pkg else cls


def source_relpath(binary_name: str) -> str:
    """JVM binary name -> compilation-unit path.

    demo.Pack -> demo/Pack.java
    demo.Outer$Inner / demo.Outer$1 -> demo/Outer.java (outer compilation unit)
    App -> App.java
    """
    unit = (binary_name or "").split("$", 1)[0].strip()
    if not unit:
        raise InfraError("empty binary name")
    return unit.replace(".", "/") + ".java"


def class_relpath(binary_name: str) -> str:
    """JVM binary name -> class-file path, including inner/anonymous suffixes."""
    name = (binary_name or "").strip()
    if not name:
        raise InfraError("empty binary name")
    return name.replace(".", "/") + ".class"


def _write_text(path: Path, text: str) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")
    return path


def _write_bytes(path: Path, data: bytes) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)
    return path


ISOLATION_STAGES = frozenset(
    {
        "isolation_unavailable",
        "policy_deny",
        "capability_unsupported",
        "leftover_process",
        "leftover_query_failed",
        "resource_limit",
        "timeout",
    }
)


def _isolation_failure_class(ran: dict[str, Any]) -> str | None:
    stage = ran.get("stage") or ""
    status = ran.get("status") or ""
    if stage in ISOLATION_STAGES or status in {
        "unsupported",
        "infra_error",
        "timeout",
        "resource_limit",
        "isolation_unavailable",
    }:
        if status in {"unsupported", "isolation_unavailable"} or stage in {
            "isolation_unavailable",
            "capability_unsupported",
            "policy_deny",
        }:
            return "unsupported"
        return "infra_error"
    if ran.get("leftover_query_failed") or ran.get("leftover_cleanup_failed"):
        return "infra_error"
    if ran.get("verified_and_ran") and (
        ran.get("leftover_host_pids") or ran.get("leftover_containers")
    ):
        return "infra_error"
    return None


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


def decompile_class(
    class_file: Path,
    mode: str,
    work: Path,
    *,
    max_analysis_updates: int | None = None,
) -> dict[str, Any]:
    probe = ensure_probe(work)
    env = os.environ.copy()
    env["CGO_ENABLED"] = "1"
    env["GOTOOLCHAIN"] = "local"
    argv = [str(probe), "-mode", mode]
    if max_analysis_updates is not None:
        argv.extend(["-max-analysis-updates", str(int(max_analysis_updates))])
    argv.append(str(class_file))
    proc = subprocess.run(
        argv,
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
    diagnostics = result.get("diagnostics") or []
    budget_hit = any(
        "analysis_budget_exceeded" in str(d.get("message") or d) for d in diagnostics
    )
    status = result.get("status") or ("infra_error" if proc.returncode == 2 else "behavior")
    if budget_hit:
        status = "budget"
    return {
        "rc": proc.returncode,
        "stderr": proc.stderr,
        "stdout": proc.stdout,
        "source": result.get("source") or "",
        "status": status,
        "stub_methods": result.get("stub_methods") or [],
        "error": parsed.get("error") or "",
        "input_hash": result.get("input_hash") or "",
        "mode": result.get("mode") or mode,
        "diagnostics": diagnostics,
        "budget_hit": budget_hit,
        "argv": argv,
    }


def run_pipeline(
    source: str,
    identity: CompilerIdentity,
    work: Path,
    *,
    mode: str = "precision",
    debug: str = "nodebug",
    class_name: str | None = None,
    trusted: bool = False,
) -> PipelineResult:
    work.mkdir(parents=True, exist_ok=True)
    artifacts = work / "artifacts"
    artifacts.mkdir(exist_ok=True)
    name = class_name or extract_binary_name(source)
    src_path = _write_text(artifacts / source_relpath(name), source)
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
        missing_compiler = compiled.get("rc") == 127 or str(compiled.get("stderr") or "").startswith("infra_error:")
        if missing_compiler:
            stages["compile"].status = "infra_error"
            return PipelineResult(
                name, mode, debug, "infra_error", stages, source, "", "", "", "",
                None, str(src_path), classpath, str(work), "", [],
                notes="compiler missing or unusable (infra_error, not invalid_input)",
            )
        return PipelineResult(
            name, mode, debug, "invalid_input", stages, source, "", "", "", "",
            None, str(src_path), classpath, str(work), "", [],
            notes="original source did not compile",
        )

    class_file = orig_dir / class_relpath(name)
    if not class_file.is_file():
        stages["compile"].status = "compile_fail"
        return PipelineResult(
            name, mode, debug, "invalid_input", stages, source, "", "", "", "",
            None, str(src_path), classpath, str(work), "", [],
            notes="missing class file after compile",
        )
    class_bytes = class_file.read_bytes()
    return _pipeline_after_class(
        source=source,
        identity=identity,
        work=work,
        artifacts=artifacts,
        name=name,
        mode=mode,
        debug=debug,
        src_path=src_path,
        orig_dir=orig_dir,
        class_file=class_file,
        class_bytes=class_bytes,
        stages=stages,
        classpath=classpath,
        trusted=trusted,
    )


def run_pipeline_from_class_bytes(
    source: str,
    class_bytes: bytes,
    identity: CompilerIdentity,
    work: Path,
    *,
    mode: str = "precision",
    debug: str = "nodebug",
    class_name: str | None = None,
    trusted: bool = False,
) -> PipelineResult:
    """Decompile provided class bytes. Default untrusted (sandbox orig+rebuilt)."""
    work.mkdir(parents=True, exist_ok=True)
    artifacts = work / "artifacts"
    artifacts.mkdir(exist_ok=True)
    name = class_name or extract_binary_name(source)
    src_path = _write_text(artifacts / source_relpath(name), source)
    orig_dir = work / "original"
    orig_dir.mkdir(parents=True, exist_ok=True)
    class_file = _write_bytes(orig_dir / class_relpath(name), class_bytes)
    _write_bytes(artifacts / class_relpath(name), class_bytes)
    stages: dict[str, StageRecord] = {
        "compile": StageRecord(
            "compile",
            "provided_class_bytes",
            0,
            "",
            f"input_class_sha256={_sha(class_bytes)} debug={debug}",
            [],
            str(class_file),
        )
    }
    return _pipeline_after_class(
        source=source,
        identity=identity,
        work=work,
        artifacts=artifacts,
        name=name,
        mode=mode,
        debug=debug,
        src_path=src_path,
        orig_dir=orig_dir,
        class_file=class_file,
        class_bytes=class_bytes,
        stages=stages,
        classpath=[],
        trusted=trusted,
    )


def _pipeline_after_class(
    *,
    source: str,
    identity: CompilerIdentity,
    work: Path,
    artifacts: Path,
    name: str,
    mode: str,
    debug: str,
    src_path: Path,
    orig_dir: Path,
    class_file: Path,
    class_bytes: bytes,
    stages: dict[str, StageRecord],
    classpath: list[str],
    trusted: bool,
) -> PipelineResult:
    bytes_path = _write_bytes(artifacts / class_relpath(name), class_bytes)
    classpath = list(classpath) + [str(orig_dir)]

    orig_run = verify_and_run(
        orig_dir,
        name,
        java_bin=java_command(identity),
        extra_cp=[Path(p) for p in classpath if p != str(orig_dir)],
        trusted=trusted,
    )
    orig_stage = orig_run["stage"]
    iso = _isolation_failure_class(orig_run)
    stages["original_verify_run"] = StageRecord(
        "original_verify_run",
        orig_stage,
        orig_run.get("rc"),
        orig_run.get("stdout") or "",
        orig_run.get("stderr") or "",
        orig_run.get("argv") or [],
        str(class_file),
    )
    if iso:
        return PipelineResult(
            name, mode, debug, iso, stages, source, _sha(class_bytes),
            "", "", orig_run.get("stdout") or "", str(bytes_path), str(src_path),
            classpath, str(work), orig_stage, [], notes=f"original isolation {orig_stage}/{orig_run.get('status')}",
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
    rebuilt_java = _write_text(rebuilt_src_dir / source_relpath(name), decompiled)
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

    rebuilt_run = verify_and_run(
        rebuilt_classes,
        name,
        java_bin=java_command(identity),
        trusted=trusted,
    )
    rstage = rebuilt_run["stage"]
    rebuilt_iso = _isolation_failure_class(rebuilt_run)
    stages["rebuild_verify_run"] = StageRecord(
        "rebuild_verify_run",
        rstage,
        rebuilt_run.get("rc"),
        rebuilt_run.get("stdout") or "",
        rebuilt_run.get("stderr") or "",
        rebuilt_run.get("argv") or [],
        str(rebuilt_classes / class_relpath(name)),
    )
    if rebuilt_iso:
        fc = rebuilt_iso
    elif rstage == "infra_error":
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
