"""Frozen SHA 31de113 Precision/Compatibility API replay.

Builds a DecompileWithOptions probe against the frozen candidate tree without
writing that tree. Live Precision/Compatibility rows are labeled
candidate_revision=31de113 (not provisional). Evidence-repo dirtiness is
unrelated. 31de113 is not claimed as latest integrated.
"""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import stat
from pathlib import Path
from typing import Any, Mapping, Sequence

from .compare import comparability_reasons, compare_pair
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
from .live_replay import (
    DEFAULT_DEBUGS,
    DEFAULT_MODES,
    DEFAULT_SAMPLES,
    FIXTURE_RELEASE,
    FIXTURES_DIR,
    LiveReplayInfraError,
    MAX_ANALYSIS_UPDATES,
    PROBE_PKG,
    _run,
    classify_status,
    compile_java,
    decompile_api,
    ensure_verifier,
    go_build_env,
    harness_digest as protocol_harness_digest,
    java_argv,
    runs_match,
)
from .observation import Observation, parse_observation
from .provisional import worktree_dirty
from .toolchain import (
    dependency_lock_digest,
    digest_file,
    probe_compiler,
    probe_revision,
    sha256_hex,
)

from tools.evidence_paths import evidence_subdir

from .anchor_checkout import (
    ENV_FROZEN,
    ENV_PR_BASE,
    protect_tree,
    refuse_write_inside_protected,
    resolve_anchor_tree,
    unique_scratch,
)

PACKAGE_DIR = Path(__file__).resolve().parent
EVIDENCE_REPO = PACKAGE_DIR.parents[1]
FROZEN_SHA = "31de113dded2fec7c34799776f43cca213aa1d77"
ANCHOR_ROLE = "frozen_31de113"
PR_BASE_ANCHOR_ROLE = "pr_base_81f8ef5_live"
EVIDENCE_DIR = evidence_subdir("t01", "frozen_31de113")
PR_BASE_LIVE_PATH = evidence_subdir("t01", "pr_base_81f8ef5") / "observations.json"
HOME_REDACT = "$HOME"


def resolve_frozen_tree(*, source_repo: Path | None = None, explicit: Path | str | None = None) -> Path:
    try:
        return resolve_anchor_tree(
            FROZEN_SHA,
            source_repo=Path(source_repo or EVIDENCE_REPO),
            explicit=explicit,
            env_key=ENV_FROZEN,
        )
    except ObservationError as exc:
        raise LiveReplayInfraError(str(exc)) from exc


def resolve_pr_base_tree(*, source_repo: Path | None = None, explicit: Path | str | None = None) -> Path:
    try:
        return resolve_anchor_tree(
            PR_BASE_SHA,
            source_repo=Path(source_repo or EVIDENCE_REPO),
            explicit=explicit,
            env_key=ENV_PR_BASE,
        )
    except ObservationError as exc:
        raise LiveReplayInfraError(str(exc)) from exc


# Import-time aliases for tests that still name the symbols. No shared /private/tmp default.
FROZEN_TREE = Path(os.environ[ENV_FROZEN]) if os.environ.get(ENV_FROZEN) else None
PR_BASE_TREE = Path(os.environ[ENV_PR_BASE]) if os.environ.get(ENV_PR_BASE) else None

# Evidence HEAD 56b59fe is a checkpoint, not the latest integrated candidate.
EVIDENCE_CHECKPOINT_NOTE = (
    "evidence HEAD is a checkpoint, NOT the latest integrated candidate; "
    "31de113 is a frozen SHA replay, not latest integrated, not provisional"
)


def frozen_harness_digest() -> str:
    """Same protocol digest as live_replay (not the frozen adapter file, not the probe binary)."""
    return protocol_harness_digest()


def _home_prefixes() -> tuple[str, ...]:
    prefixes = ["/Users/v1ll4n"]
    home = str(Path.home())
    if home and home not in ("/", ".") and home not in prefixes:
        prefixes.insert(0, home)
    return tuple(prefixes)


def redact_paths(value: Any) -> Any:
    """Replace home prefixes with $HOME so evidence JSON never embeds /Users/v1ll4n."""
    if isinstance(value, str):
        out = value
        for prefix in _home_prefixes():
            if prefix and prefix in out:
                out = out.replace(prefix, HOME_REDACT)
        return out
    if isinstance(value, list):
        return [redact_paths(item) for item in value]
    if isinstance(value, tuple):
        return [redact_paths(item) for item in value]
    if isinstance(value, dict):
        return {str(key): redact_paths(item) for key, item in value.items()}
    return value


def _outside_frozen(path: Path) -> None:
    try:
        refuse_write_inside_protected(path)
    except ObservationError as exc:
        raise LiveReplayInfraError(str(exc)) from exc


def verify_anchor_tree(tree: Path, sha: str) -> dict[str, Any]:
    if not tree.is_dir():
        raise LiveReplayInfraError(f"infra_error: anchor tree missing: {tree}")
    head = probe_revision(tree)
    if head != sha:
        raise LiveReplayInfraError(f"infra_error: tree HEAD {head} != {sha} ({tree})")
    dirty = worktree_dirty(tree)
    javajive = tree / "javajive.go"
    if not javajive.is_file():
        raise LiveReplayInfraError(f"infra_error: javajive.go missing in {tree}")
    text = javajive.read_text(encoding="utf-8")
    if "func DecompileWithOptions(" not in text:
        raise LiveReplayInfraError(f"infra_error: tree lacks DecompileWithOptions: {tree}")
    return {
        "path": str(tree),
        "candidate_revision": sha,
        "git_rev_parse_HEAD": head,
        "worktree_dirty": dirty,
        "has_decompile_with_options": True,
    }


def verify_frozen_tree(tree: Path | None = None) -> dict[str, Any]:
    resolved = Path(tree) if tree is not None else resolve_frozen_tree()
    return verify_anchor_tree(resolved, FROZEN_SHA)


def _write_probe_module(build_dir: Path, tree: Path) -> Path:
    _outside_frozen(build_dir)
    if build_dir.exists():
        shutil.rmtree(build_dir)
    build_dir.mkdir(parents=True, exist_ok=True)
    src = PROBE_PKG / "main.go"
    if not src.is_file():
        raise LiveReplayInfraError(f"infra_error: missing probe source {src}")
    dest = build_dir / "main.go"
    dest.write_bytes(src.read_bytes())
    go_mod = (
        "module t01probe\n\n"
        "go 1.22.0\n\n"
        "require github.com/yaklang/javajive v0.0.0\n\n"
        f"replace github.com/yaklang/javajive => {tree}\n"
    )
    (build_dir / "go.mod").write_text(go_mod, encoding="utf-8")
    return dest


def build_anchor_probe(
    *,
    tree: Path,
    sha: str,
    build_dir: Path,
    probe_out: Path,
) -> dict[str, Any]:
    """Compile cmd_probe against an immutable tree via go.mod replace. Writes only /tmp."""
    verify_anchor_tree(tree, sha)
    bdir = Path(build_dir)
    out = Path(probe_out)
    _outside_frozen(bdir)
    _outside_frozen(out)
    _write_probe_module(bdir, tree)
    env = go_build_env()
    env["CGO_ENABLED"] = "1"
    env["GOTOOLCHAIN"] = "local"
    env["GOFLAGS"] = "-ldflags=-linkmode=external"
    tidy = _run(
        ["go", "mod", "tidy"],
        cwd=bdir,
        env=env,
        timeout=180.0,
    )
    proxy_mode = "default"
    if tidy.get("rc") != 0:
        env_off = dict(env)
        env_off["GOPROXY"] = "off"
        tidy_off = _run(["go", "mod", "tidy"], cwd=bdir, env=env_off, timeout=180.0)
        if tidy_off.get("rc") == 0:
            tidy = tidy_off
            env = env_off
            proxy_mode = "GOPROXY=off"
        else:
            raise LiveReplayInfraError(
                "infra_error: go mod tidy for frozen probe failed: "
                + (tidy.get("stderr") or tidy.get("stdout") or "")
            )
    else:
        # Prefer offline rebuild once the module graph is resolved.
        env_off = dict(env)
        env_off["GOPROXY"] = "off"
        proxy_mode = "tidy-default then GOPROXY=off build"
        env = env_off
    argv = ["go", "build", "-ldflags=-linkmode=external", "-o", str(out), "."]
    built = _run(argv, cwd=bdir, env=env, timeout=300.0)
    if built.get("rc") != 0 or not out.is_file():
        env_retry = go_build_env()
        env_retry["CGO_ENABLED"] = "1"
        env_retry["GOTOOLCHAIN"] = "local"
        env_retry["GOFLAGS"] = "-ldflags=-linkmode=external"
        built = _run(argv, cwd=bdir, env=env_retry, timeout=300.0)
        proxy_mode = "build-retry-default-proxy"
        env = env_retry
    if built.get("rc") != 0 or not out.is_file():
        raise LiveReplayInfraError(
            "infra_error: go build frozen cmd_probe failed: "
            + (built.get("stderr") or built.get("stdout") or "")
        )
    mode = out.stat().st_mode
    if not (mode & stat.S_IXUSR):
        out.chmod(mode | stat.S_IXUSR)
    return {
        "probe": str(out),
        "probe_sha256": digest_file(out),
        "build_dir": str(bdir),
        "frozen_tree": str(tree),
        "anchor_tree": str(tree),
        "replace": f"github.com/yaklang/javajive => {tree}",
        "method": (
            "go mod init t01probe; copy cmd_probe/main.go; "
            f"replace github.com/yaklang/javajive => {tree}; "
            "CGO_ENABLED=1 GOTOOLCHAIN=local GOFLAGS=-ldflags=-linkmode=external "
            f"go build -ldflags=-linkmode=external -o {out}"
        ),
        "proxy_mode": proxy_mode,
        "cgo_enabled": "1",
        "gotoolchain": "local",
        "goflags": "-ldflags=-linkmode=external",
        "tidy": tidy,
        "build": built,
        "writes_frozen_tree": False,
        "writes_anchor_tree": False,
    }


def build_frozen_probe(*, build_dir: Path | None = None, probe_out: Path | None = None) -> dict[str, Any]:
    scratch = unique_scratch("t01-probe-31de-")
    tree = resolve_frozen_tree()
    return build_anchor_probe(
        tree=tree,
        sha=FROZEN_SHA,
        build_dir=Path(build_dir or (scratch / "build")),
        probe_out=Path(probe_out or (scratch / "probe")),
    )


def describe_anchor(
    *,
    tree: Path,
    sha: str,
    anchor_role: str,
    evidence_repo: Path | None = None,
    row_kind: str = "frozen",
    claim: str | None = None,
) -> dict[str, Any]:
    verified = verify_anchor_tree(tree, sha)
    repo = Path(evidence_repo or EVIDENCE_REPO)
    evidence_head = probe_revision(repo)
    evidence_dirty = worktree_dirty(repo)
    return {
        "schema_version": SCHEMA_VERSION,
        "candidate_revision": sha,
        "anchor_role": anchor_role,
        "row_kind": row_kind,
        "not_provisional": True,
        "not_latest_integrated": True,
        "immutable_candidate_sha": sha,
        "immutable_candidate_sha_claimed": True,
        "frozen_tree": str(tree),
        "anchor_tree": str(tree),
        "frozen_worktree_dirty": bool(verified["worktree_dirty"]),
        "git_head": sha,
        "git_rev_parse_HEAD": verified["git_rev_parse_HEAD"],
        "pr_base_sha": PR_BASE_SHA,
        "milestone_sha": MILESTONE_SHA,
        "evidence_checkpoint_head": evidence_head,
        "evidence_worktree_dirty": evidence_dirty,
        "evidence_checkpoint_is_latest_integrated": False,
        "claim": claim
        or (
            f"SHA {sha} replay via DecompileWithOptions; "
            "not provisional; not latest integrated; "
            + EVIDENCE_CHECKPOINT_NOTE
        ),
    }


def describe_frozen(*, evidence_repo: Path | None = None) -> dict[str, Any]:
    return describe_anchor(
        tree=resolve_frozen_tree(source_repo=evidence_repo),
        sha=FROZEN_SHA,
        anchor_role=ANCHOR_ROLE,
        evidence_repo=evidence_repo,
        row_kind="frozen",
        claim=(
            f"frozen SHA {FROZEN_SHA} replay via DecompileWithOptions; "
            "not provisional; not latest integrated; "
            + EVIDENCE_CHECKPOINT_NOTE
        ),
    )


def stamp_frozen_extras(extras: Mapping[str, Any] | None, frozen: Mapping[str, Any]) -> dict[str, Any]:
    sha = str(frozen.get("candidate_revision") or FROZEN_SHA)
    role = str(frozen.get("anchor_role") or ANCHOR_ROLE)
    out = dict(extras or {})
    out["candidate_revision"] = sha
    out["anchor_role"] = role
    out["row_kind"] = str(frozen.get("row_kind") or "frozen")
    out["not_provisional"] = True
    out["not_latest_integrated"] = True
    out["immutable_candidate_sha"] = sha
    out["immutable_candidate_sha_claimed"] = True
    out["git_head"] = sha
    out["git_rev_parse_HEAD"] = str(frozen.get("git_rev_parse_HEAD") or sha)
    out["frozen_worktree_dirty"] = bool(frozen.get("frozen_worktree_dirty"))
    out["worktree_dirty"] = bool(frozen.get("frozen_worktree_dirty"))
    out["evidence_checkpoint_head"] = str(frozen.get("evidence_checkpoint_head") or "")
    out["evidence_worktree_dirty"] = bool(frozen.get("evidence_worktree_dirty"))
    out["evidence_checkpoint_is_latest_integrated"] = False
    out["pr_base_sha"] = PR_BASE_SHA
    out["milestone_sha"] = MILESTONE_SHA
    out["head_is_pr_base"] = sha == PR_BASE_SHA
    return out


class FrozenReplaySession:
    def __init__(
        self,
        evidence_repo: Path,
        frozen_tree: Path,
        work: Path,
        *,
        sha: str = FROZEN_SHA,
        anchor_role: str = ANCHOR_ROLE,
        row_kind: str = "frozen",
        probe_out: Path | None = None,
        build_dir: Path | None = None,
        claim: str | None = None,
    ) -> None:
        self.repo = Path(evidence_repo)
        self.frozen_tree = protect_tree(Path(frozen_tree))
        self.sha = sha
        self.anchor_role = anchor_role
        self.row_kind = row_kind
        self.work = Path(work)
        _outside_frozen(self.work)
        self.work.mkdir(parents=True, exist_ok=True)
        self.javac = shutil.which("javac")
        self.java = shutil.which("java")
        if not self.javac or not self.java:
            raise LiveReplayInfraError("infra_error: javac/java missing")
        self.compiler_version = probe_compiler()
        self.lock = dependency_lock_digest(self.frozen_tree)
        self.harness = frozen_harness_digest()
        self.frozen = describe_anchor(
            tree=self.frozen_tree,
            sha=self.sha,
            anchor_role=self.anchor_role,
            evidence_repo=self.repo,
            row_kind=self.row_kind,
            claim=claim,
        )
        scratch = unique_scratch(f"t01-{self.sha[:12]}-sess-")
        self.probe_build = build_anchor_probe(
            tree=self.frozen_tree,
            sha=self.sha,
            build_dir=Path(build_dir or (scratch / "build")),
            probe_out=Path(probe_out or (scratch / "probe")),
        )
        self.binaries = {
            "probe": self.probe_build["probe"],
            "probe_sha256": self.probe_build["probe_sha256"],
            "cgo_enabled": "1",
            "gotoolchain": "local",
            "goflags": "-ldflags=-linkmode=external",
        }
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
            "max_analysis_updates": MAX_ANALYSIS_UPDATES,
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
        extras = stamp_frozen_extras(
            {
                "source_sha256": original["source_sha256"],
                "source_sha_match": original["source_sha_match"],
                "api_input_hash": decompile_rec.get("api_input_hash"),
                "probe_sha256": self.binaries["probe_sha256"],
                "frozen_tree": str(self.frozen_tree),
            },
            self.frozen,
        )
        payload = {
            "schema_version": SCHEMA_VERSION,
            "sample": sample,
            "interface": "api",
            "status": status,
            "source": "api-live",
            "notes": (
                f"{reason}; candidate_revision={self.sha}; "
                f"anchor_role={self.anchor_role}; frozen_worktree_dirty={self.frozen['frozen_worktree_dirty']}; "
                f"evidence_checkpoint_head={self.frozen['evidence_checkpoint_head']}; "
                + EVIDENCE_CHECKPOINT_NOTE
            ),
            "evidence": {
                "input_hash": original["input_hash"],
                "compiler_version": self.compiler_version,
                "release": int(original["release"]),
                "debug": debug,
                "mode": mode,
                "output_target": f"{self.anchor_role}/{sample}/{debug}/{mode}",
                "dependency_lock_digest": self.lock,
                "harness_digest": self.harness,
                "revision": self.sha,
            },
            "execution_evidence": execution,
            "historical_raw": None,
            "extras": extras,
        }
        return parse_observation(payload)


def run_anchor_replay(
    *,
    tree: Path,
    sha: str,
    anchor_role: str,
    work: Path,
    evidence_repo: Path | None = None,
    probe_out: Path | None = None,
    build_dir: Path | None = None,
    row_kind: str = "frozen",
    claim: str | None = None,
    samples: Sequence[str] | None = None,
    modes: Sequence[str] | None = None,
    debugs: Sequence[str] | None = None,
) -> dict[str, Any]:
    repo_path = Path(evidence_repo) if evidence_repo is not None else EVIDENCE_REPO
    work_path = Path(work)
    _outside_frozen(work_path)
    work_path.mkdir(parents=True, exist_ok=True)
    sample_list = list(samples or DEFAULT_SAMPLES)
    mode_list = list(modes or DEFAULT_MODES)
    debug_list = list(debugs or DEFAULT_DEBUGS)
    session = FrozenReplaySession(
        repo_path,
        Path(tree),
        work_path,
        sha=sha,
        anchor_role=anchor_role,
        row_kind=row_kind,
        probe_out=probe_out,
        build_dir=build_dir,
        claim=claim,
    )
    observations: list[Observation] = []
    for sample in sample_list:
        for debug in debug_list:
            for mode in mode_list:
                observations.append(session.replay_one(sample, debug, mode))
    after = verify_anchor_tree(Path(tree), sha)
    return {
        "observations": observations,
        "frozen": session.frozen,
        "frozen_after": after,
        "toolchain": {
            "compiler_version": session.compiler_version,
            "javac": session.javac,
            "java": session.java,
            "dependency_lock_digest": session.lock,
            "harness_digest": session.harness,
            "binaries": session.binaries,
            "probe_build": session.probe_build,
            "pr_base_sha": PR_BASE_SHA,
            "milestone_sha": MILESTONE_SHA,
            "candidate_revision": session.sha,
            "anchor_role": session.anchor_role,
            "protocol_harness_digest": protocol_harness_digest(),
            "not_provisional": True,
            "not_latest_integrated": True,
        },
        "work": str(work_path),
        "samples": sample_list,
        "modes": mode_list,
        "debugs": debug_list,
        "infra_error": None,
    }


def run_frozen_replay(
    *,
    evidence_repo: Path | str | None = None,
    frozen_tree: Path | str | None = None,
    work: Path | str | None = None,
    dest: Path | str | None = None,
    samples: Sequence[str] | None = None,
    modes: Sequence[str] | None = None,
    debugs: Sequence[str] | None = None,
) -> dict[str, Any]:
    repo_path = Path(evidence_repo) if evidence_repo is not None else EVIDENCE_REPO
    tree = resolve_frozen_tree(source_repo=repo_path, explicit=frozen_tree)
    scratch = unique_scratch("t01-frozen-sess-")
    report = run_anchor_replay(
        tree=tree,
        sha=FROZEN_SHA,
        anchor_role=ANCHOR_ROLE,
        work=Path(work) if work is not None else (scratch / "work"),
        evidence_repo=repo_path,
        probe_out=scratch / "probe",
        build_dir=scratch / "build",
        row_kind="frozen",
        samples=samples,
        modes=modes,
        debugs=debugs,
    )
    report["scratch"] = str(scratch)
    report["anchor_tree"] = str(tree)
    if dest is not None:
        write_frozen_replay_evidence(report, dest)
    return report


def run_pr_base_replay(
    *,
    evidence_repo: Path | str | None = None,
    tree: Path | str | None = None,
    work: Path | str | None = None,
    samples: Sequence[str] | None = None,
    modes: Sequence[str] | None = None,
    debugs: Sequence[str] | None = None,
) -> dict[str, Any]:
    repo_path = Path(evidence_repo) if evidence_repo is not None else EVIDENCE_REPO
    resolved = resolve_pr_base_tree(source_repo=repo_path, explicit=tree)
    scratch = unique_scratch("t01-prbase-sess-")
    report = run_anchor_replay(
        tree=resolved,
        sha=PR_BASE_SHA,
        anchor_role=PR_BASE_ANCHOR_ROLE,
        work=Path(work) if work is not None else (scratch / "work"),
        evidence_repo=repo_path,
        probe_out=scratch / "probe",
        build_dir=scratch / "build",
        row_kind="pr_base",
        claim=(
            f"historical PR-base SHA {PR_BASE_SHA} replay via DecompileWithOptions; "
            "same protocol harness as frozen 31de113; not provisional; not latest integrated"
        ),
        samples=samples,
        modes=modes,
        debugs=debugs,
    )
    report["scratch"] = str(scratch)
    report["anchor_tree"] = str(resolved)
    return report


def _compact_pr_base_row(payload: Mapping[str, Any]) -> dict[str, Any]:
    ee = payload.get("execution_evidence") if isinstance(payload.get("execution_evidence"), Mapping) else {}
    extras = payload.get("extras") if isinstance(payload.get("extras"), Mapping) else {}
    evidence = payload.get("evidence") if isinstance(payload.get("evidence"), Mapping) else {}
    original_run = ee.get("original_run") if isinstance(ee.get("original_run"), Mapping) else {}
    rebuilt_run = ee.get("rebuilt_run") if isinstance(ee.get("rebuilt_run"), Mapping) else {}
    return {
        "case_id": payload.get("case_id"),
        "sample": payload.get("sample"),
        "interface": payload.get("interface"),
        "status": payload.get("status"),
        "source": payload.get("source"),
        "evidence_revision": evidence.get("revision"),
        "candidate_revision": extras.get("candidate_revision"),
        "anchor_role": extras.get("anchor_role"),
        "stdout_match": ee.get("stdout_match"),
        "compile_ok": ee.get("compile_ok"),
        "verify_ok": ee.get("verify_ok"),
        "failure": ee.get("failure"),
        "original_stdout": original_run.get("stdout"),
        "rebuilt_stdout": rebuilt_run.get("stdout"),
        "decompile_status": ee.get("decompile_status"),
        "harness_digest": evidence.get("harness_digest"),
    }


def load_pr_base_live_rows(path: Path | None = None) -> dict[str, Any]:
    src = Path(path or PR_BASE_LIVE_PATH)
    if not src.is_file():
        return {
            "present": False,
            "path": str(src),
            "observations": [],
            "compact": [],
            "note": "historical 81f8ef5 live rows file not present",
        }
    raw = json.loads(src.read_text(encoding="utf-8"))
    rows = list(raw.get("observations") or [])
    parsed = [parse_observation(row) for row in rows]
    return {
        "present": True,
        "path": str(src),
        "source": "api-live",
        "separate": True,
        "anchor_role": "pr_base_81f8ef5_live",
        "git_head": PR_BASE_SHA,
        "candidate_revision": "PROVISIONAL_UNCOMMITTED",
        "note": (
            "Historical PR-base 81f8ef55603419bf737f10aa36e582f94619b35d live API rows. "
            "Separate from frozen 31de113. Not latest integrated."
        ),
        "observations": [obs.to_dict() for obs in parsed],
        "compact": [_compact_pr_base_row(obs.to_dict()) for obs in parsed],
        "origin_sha256": digest_file(src),
    }


def compare_frozen_to_pr_base(
    frozen_obs: Sequence[Observation],
    pr_base_rows: Sequence[Observation],
) -> dict[str, Any]:
    left_by = {obs.case_id: obs for obs in frozen_obs}
    right_by = {obs.case_id: obs for obs in pr_base_rows}
    case_ids = sorted(set(left_by) | set(right_by))
    cases = []
    for case_id in case_ids:
        left = left_by.get(case_id)
        right = right_by.get(case_id)
        compared = compare_pair(left, right, anchor="pr_base", case_id=case_id)
        payload = compared.to_dict()
        payload["frozen_status"] = left.status if left else None
        payload["pr_base_status"] = right.status if right else None
        if left and right:
            payload["axis_reasons"] = comparability_reasons(left, right)
        else:
            payload["axis_reasons"] = payload.get("reasons") or []
        # Informational side-by-side; incomparable toolchains never yield equality.
        payload["status_side_by_side"] = {
            "frozen_31de113": left.status if left else None,
            "pr_base_81f8ef5": right.status if right else None,
        }
        cases.append(payload)
    protocol = protocol_harness_digest()
    frozen_h = next((obs.evidence.harness_digest for obs in frozen_obs if obs is not None), None)
    pr_h = next((obs.evidence.harness_digest for obs in pr_base_rows if obs is not None), None)
    same_harness = bool(frozen_h and pr_h and frozen_h == pr_h == protocol)
    return {
        "schema_version": SCHEMA_VERSION,
        "separate": True,
        "left_anchor": ANCHOR_ROLE,
        "right_anchor": "pr_base_81f8ef5_live",
        "right_source_file": "pr_base_81f8ef5_live.json",
        "candidate_revision": FROZEN_SHA,
        "pr_base_sha": PR_BASE_SHA,
        "milestone_sha": MILESTONE_SHA,
        "not_latest_integrated": True,
        "not_provisional": True,
        "note": (
            "Historical PR-base 81f8ef5 live rows remain a separate file. "
            "31de113 is a frozen candidate SHA, not latest integrated. "
            "Both sides use protocol_harness_digest (live_replay.py + probe + verifier). "
            "Incomparable lock/input/compiler never yields equality."
        ),
        "cases": cases,
        "protocol_harness_digest": protocol,
        "same_protocol_harness_digest": same_harness,
        "harness_digest_frozen": frozen_h,
        "harness_digest_pr_base": pr_h,
    }


def write_frozen_replay_evidence(
    report: Mapping[str, Any],
    dest: Path | str | None = None,
    pr_base_report: Mapping[str, Any] | None = None,
) -> Path:
    out = Path(dest) if dest is not None else EVIDENCE_DIR
    _outside_frozen(out)
    out.mkdir(parents=True, exist_ok=True)
    observations: Sequence[Observation] = report["observations"]
    frozen = report["frozen"]
    obs_dicts = [redact_paths(obs.to_dict()) for obs in observations]
    if pr_base_report is None:
        pr_base_report = run_pr_base_replay(evidence_repo=EVIDENCE_REPO)
    if pr_base_report is not None:
        pr_base_obs = list(pr_base_report["observations"])
        pr_base = {
            "present": True,
            "generated": True,
            "origin": "run_pr_base_replay same protocol harness",
            "origin_sha256": None,
            "note": (
                "Generated against immutable PR-base tree "
                f"{PR_BASE_SHA} with protocol_harness_digest. Not latest integrated."
            ),
            "compact": [_compact_pr_base_row(obs.to_dict()) for obs in pr_base_obs],
            "observations": [obs.to_dict() for obs in pr_base_obs],
        }
    else:
        pr_base = load_pr_base_live_rows()
        pr_base_obs = [parse_observation(row) for row in pr_base.get("observations") or []]
    compare = compare_frozen_to_pr_base(observations, pr_base_obs)
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

    frozen_doc = {
        "schema_version": SCHEMA_VERSION,
        "ledger": "t01-live-api-replay",
        "row_kind": "frozen",
        "anchor_role": ANCHOR_ROLE,
        "candidate_revision": FROZEN_SHA,
        "git_head": FROZEN_SHA,
        "frozen_tree": str(frozen.get("frozen_tree") or frozen.get("anchor_tree") or ""),
        "frozen_worktree_dirty": bool(frozen.get("frozen_worktree_dirty")),
        "pr_base_sha": PR_BASE_SHA,
        "milestone_sha": MILESTONE_SHA,
        "immutable_candidate_sha": FROZEN_SHA,
        "not_provisional": True,
        "not_latest_integrated": True,
        "evidence_checkpoint_head": frozen.get("evidence_checkpoint_head"),
        "evidence_worktree_dirty": bool(frozen.get("evidence_worktree_dirty")),
        "claim": frozen.get("claim"),
        "source": "api-live",
        "observations": obs_dicts,
    }
    statuses = {obs.case_id: obs.status for obs in observations}
    summary = {
        "schema_version": SCHEMA_VERSION,
        "candidate_revision": FROZEN_SHA,
        "anchor_role": ANCHOR_ROLE,
        "row_kind": "frozen",
        "not_provisional": True,
        "not_latest_integrated": True,
        "immutable_candidate_sha": FROZEN_SHA,
        "git_head": FROZEN_SHA,
        "frozen_worktree_dirty": bool(frozen.get("frozen_worktree_dirty")),
        "evidence_checkpoint_head": frozen.get("evidence_checkpoint_head"),
        "evidence_worktree_dirty": bool(frozen.get("evidence_worktree_dirty")),
        "pr_base_sha": PR_BASE_SHA,
        "milestone_sha": MILESTONE_SHA,
        "source": "api-live",
        "counts": {
            "live_api": len(obs_dicts),
            "pass": sum(1 for o in observations if o.status == "pass"),
            "fail": sum(1 for o in observations if o.status == "fail"),
            "unsupported": sum(1 for o in observations if o.status == "unsupported"),
            "infra_error": sum(1 for o in observations if o.status == "infra_error"),
            "invalid_input": sum(1 for o in observations if o.status == "invalid_input"),
        },
        "case_ids": [obs.case_id for obs in observations],
        "statuses": statuses,
        "note": (
            "Frozen 31de113 Precision/Compatibility API rows. Not provisional. "
            "Not latest integrated. Evidence HEAD is a checkpoint. "
            "Historical 81f8ef5 live rows are a separate file."
        ),
    }
    pr_base_doc = redact_paths(
        {
            "schema_version": SCHEMA_VERSION,
            "source": "api-live",
            "separate": True,
            "anchor_role": "pr_base_81f8ef5_live",
            "git_head": PR_BASE_SHA,
            "candidate_revision": PR_BASE_SHA,
            "not_provisional": True,
            "not_latest_integrated": True,
            "origin": pr_base.get("origin") or "run_pr_base_replay",
            "origin_sha256": pr_base.get("origin_sha256"),
            "present": pr_base.get("present"),
            "generated": bool(pr_base.get("generated")),
            "protocol_harness_digest": protocol_harness_digest(),
            "note": pr_base.get("note"),
            "observations": pr_base.get("compact") or [],
        }
    )
    files = {
        "observations.json": {
            "schema_version": SCHEMA_VERSION,
            "source": "api-live",
            "anchor_role": ANCHOR_ROLE,
            "candidate_revision": FROZEN_SHA,
            "not_provisional": True,
            "not_latest_integrated": True,
            "observations": obs_dicts,
        },
        "frozen_31de113.json": frozen_doc,
        "summary.json": summary,
        "toolchain.json": redact_paths(dict(report["toolchain"]) | {"frozen": dict(frozen)}),
        "frozen.json": redact_paths(dict(frozen)),
        "classes_index.json": {"schema_version": SCHEMA_VERSION, "files": class_index},
        "pr_base_81f8ef5_live.json": pr_base_doc,
        "compare_vs_pr_base_81f8ef5.json": redact_paths(compare),
        "probe_build.json": redact_paths(report["toolchain"].get("probe_build") or {}),
    }
    written: list[Path] = []
    for name, payload in files.items():
        path = out / name
        path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        written.append(path)
    sums = [f"{digest_file(path)}  {path.name}" for path in written]
    (out / "SHA256SUMS").write_text("\n".join(sums) + "\n", encoding="utf-8")
    return out


def main(argv: list[str] | None = None) -> int:
    import sys

    args = list(sys.argv[1:] if argv is None else argv)
    dest = EVIDENCE_DIR
    work = None
    if "--out" in args:
        dest = Path(args[args.index("--out") + 1])
    if "--work" in args:
        work = Path(args[args.index("--work") + 1])
    try:
        report = run_frozen_replay(work=work)
        write_frozen_replay_evidence(report, dest)
    except LiveReplayInfraError as exc:
        print(str(exc), file=sys.stderr)
        dest.mkdir(parents=True, exist_ok=True)
        (dest / "infra_error.json").write_text(
            json.dumps({"status": "infra_error", "error": str(exc), "candidate_revision": FROZEN_SHA}, indent=2)
            + "\n",
            encoding="utf-8",
        )
        return 2
    return 0


if __name__ == "__main__":
    import sys
    from pathlib import Path as _Path

    if __package__ in (None, ""):
        sys.path.insert(0, str(_Path(__file__).resolve().parents[1]))
        sys.path.insert(0, str(_Path(__file__).resolve().parents[2]))
    raise SystemExit(main())
