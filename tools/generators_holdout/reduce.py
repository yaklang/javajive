"""Minimize a failing sample while keeping it legal and in the same observed failure class.

The default classifier is a live JavaJive compile/verify/decompile/rebuild/run
oracle. A regex over source features is NOT a reproduced defect and is not used
as the reduction predicate.
"""

from __future__ import annotations

import hashlib
import re
import shutil
import tempfile
from pathlib import Path
from typing import Callable

from .identity import CompilerIdentity, InfraError
from .pipeline import FaultInjectedOracle, PipelineResult, extract_class_name, run_pipeline

FAILURE_INVALID = "invalid_input"


class ReductionError(Exception):
    pass


def observed_failure_class(result: PipelineResult) -> str:
    return result.failure_class


def classify_original_legality(result: PipelineResult) -> str:
    compile_st = result.stages.get("compile")
    orig = result.stages.get("original_verify_run")
    if compile_st and compile_st.status != "ok":
        return "invalid_input"
    if orig is None:
        return "invalid_input"
    if orig.status == "infra_error":
        return "infra_error"
    if orig.status in {"verify_fail", "linkage_error", "run_fail", "compile_fail"}:
        return "invalid_input"
    if orig.status == "run_ok":
        return "legal"
    return orig.status


def _legal_original(identity: CompilerIdentity, source: str, class_name: str, work: Path) -> tuple[bool, str, PipelineResult | None]:
    """Original input must compile, verify, and run before it can be a decompiler defect."""
    try:
        result = run_pipeline(source, identity, work, mode="precision", class_name=class_name)
    except InfraError as exc:
        return False, "infra_error", None
    kind = classify_original_legality(result)
    return kind == "legal", kind, result


def reduce_source(
    source: str,
    identity: CompilerIdentity,
    *,
    predicate: Callable[[str], str | None] | None = None,
    work: Path | None = None,
    mode: str = "precision",
) -> dict:
    """Delta-debug line deletion. Each kept trial re-runs the live oracle unless a
    labeled harness-control predicate is supplied (FaultInjectedOracle).
    """
    class_name = extract_class_name(source)
    tmp_own = None
    if work is None:
        tmp_own = Path(tempfile.mkdtemp(prefix="t28-reduce-"))
        work = tmp_own
    try:
        legal, kind, original_obs = _legal_original(identity, source, class_name, work / "orig")
        if kind == "infra_error":
            raise ReductionError("infra_error: cannot reduce without a working toolchain")
        if not legal:
            raise ReductionError(
                f"original source is not a legal runnable class ({kind}); cannot claim semantic defect"
            )
        assert original_obs is not None

        if predicate is not None:
            original_class = predicate(source)
            if original_class is None:
                raise ReductionError("harness-control predicate found no injected failure")
            oracle_kind = "harness_control"
        else:
            original_class = original_obs.failure_class
            if original_class in {"pass", "infra_error"}:
                raise ReductionError(
                    f"live oracle status {original_class!r} is not a reducible decompiler defect"
                )
            oracle_kind = "javajive_live"

        def trial_class(text: str, folder: Path) -> tuple[str | None, str]:
            if predicate is not None:
                # Still require the trial to be a legal class via compile/verify/run,
                # but the failure label comes from the labeled harness control.
                ok, lkind, _ = _legal_original(identity, text, class_name, folder)
                if lkind == "infra_error":
                    return "infra_error", lkind
                if not ok:
                    return None, lkind
                return predicate(text), "legal"
            try:
                obs = run_pipeline(text, identity, folder, mode=mode, class_name=class_name)
            except InfraError:
                return "infra_error", "infra_error"
            legality = classify_original_legality(obs)
            if legality != "legal":
                return None, legality
            return obs.failure_class, legality

        lines = source.splitlines(keepends=True)
        changed = True
        while changed:
            changed = False
            i = 0
            while i < len(lines):
                line = lines[i]
                stripped = line.strip()
                if stripped.startswith("public class") or stripped.startswith("public static void main"):
                    i += 1
                    continue
                if stripped in {"{", "}", ""}:
                    i += 1
                    continue
                trial_lines = lines[:i] + lines[i + 1 :]
                trial = "".join(trial_lines)
                fc, legality = trial_class(trial, work / "trial")
                if legality != "legal" or fc != original_class:
                    i += 1
                    continue
                lines = trial_lines
                changed = True
                continue
        reduced = "".join(lines)
        fc, legality = trial_class(reduced, work / "final")
        if legality != "legal":
            raise ReductionError(f"reduction produced illegal class ({legality})")
        if fc != original_class:
            raise ReductionError("reduction lost observed failure class")
        return {
            "original_sha256": hashlib.sha256(source.encode()).hexdigest(),
            "reduced_sha256": hashlib.sha256(reduced.encode()).hexdigest(),
            "original": source,
            "reduced": reduced,
            "failure_class": original_class,
            "legal": True,
            "shrunk": len(reduced) < len(source),
            "oracle_kind": oracle_kind,
            "original_observation": original_obs.to_dict() if oracle_kind == "javajive_live" else None,
        }
    finally:
        if tmp_own is not None:
            shutil.rmtree(tmp_own, ignore_errors=True)


def reject_illegal_as_semantic(source: str, identity: CompilerIdentity) -> dict:
    class_name = extract_class_name(source)
    with tempfile.TemporaryDirectory(prefix="t28-illegal-") as raw:
        work = Path(raw)
        ok, kind, obs = _legal_original(identity, source, class_name, work)
        return {
            "legal": ok,
            "failure_class": FAILURE_INVALID if not ok else (obs.failure_class if obs else "pass"),
            "stage": kind,
            "may_label_semantic_defect": bool(ok and obs and obs.failure_class not in {"pass", "infra_error"}),
            "oracle_kind": "javajive_live",
        }


# Kept only as a negative control in tests: must NOT be used as a defect oracle.
def classify_source_failure(source: str) -> str | None:
    raise RuntimeError(
        "classify_source_failure is a source-feature regex and is not a reproduced "
        "JavaJive defect; use run_pipeline / reduce_source live oracle"
    )


def feature_regex_negative_control(source: str) -> str | None:
    """Intentionally NOT an oracle. Tests assert this must not be treated as a defect."""
    if re.search(r"\\uD[89A-Fa-f][0-9A-Fa-f]{2}", source) or any(
        0xD800 <= ord(ch) <= 0xDFFF for ch in source
    ):
        return "unicode_surrogate_pair_FEATURE_ONLY"
    if "pick(Object" in source and "pick(String" in source:
        return "overload_witness_FEATURE_ONLY"
    compact = re.sub(r"\s+", "", source)
    if "t=a;a=b;b=t" in compact:
        return "cfg_phi_swap_FEATURE_ONLY"
    return None
