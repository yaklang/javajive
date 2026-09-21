"""Live Precision/Compatibility API round-trip replay.

Compiles reviewed fixtures, decompiles via javajive.DecompileWithOptions
(through tools/milestone_ledger/cmd_probe), rebuilds, verifies, and runs.
Pass requires compile+verify success and original/rebuilt stdout+stderr match.
Missing javac/go is infra_error, never skip and never pass.
"""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import subprocess
import sys
import time
from pathlib import Path
from typing import Any, Mapping, Sequence

from .compare import compare_anchors
from .constants import (
    API_MODES,
    DEBUGS,
    MILESTONE_SHA,
    NEXT_STAGE_VERIFIER_SOURCE,
    PR_BASE_SHA,
    SCHEMA_VERSION,
    SEED_SOURCE_SHA256,
)
from .errors import ObservationError
from .historical import load_historical_cli_report
from .observation import Observation, parse_observation
from .provisional import (
    PROVISIONAL_CANDIDATE_REVISION,
    describe_provisional,
    stamp_extras,
    wrap_provisional_ledger,
)
from .toolchain import (
    dependency_lock_digest,
    digest_file,
    probe_compiler,
    sha256_hex,
)

PACKAGE_DIR = Path(__file__).resolve().parent
REPO_ROOT = PACKAGE_DIR.parents[1]
FIXTURES_DIR = PACKAGE_DIR / "fixtures"
EVIDENCE_DIR = (
    Path(os.environ["JAVAJIVE_EVIDENCE_DIR"]) / "milestone_ledger" / "live_replay"
    if os.environ.get("JAVAJIVE_EVIDENCE_DIR")
    else PACKAGE_DIR / "evidence" / "live_replay"
)
PROBE_PKG = PACKAGE_DIR / "cmd_probe"
JAVAJIVE_PKG = "./cmd/javajive"
PROBE_IMPORT = "./tools/milestone_ledger/cmd_probe"

# seed_scope / historical CLI --release per reviewed fixture.
FIXTURE_RELEASE = {
    "Baseline": 8,
    "UnicodePair": 8,
    "ConcatProbe": 17,
    "ParamAnnotation": 8,
}
DEFAULT_SAMPLES = ("Baseline", "UnicodePair", "ConcatProbe", "ParamAnnotation")
DEFAULT_MODES = ("precision", "compatibility")
DEFAULT_DEBUGS = ("debug", "nodebug")
MAX_ANALYSIS_UPDATES = 1_000_000


class LiveReplayInfraError(ObservationError):
    def __init__(self, message: str, **kwargs: Any) -> None:
        kwargs.setdefault("code", "infra_error")
        super().__init__(message, **kwargs)


def _now() -> float:
    return time.perf_counter()


def _which_or_infra(name: str) -> str:
    path = shutil.which(name)
    if not path:
        raise LiveReplayInfraError(f"infra_error: {name} missing")
    return path


def go_build_env() -> dict[str, str]:
    env = os.environ.copy()
    env["CGO_ENABLED"] = "1"
    env["GOTOOLCHAIN"] = "local"
    env["GOFLAGS"] = "-ldflags=-linkmode=external"
    return env


def harness_digest() -> str:
    parts = [
        Path(__file__).read_bytes(),
        (PROBE_PKG / "main.go").read_bytes(),
        NEXT_STAGE_VERIFIER_SOURCE.encode("utf-8"),
    ]
    h = hashlib.sha256()
    for blob in parts:
        h.update(blob)
    return h.hexdigest()


def _run(
    argv: Sequence[str],
    *,
    cwd: Path | str | None = None,
    env: Mapping[str, str] | None = None,
    timeout: float = 60.0,
    text_input: str | None = None,
) -> dict[str, Any]:
    started = _now()
    try:
        proc = subprocess.run(
            list(argv),
            cwd=None if cwd is None else str(cwd),
            env=dict(env) if env is not None else None,
            input=text_input,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
            timeout=timeout,
            check=False,
        )
        return {
            "argv": list(argv),
            "rc": proc.returncode,
            "timeout": False,
            "seconds": _now() - started,
            "stdout": proc.stdout or "",
            "stderr": proc.stderr or "",
        }
    except subprocess.TimeoutExpired as exc:
        return {
            "argv": list(argv),
            "rc": None,
            "timeout": True,
            "seconds": _now() - started,
            "stdout": (exc.stdout or "") if isinstance(exc.stdout, str) else "",
            "stderr": ((exc.stderr or "") if isinstance(exc.stderr, str) else "") + f"\ntimeout after {timeout}s",
        }
    except OSError as exc:
        return {
            "argv": list(argv),
            "rc": None,
            "timeout": False,
            "seconds": _now() - started,
            "stdout": "",
            "stderr": f"infra_error: {exc}",
            "os_error": str(exc),
        }


def compile_java(
    source: Path,
    dest: Path,
    *,
    javac: str,
    release: int,
    debug: str,
    timeout: float = 60.0,
) -> dict[str, Any]:
    dest.mkdir(parents=True, exist_ok=True)
    debug_flag = "-g" if debug == "debug" else "-g:none"
    argv = [
        javac,
        "-proc:none",
        "-encoding",
        "UTF-8",
        "--release",
        str(release),
        debug_flag,
        "-d",
        str(dest),
        str(source),
    ]
    record = _run(argv, timeout=timeout)
    class_files = sorted(dest.glob("*.class"))
    record["class_files"] = [str(p) for p in class_files]
    record["ok"] = record.get("rc") == 0 and not record.get("timeout")
    return record


def java_argv(java: str, classpath: Sequence[str], main_class: str) -> list[str]:
    return [
        java,
        "-Xmx192m",
        "-XX:ActiveProcessorCount=2",
        "-Xverify:all",
        "-Dfile.encoding=UTF-8",
        "-Duser.language=en",
        "-Duser.country=US",
        "-cp",
        os.pathsep.join(str(p) for p in classpath),
        main_class,
    ]


def build_binaries(work: Path, repo: Path) -> dict[str, Any]:
    env = go_build_env()
    javajive_bin = work / "bin" / "javajive"
    probe_bin = work / "bin" / "t01-api-probe"
    javajive_bin.parent.mkdir(parents=True, exist_ok=True)
    jj = _run(
        ["go", "build", "-ldflags=-linkmode=external", "-o", str(javajive_bin), JAVAJIVE_PKG],
        cwd=repo,
        env=env,
        timeout=300.0,
    )
    if jj.get("rc") != 0 or not javajive_bin.is_file():
        raise LiveReplayInfraError(
            "infra_error: go build javajive failed: " + (jj.get("stderr") or jj.get("stdout") or "")
        )
    probe = _run(
        ["go", "build", "-ldflags=-linkmode=external", "-o", str(probe_bin), PROBE_IMPORT],
        cwd=repo,
        env=env,
        timeout=300.0,
    )
    if probe.get("rc") != 0 or not probe_bin.is_file():
        raise LiveReplayInfraError(
            "infra_error: go build cmd_probe failed: " + (probe.get("stderr") or probe.get("stdout") or "")
        )
    return {
        "javajive": str(javajive_bin),
        "javajive_sha256": digest_file(javajive_bin),
        "probe": str(probe_bin),
        "probe_sha256": digest_file(probe_bin),
        "javajive_build": jj,
        "probe_build": probe,
        "cgo_enabled": "1",
        "gotoolchain": "local",
        "goflags": "-ldflags=-linkmode=external",
    }


def ensure_verifier(work: Path, javac: str) -> Path:
    harness = work / "harness"
    src = harness / "NextStageVerify.java"
    src.parent.mkdir(parents=True, exist_ok=True)
    src.write_text(NEXT_STAGE_VERIFIER_SOURCE, encoding="utf-8")
    record = compile_java(src, harness, javac=javac, release=8, debug="nodebug")
    if not record.get("ok"):
        raise LiveReplayInfraError(
            "infra_error: cannot compile NextStageVerify: " + (record.get("stderr") or "")
        )
    return harness


def decompile_api(probe: Path, class_file: Path, mode: str) -> dict[str, Any]:
    argv = [
        str(probe),
        "-mode",
        mode,
        "-max-analysis-updates",
        str(MAX_ANALYSIS_UPDATES),
        str(class_file),
    ]
    record = _run(argv, timeout=60.0)
    parsed: dict[str, Any] = {}
    raw = (record.get("stdout") or "").strip()
    if raw:
        try:
            loaded = json.loads(raw)
            if isinstance(loaded, dict):
                parsed = loaded
        except json.JSONDecodeError as exc:
            record["json_error"] = str(exc)
    record["parsed"] = parsed
    record["source"] = str(parsed.get("source") or "")
    record["err"] = parsed.get("err")
    record["decompile_status"] = str(parsed.get("status") or "")
    stubs = parsed.get("stub_methods") or []
    record["stub_methods"] = list(stubs) if isinstance(stubs, list) else []
    record["api_mode"] = str(parsed.get("mode") or mode)
    record["api_input_hash"] = str(parsed.get("input_hash") or "")
    if record.get("timeout") or record.get("os_error") or record.get("json_error"):
        record["infra"] = True
    if record.get("rc") == 2:
        record["infra"] = True
    return record


def runs_match(left: Mapping[str, Any] | None, right: Mapping[str, Any] | None) -> bool:
    if not isinstance(left, Mapping) or not isinstance(right, Mapping):
        return False
    return (
        left.get("rc") == right.get("rc")
        and left.get("stdout") == right.get("stdout")
        and left.get("stderr") == right.get("stderr")
        and not left.get("timeout")
        and not right.get("timeout")
    )


def classify_status(
    *,
    infra: bool,
    original_compile: Mapping[str, Any],
    original_verify: Mapping[str, Any] | None,
    original_run: Mapping[str, Any] | None,
    decompile: Mapping[str, Any],
    rebuilt_compile: Mapping[str, Any] | None,
    rebuilt_verify: Mapping[str, Any] | None,
    rebuilt_run: Mapping[str, Any] | None,
) -> tuple[str, str]:
    if infra:
        return "infra_error", "toolchain or probe infrastructure failed"
    if original_compile.get("rc") != 0 or original_compile.get("timeout"):
        return "invalid_input", "original fixture failed to compile"
    dec_status = str(decompile.get("decompile_status") or "")
    if decompile.get("infra"):
        return "infra_error", "decompile probe infra_error"
    if dec_status == "invalid_input":
        return "invalid_input", "DecompileResult status=invalid_input"
    if dec_status != "complete":
        return "unsupported", f"DecompileResult incomplete status={dec_status or 'empty'}"
    if decompile.get("err"):
        return "fail", f"DecompileWithOptions error: {decompile.get('err')}"
    if not rebuilt_compile or rebuilt_compile.get("rc") != 0 or rebuilt_compile.get("timeout"):
        return "fail", "rebuilt compile failed"
    if not rebuilt_verify or rebuilt_verify.get("rc") != 0 or rebuilt_verify.get("timeout"):
        return "fail", "rebuilt verify failed"
    if not original_verify or original_verify.get("rc") != 0:
        return "invalid_input", "original verify failed"
    if not original_run or original_run.get("timeout"):
        return "invalid_input", "original run failed to execute"
    if not runs_match(original_run, rebuilt_run):
        return "fail", "behavior mismatch: original vs rebuilt stdout/stderr/rc"
    if original_run.get("rc") != 0:
        return "invalid_input", "original run rc != 0"
    return "pass", "compile+verify succeeded and original/rebuilt runs match"


class LiveReplaySession:
    def __init__(self, repo: Path, work: Path) -> None:
        self.repo = Path(repo)
        self.work = Path(work)
        self.work.mkdir(parents=True, exist_ok=True)
        self.javac = _which_or_infra("javac")
        self.java = _which_or_infra("java")
        self.compiler_version = probe_compiler()
        self.lock = dependency_lock_digest(self.repo)
        self.harness = harness_digest()
        self.provisional = describe_provisional(self.repo)
        self.binaries = build_binaries(self.work, self.repo)
        self.verifier_dir = ensure_verifier(self.work, self.javac)
        self._originals: dict[tuple[str, str], dict[str, Any]] = {}

    def original_cell(self, sample: str, debug: str) -> dict[str, Any]:
        key = (sample, debug)
        cached = self._originals.get(key)
        if cached is not None:
            return cached
        if sample not in FIXTURE_RELEASE:
            raise ObservationError(f"unknown reviewed fixture {sample}")
        src = FIXTURES_DIR / f"{sample}.java"
        if not src.is_file():
            raise LiveReplayInfraError(f"infra_error: missing fixture {src}")
        source_bytes = src.read_bytes()
        source_sha = sha256_hex(source_bytes)
        expected = SEED_SOURCE_SHA256.get(sample)
        dest = self.work / "original" / sample / debug
        if dest.exists():
            shutil.rmtree(dest)
        dest.mkdir(parents=True, exist_ok=True)
        copied = dest / f"{sample}.java"
        copied.write_bytes(source_bytes)
        release = FIXTURE_RELEASE[sample]
        compile_rec = compile_java(copied, dest, javac=self.javac, release=release, debug=debug)
        class_file = dest / f"{sample}.class"
        input_hash = digest_file(class_file) if class_file.is_file() else sha256_hex(b"")
        verify_rec = None
        run_rec = None
        if compile_rec.get("ok") and class_file.is_file():
            verify_rec = _run(
                java_argv(self.java, [str(self.verifier_dir), str(dest)], "NextStageVerify") + [sample],
                timeout=20.0,
            )
            run_rec = _run(java_argv(self.java, [str(dest)], sample), timeout=20.0)
        cell = {
            "sample": sample,
            "debug": debug,
            "release": release,
            "source_path": str(src),
            "source_sha256": source_sha,
            "expected_source_sha256": expected,
            "source_sha_match": expected == source_sha,
            "dest": str(dest),
            "class_file": str(class_file) if class_file.is_file() else None,
            "input_hash": input_hash,
            "compile": compile_rec,
            "verify": verify_rec,
            "run": run_rec,
        }
        self._originals[key] = cell
        return cell

    def replay_one(self, sample: str, debug: str, mode: str) -> Observation:
        if mode not in API_MODES:
            raise ObservationError(f"MISSING_MODE: {mode!r}")
        if debug not in DEBUGS:
            raise ObservationError(f"invalid debug {debug!r}")
        original = self.original_cell(sample, debug)
        rebuilt_dir = self.work / "rebuilt" / sample / debug / mode
        if rebuilt_dir.exists():
            shutil.rmtree(rebuilt_dir)
        rebuilt_dir.mkdir(parents=True, exist_ok=True)
        infra = False
        decompile_rec: dict[str, Any] = {
            "decompile_status": "",
            "source": "",
            "err": "missing original class",
            "stub_methods": [],
            "infra": False,
        }
        rebuilt_compile = None
        rebuilt_verify = None
        rebuilt_run = None
        class_file = original.get("class_file")
        if not class_file or not Path(class_file).is_file():
            infra = bool(original["compile"].get("timeout") or original["compile"].get("os_error"))
        else:
            decompile_rec = decompile_api(Path(self.binaries["probe"]), Path(class_file), mode)
            infra = bool(decompile_rec.get("infra"))
            source_text = decompile_rec.get("source") or ""
            src_out = rebuilt_dir / f"{sample}.java"
            src_out.write_text(source_text, encoding="utf-8")
            if source_text.strip() and not infra:
                rebuilt_compile = compile_java(
                    src_out,
                    rebuilt_dir,
                    javac=self.javac,
                    release=int(original["release"]),
                    debug=debug,
                )
                rebuilt_class = rebuilt_dir / f"{sample}.class"
                if rebuilt_compile.get("ok") and rebuilt_class.is_file():
                    rebuilt_cp = [str(self.verifier_dir), str(rebuilt_dir)]
                    rebuilt_verify = _run(
                        java_argv(self.java, rebuilt_cp, "NextStageVerify") + [sample],
                        timeout=20.0,
                    )
                    rebuilt_run = _run(java_argv(self.java, [str(rebuilt_dir)], sample), timeout=20.0)
            elif decompile_rec.get("timeout") or decompile_rec.get("os_error") or decompile_rec.get("rc") == 2:
                infra = True

        original_dir = str(original["dest"])
        rebuilt_run_cp = [str(rebuilt_dir)]
        original_on_rebuilt = original_dir in rebuilt_run_cp or any(
            Path(p).resolve() == Path(original_dir).resolve() for p in rebuilt_run_cp if p
        )
        stdout_match = runs_match(original.get("run"), rebuilt_run)
        status, reason = classify_status(
            infra=infra,
            original_compile=original["compile"],
            original_verify=original.get("verify"),
            original_run=original.get("run"),
            decompile=decompile_rec,
            rebuilt_compile=rebuilt_compile,
            rebuilt_verify=rebuilt_verify,
            rebuilt_run=rebuilt_run,
        )
        if original_on_rebuilt and status == "pass":
            status, reason = "fail", "original class was on rebuilt classpath"
        if status == "pass" and not stdout_match:
            status, reason = "fail", "stdout/stderr mismatch cannot be recorded as pass"
        if status == "pass" and infra:
            status, reason = "infra_error", "infra_error cannot be recorded as pass"

        compile_ok = bool(rebuilt_compile and rebuilt_compile.get("ok"))
        verify_ok = bool(rebuilt_verify and rebuilt_verify.get("rc") == 0 and not rebuilt_verify.get("timeout"))
        execution = {
            "executed": True,
            "kind": "api-live",
            "probe": self.binaries["probe"],
            "decompile_status": decompile_rec.get("decompile_status"),
            "decompile_err": decompile_rec.get("err"),
            "stub_methods": list(decompile_rec.get("stub_methods") or []),
            "decompiled_source": decompile_rec.get("source") or "",
            "decompiled_source_sha256": sha256_hex((decompile_rec.get("source") or "").encode("utf-8")),
            "original_compile": original["compile"],
            "original_verify": original.get("verify"),
            "original_run": original.get("run"),
            "rebuilt_compile": rebuilt_compile,
            "rebuilt_verify": rebuilt_verify,
            "rebuilt_run": rebuilt_run,
            "stdout_match": stdout_match,
            "compile_ok": compile_ok,
            "verify_ok": verify_ok,
            "original_class_on_rebuilt_classpath": original_on_rebuilt,
            "rebuilt_classpath": rebuilt_run_cp,
            "original_classpath": [original_dir],
            "original_class_file": class_file,
            "rebuilt_dir": str(rebuilt_dir),
            "failure": None if status == "pass" else reason,
        }
        extras = stamp_extras(
            {
                "source_sha256": original["source_sha256"],
                "source_sha_match": original["source_sha_match"],
                "api_input_hash": decompile_rec.get("api_input_hash"),
                "javajive_sha256": self.binaries["javajive_sha256"],
            },
            self.provisional,
            role="candidate-provisional",
        )
        payload = {
            "schema_version": SCHEMA_VERSION,
            "sample": sample,
            "interface": "api",
            "status": status,
            "source": "api-live",
            "notes": (
                f"{reason}; candidate_revision={PROVISIONAL_CANDIDATE_REVISION}; "
                f"git_head={self.provisional['git_head']}; "
                f"worktree_dirty={self.provisional['worktree_dirty']}; "
                "not an immutable candidate SHA"
            ),
            "evidence": {
                "input_hash": original["input_hash"],
                "compiler_version": self.compiler_version,
                "release": int(original["release"]),
                "debug": debug,
                "mode": mode,
                "output_target": f"api-live/{sample}/{debug}/{mode}",
                "dependency_lock_digest": self.lock,
                "harness_digest": self.harness,
                "revision": self.provisional["git_head"],
            },
            "execution_evidence": execution,
            "historical_raw": None,
            "extras": extras,
        }
        return parse_observation(payload)


def run_live_replay(
    *,
    repo: Path | str | None = None,
    work: Path | str | None = None,
    samples: Sequence[str] | None = None,
    modes: Sequence[str] | None = None,
    debugs: Sequence[str] | None = None,
) -> dict[str, Any]:
    repo_path = Path(repo) if repo is not None else REPO_ROOT
    work_path = Path(work) if work is not None else (Path(os.environ.get("TMPDIR") or "/tmp") / "t01-live-replay")
    work_path.mkdir(parents=True, exist_ok=True)
    sample_list = list(samples or DEFAULT_SAMPLES)
    mode_list = list(modes or DEFAULT_MODES)
    debug_list = list(debugs or DEFAULT_DEBUGS)
    session = LiveReplaySession(repo_path, work_path)
    observations: list[Observation] = []
    for sample in sample_list:
        for debug in debug_list:
            for mode in mode_list:
                observations.append(session.replay_one(sample, debug, mode))
    return {
        "observations": observations,
        "provisional": session.provisional,
        "toolchain": {
            "compiler_version": session.compiler_version,
            "javac": session.javac,
            "java": session.java,
            "dependency_lock_digest": session.lock,
            "harness_digest": session.harness,
            "binaries": session.binaries,
            "pr_base_sha": PR_BASE_SHA,
            "milestone_sha": MILESTONE_SHA,
        },
        "work": str(work_path),
        "samples": sample_list,
        "modes": mode_list,
        "debugs": debug_list,
        "infra_error": None,
    }


def _role_dicts(observations: Sequence[Observation], provisional: Mapping[str, Any], role: str) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for obs in observations:
        payload = obs.to_dict()
        payload["extras"] = stamp_extras(payload.get("extras"), provisional, role=role)
        rows.append(parse_observation(payload).to_dict())
    return rows


def write_live_replay_evidence(
    report: Mapping[str, Any],
    dest: Path | str | None = None,
) -> Path:
    out = Path(dest) if dest is not None else EVIDENCE_DIR
    out.mkdir(parents=True, exist_ok=True)
    observations: Sequence[Observation] = report["observations"]
    provisional = report["provisional"]
    obs_dicts = [obs.to_dict() for obs in observations]
    candidate_rows = _role_dicts(observations, provisional, "candidate-provisional")
    head_rows = _role_dicts(observations, provisional, "head_replay")
    historical = load_historical_cli_report(synthesize_api_placeholders=True)
    historical_cli = [obs.to_dict() for obs in historical["cli"]]
    compare = compare_anchors(
        [parse_observation(row) for row in candidate_rows],
        [parse_observation(row) for row in head_rows],
        [],
        milestone_sha=MILESTONE_SHA,
        pr_base_sha=PR_BASE_SHA,
    )
    sources_dir = out / "sources"
    classes_dir = out / "classes"
    sources_dir.mkdir(parents=True, exist_ok=True)
    classes_dir.mkdir(parents=True, exist_ok=True)
    class_index: list[dict[str, Any]] = []
    for obs in observations:
        ee = obs.execution_evidence or {}
        src = ee.get("decompiled_source")
        if isinstance(src, str):
            path = sources_dir / obs.sample / obs.evidence.debug / f"{obs.evidence.mode}.java"
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(src, encoding="utf-8")
        orig = ee.get("original_class_file")
        if orig and Path(orig).is_file():
            class_dest = classes_dir / "original" / obs.sample / obs.evidence.debug / f"{obs.sample}.class"
            class_dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(orig, class_dest)
            class_index.append(
                {
                    "kind": "original",
                    "case_id": obs.case_id,
                    "path": str(class_dest.relative_to(out)),
                    "sha256": digest_file(class_dest),
                }
            )
        rebuilt_dir = ee.get("rebuilt_dir")
        rebuilt_class = Path(rebuilt_dir) / f"{obs.sample}.class" if rebuilt_dir else None
        if rebuilt_class is not None and rebuilt_class.is_file():
            class_dest = (
                classes_dir / "rebuilt" / obs.sample / obs.evidence.debug / obs.evidence.mode / f"{obs.sample}.class"
            )
            class_dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(rebuilt_class, class_dest)
            class_index.append(
                {
                    "kind": "rebuilt",
                    "case_id": obs.case_id,
                    "path": str(class_dest.relative_to(out)),
                    "sha256": digest_file(class_dest),
                }
            )

    candidate_doc = wrap_provisional_ledger(
        candidate_rows,
        role="candidate-provisional",
        provisional=provisional,
        extra={"source": "api-live"},
    )
    head_doc = wrap_provisional_ledger(
        head_rows,
        role="head_replay",
        provisional=provisional,
        extra={"source": "api-live"},
    )
    summary = {
        "schema_version": SCHEMA_VERSION,
        "candidate_revision": PROVISIONAL_CANDIDATE_REVISION,
        "git_head": provisional["git_head"],
        "worktree_dirty": provisional["worktree_dirty"],
        "immutable_candidate_sha": None,
        "pr_base_sha": PR_BASE_SHA,
        "milestone_sha": MILESTONE_SHA,
        "source": "api-live",
        "counts": {
            "live_api": len(obs_dicts),
            "pass": sum(1 for o in observations if o.status == "pass"),
            "fail": sum(1 for o in observations if o.status == "fail"),
            "unsupported": sum(1 for o in observations if o.status == "unsupported"),
            "infra_error": sum(1 for o in observations if o.status == "infra_error"),
            "historical_cli": len(historical_cli),
        },
        "case_ids": [obs.case_id for obs in observations],
        "statuses": {obs.case_id: obs.status for obs in observations},
        "historical_cli_case_ids": [row["case_id"] for row in historical_cli],
        "note": (
            "Live Precision/Compatibility rows are candidate-provisional and head_replay. "
            "Historical CLI 13-check rows remain separate (compatibility-cli only)."
        ),
    }
    files = {
        "observations.json": {"schema_version": SCHEMA_VERSION, "source": "api-live", "observations": obs_dicts},
        "candidate_provisional.json": candidate_doc,
        "head_replay.json": head_doc,
        "two_anchor_compare.json": compare,
        "historical_cli.json": {
            "schema_version": SCHEMA_VERSION,
            "source": "historical-cli",
            "separate": True,
            "observations": historical_cli,
        },
        "summary.json": summary,
        "toolchain.json": report["toolchain"] | {"provisional": dict(provisional)},
        "provisional.json": dict(provisional),
        "classes_index.json": {"schema_version": SCHEMA_VERSION, "files": class_index},
    }
    written: list[Path] = []
    for name, payload in files.items():
        path = out / name
        path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        written.append(path)
    sums = []
    for path in written:
        sums.append(f"{digest_file(path)}  {path.name}")
    (out / "SHA256SUMS").write_text("\n".join(sums) + "\n", encoding="utf-8")
    return out


def main(argv: list[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    dest = EVIDENCE_DIR
    work = EVIDENCE_DIR / "work"
    if "--out" in args:
        idx = args.index("--out")
        dest = Path(args[idx + 1])
    if "--work" in args:
        idx = args.index("--work")
        work = Path(args[idx + 1])
    try:
        report = run_live_replay(repo=REPO_ROOT, work=work)
        write_live_replay_evidence(report, dest)
    except LiveReplayInfraError as exc:
        print(str(exc), file=sys.stderr)
        dest.mkdir(parents=True, exist_ok=True)
        (dest / "infra_error.json").write_text(
            json.dumps({"status": "infra_error", "error": str(exc)}, indent=2) + "\n",
            encoding="utf-8",
        )
        return 2
    return 0


if __name__ == "__main__":
    if __package__ in (None, ""):
        sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
        sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
    raise SystemExit(main())
