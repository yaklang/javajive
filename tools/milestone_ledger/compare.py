"""Two-anchor comparison. Incomparable toolchains never yield equality."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Sequence

from .constants import COMPARABILITY_AXES, MILESTONE_SHA, PR_BASE_SHA, SUCCESS_STATUSES
from .observation import Observation, parse_observation, parse_observation_list


def _as_obs_list(value: Any) -> list[Observation]:
    if value is None:
        return []
    if isinstance(value, Observation):
        return [value]
    if isinstance(value, Sequence) and not isinstance(value, (str, bytes, bytearray)):
        if value and all(isinstance(item, Observation) for item in value):
            return list(value)
        return [parse_observation(item) for item in value]
    return parse_observation_list(value)


@dataclass
class CaseComparison:
    case_id: str
    anchor: str
    verdict: str
    left_status: str | None
    right_status: str | None
    reasons: list[str] = field(default_factory=list)
    comparable: bool = True
    equality_conclusion: bool | None = None
    no_regression_conclusion: bool | None = None
    status_equal: bool | None = None
    runtime_equal: bool | None = None
    source_equal: bool | None = None
    runtime_proof: bool = False

    def to_dict(self) -> dict[str, Any]:
        payload = {
            "case_id": self.case_id,
            "anchor": self.anchor,
            "verdict": self.verdict,
            "left_status": self.left_status,
            "right_status": self.right_status,
            "reasons": list(self.reasons),
            "comparable": self.comparable,
            "status_equal": self.status_equal,
            "runtime_equal": self.runtime_equal,
            "source_equal": self.source_equal,
            "runtime_proof": self.runtime_proof,
        }
        # Incomparable/infra must not emit equality / no-regression conclusions.
        if self.verdict in {"incomparable", "infra_error"}:
            payload["equality_conclusion"] = None
            payload["no_regression_conclusion"] = None
            payload["equals"] = False
            payload["no_regression"] = False
        else:
            payload["equality_conclusion"] = self.equality_conclusion
            payload["no_regression_conclusion"] = self.no_regression_conclusion
            payload["equals"] = bool(self.equality_conclusion)
            payload["no_regression"] = bool(self.no_regression_conclusion)
        return payload


def _run_payload(ee: dict[str, Any] | None, key: str) -> tuple[Any, Any, Any]:
    rec = (ee or {}).get(key) if isinstance(ee, dict) else None
    if not isinstance(rec, dict):
        return (None, None, None)
    return (rec.get("stdout"), rec.get("stderr"), rec.get("rc"))


def observation_runtime(obs: Observation) -> tuple[Any, ...] | None:
    """Runtime/oracle payload. Decompiled source SHA is format, not behavior."""
    ee = obs.execution_evidence if isinstance(obs.execution_evidence, dict) else None
    if not ee:
        return None
    orig = _run_payload(ee, "original_run")
    rebuilt = _run_payload(ee, "rebuilt_run")
    payload = (
        orig,
        rebuilt,
        ee.get("failure"),
        ee.get("stdout_match"),
        ee.get("decompile_status"),
        ee.get("failure_stage") or ee.get("stage"),
    )
    if payload == ((None, None, None), (None, None, None), None, None, None, None):
        return None
    return payload


def observation_source(obs: Observation) -> str | None:
    ee = obs.execution_evidence if isinstance(obs.execution_evidence, dict) else None
    if not ee:
        return None
    sha = ee.get("decompiled_source_sha256")
    if not sha:
        return None
    return str(sha)


def observation_behavior(obs: Observation) -> tuple[Any, ...] | None:
    """Backward-compatible alias: runtime payload only (not decompiled source SHA)."""
    return observation_runtime(obs)


def comparability_reasons(left: Observation, right: Observation) -> list[str]:
    reasons: list[str] = []
    for axis in COMPARABILITY_AXES:
        lv = left.evidence.axis(axis)
        rv = right.evidence.axis(axis)
        if lv != rv:
            reasons.append(f"{axis} differs: {lv!r} vs {rv!r}")
    return reasons


def compare_pair(
    candidate: Observation | None,
    reference: Observation | None,
    *,
    anchor: str,
    case_id: str,
) -> CaseComparison:
    if candidate is None and reference is None:
        return CaseComparison(
            case_id=case_id,
            anchor=anchor,
            verdict="infra_error",
            left_status=None,
            right_status=None,
            reasons=["both sides missing"],
            comparable=False,
        )
    if candidate is not None and candidate.status == "infra_error":
        return CaseComparison(
            case_id=case_id,
            anchor=anchor,
            verdict="infra_error",
            left_status=candidate.status,
            right_status=reference.status if reference else None,
            reasons=["candidate run is infra_error"],
            comparable=False,
        )
    if reference is not None and reference.status == "infra_error":
        return CaseComparison(
            case_id=case_id,
            anchor=anchor,
            verdict="infra_error",
            left_status=candidate.status if candidate else None,
            right_status=reference.status,
            reasons=["reference run is infra_error"],
            comparable=False,
        )
    if candidate is None or reference is None:
        missing_side = "candidate" if candidate is None else "reference"
        ref_pass = reference is not None and reference.status in SUCCESS_STATUSES
        cand_pass = candidate is not None and candidate.status in SUCCESS_STATUSES
        reasons = [f"{missing_side} missing observation for {case_id}"]
        if anchor == "milestone" and missing_side == "candidate" and ref_pass:
            return CaseComparison(
                case_id=case_id,
                anchor=anchor,
                verdict="drift",
                left_status=None,
                right_status=reference.status if reference else None,
                reasons=reasons + ["milestone had a capability the candidate lacks"],
                comparable=True,
                equality_conclusion=False,
                no_regression_conclusion=False,
            )
        if anchor == "pr_base" and missing_side == "candidate" and ref_pass:
            return CaseComparison(
                case_id=case_id,
                anchor=anchor,
                verdict="regression",
                left_status=None,
                right_status=reference.status if reference else None,
                reasons=reasons,
                comparable=True,
                equality_conclusion=False,
                no_regression_conclusion=False,
            )
        if missing_side == "reference" and cand_pass:
            return CaseComparison(
                case_id=case_id,
                anchor=anchor,
                verdict="improvement",
                left_status=candidate.status if candidate else None,
                right_status=None,
                reasons=reasons,
                comparable=True,
                equality_conclusion=False,
                no_regression_conclusion=True,
            )
        # Both missing a capability relative to a third party is handled at the
        # two-anchor layer; a single missing side with non-pass reference is equal-absent.
        return CaseComparison(
            case_id=case_id,
            anchor=anchor,
            verdict="missing",
            left_status=candidate.status if candidate else None,
            right_status=reference.status if reference else None,
            reasons=reasons,
            comparable=True,
            equality_conclusion=False,
            no_regression_conclusion=not ref_pass,
        )

    reasons = comparability_reasons(candidate, reference)
    if reasons:
        return CaseComparison(
            case_id=case_id,
            anchor=anchor,
            verdict="incomparable",
            left_status=candidate.status,
            right_status=reference.status,
            reasons=reasons,
            comparable=False,
            equality_conclusion=None,
            no_regression_conclusion=None,
        )

    left, right = candidate.status, reference.status
    status_equal = left == right
    if left == right:
        lb = observation_runtime(candidate)
        rb = observation_runtime(reference)
        ls = observation_source(candidate)
        rs = observation_source(reference)
        source_equal = None if (ls is None and rs is None) else (ls == rs)
        if (lb is None) != (rb is None):
            return CaseComparison(
                case_id=case_id,
                anchor=anchor,
                verdict="status_match_incomplete_observation",
                left_status=left,
                right_status=right,
                reasons=[
                    "one side lacks runtime payload; status match is not behavior equality or no-regression",
                ],
                comparable=True,
                equality_conclusion=False,
                no_regression_conclusion=False,
                status_equal=True,
                runtime_equal=None,
                source_equal=source_equal,
                runtime_proof=False,
            )
        if lb is None and rb is None:
            return CaseComparison(
                case_id=case_id,
                anchor=anchor,
                verdict="status_match",
                left_status=left,
                right_status=right,
                reasons=["synthetic/status-only ledger: status relation only, not runtime proof"],
                comparable=True,
                equality_conclusion=False,
                no_regression_conclusion=False,
                status_equal=True,
                runtime_equal=None,
                source_equal=source_equal,
                runtime_proof=False,
            )
        if lb != rb:
            return CaseComparison(
                case_id=case_id,
                anchor=anchor,
                verdict="status_match_observation_differs",
                left_status=left,
                right_status=right,
                reasons=[
                    "same ledger status is not behavior equivalence",
                    f"runtime differs: {lb!r} vs {rb!r}",
                ],
                comparable=True,
                equality_conclusion=False,
                no_regression_conclusion=False,
                status_equal=True,
                runtime_equal=False,
                source_equal=source_equal,
                runtime_proof=True,
            )
        reasons = []
        if source_equal is False:
            reasons.append("decompiled_source_sha256 differs; format change is not semantic regression")
        verdict = "equal"
        equals = True
        no_reg = True
        if anchor == "milestone" and right in SUCCESS_STATUSES and left not in SUCCESS_STATUSES:
            verdict = "drift"
            equals = False
            no_reg = False
        return CaseComparison(
            case_id=case_id,
            anchor=anchor,
            verdict=verdict,
            left_status=left,
            right_status=right,
            reasons=reasons,
            comparable=True,
            equality_conclusion=equals,
            no_regression_conclusion=no_reg,
            status_equal=True,
            runtime_equal=True,
            source_equal=source_equal,
            runtime_proof=True,
        )

    lost_capability = right in SUCCESS_STATUSES and left not in SUCCESS_STATUSES
    gained = left in SUCCESS_STATUSES and right not in SUCCESS_STATUSES
    if lost_capability:
        verdict = "drift" if anchor == "milestone" else "regression"
        return CaseComparison(
            case_id=case_id,
            anchor=anchor,
            verdict=verdict,
            left_status=left,
            right_status=right,
            reasons=[f"lost capability versus {anchor}: {right} -> {left}"],
            comparable=True,
            equality_conclusion=False,
            no_regression_conclusion=False,
            status_equal=False,
            runtime_equal=False,
            runtime_proof=bool(observation_runtime(candidate) and observation_runtime(reference)),
        )
    if gained:
        return CaseComparison(
            case_id=case_id,
            anchor=anchor,
            verdict="improvement",
            left_status=left,
            right_status=right,
            reasons=[f"gained capability versus {anchor}: {right} -> {left}"],
            comparable=True,
            equality_conclusion=False,
            no_regression_conclusion=True,
            status_equal=False,
            runtime_equal=False,
            runtime_proof=bool(observation_runtime(candidate) and observation_runtime(reference)),
        )
    return CaseComparison(
        case_id=case_id,
        anchor=anchor,
        verdict="status_change",
        left_status=left,
        right_status=right,
        reasons=[f"status {right} -> {left}"],
        comparable=True,
        equality_conclusion=False,
        no_regression_conclusion=left not in {"fail", "infra_error"} or right not in SUCCESS_STATUSES,
        status_equal=False,
        runtime_proof=bool(observation_runtime(candidate) and observation_runtime(reference)),
    )


def _summarize(rows: list[CaseComparison]) -> dict[str, Any]:
    verdicts = [row.verdict for row in rows]
    incomparable = sum(v == "incomparable" for v in verdicts)
    infra = sum(v == "infra_error" for v in verdicts)
    drift = sum(v == "drift" for v in verdicts)
    regression = sum(v == "regression" for v in verdicts)
    equal = sum(v == "equal" for v in verdicts)
    if infra:
        headline = "infra_error"
    elif incomparable:
        headline = "incomparable"
    elif drift:
        headline = "drift"
    elif regression:
        headline = "regression"
    elif equal == len(rows) and rows:
        headline = "equal"
    else:
        headline = "mixed"
    equality_emitted = any(row.verdict == "incomparable" and row.equality_conclusion is True for row in rows)
    return {
        "verdict": headline,
        "counts": {
            "equal": equal,
            "drift": drift,
            "regression": regression,
            "incomparable": incomparable,
            "infra_error": infra,
            "improvement": sum(v == "improvement" for v in verdicts),
            "status_change": sum(v == "status_change" for v in verdicts),
            "missing": sum(v == "missing" for v in verdicts),
        },
        "cases": [row.to_dict() for row in rows],
        "equality_conclusion_on_incomparable": equality_emitted,
    }


def compare_anchors(
    candidate: Any,
    pr_base: Any,
    milestone: Any,
    *,
    milestone_sha: str = MILESTONE_SHA,
    pr_base_sha: str = PR_BASE_SHA,
) -> dict[str, Any]:
    """Compare candidate vs PR base and candidate vs immutable milestone.

    If base==candidate but both lost a capability the milestone had, the
    overall verdict is long-term drift, not "no change".
    """
    cand_rows = _as_obs_list(candidate)
    base_rows = _as_obs_list(pr_base)
    mile_rows = _as_obs_list(milestone)
    cand_map = {obs.case_id: obs for obs in cand_rows}
    base_map = {obs.case_id: obs for obs in base_rows}
    mile_map = {obs.case_id: obs for obs in mile_rows}
    case_ids = sorted(set(cand_map) | set(base_map) | set(mile_map))

    vs_base = [
        compare_pair(cand_map.get(cid), base_map.get(cid), anchor="pr_base", case_id=cid) for cid in case_ids
    ]
    vs_mile = [
        compare_pair(cand_map.get(cid), mile_map.get(cid), anchor="milestone", case_id=cid)
        for cid in case_ids
    ]

    base_summary = _summarize(vs_base)
    mile_summary = _summarize(vs_mile)
    long_term_drift = any(row.verdict == "drift" for row in vs_mile)
    status_equal_to_base = all(row.status_equal for row in vs_base) and bool(vs_base)
    runtime_equal_to_base = all(row.verdict == "equal" and row.runtime_proof for row in vs_base) and bool(vs_base)
    equal_to_base = all(row.verdict == "equal" and row.runtime_proof for row in vs_base) and bool(vs_base)
    incomparable_any = any(row.verdict == "incomparable" for row in vs_base + vs_mile)
    infra_any = any(row.verdict == "infra_error" for row in vs_base + vs_mile)

    if infra_any:
        overall = "infra_error"
    elif incomparable_any:
        overall = "incomparable"
    elif long_term_drift:
        overall = "long_term_drift"
    elif any(row.verdict == "regression" for row in vs_base):
        overall = "regression"
    elif equal_to_base and not long_term_drift:
        overall = "equal_to_pr_base"
    else:
        overall = "mixed"

    insufficient = bool(long_term_drift)
    return {
        "milestone_sha": milestone_sha,
        "pr_base_sha": pr_base_sha,
        "candidate_vs_pr_base": base_summary,
        "candidate_vs_milestone": mile_summary,
        "equal_to_pr_base": equal_to_base,
        "status_equal_to_pr_base": status_equal_to_base,
        "runtime_equal_to_pr_base": runtime_equal_to_base,
        "long_term_drift": long_term_drift,
        "no_regression_vs_base_only_insufficient": insufficient,
        "overall_verdict": overall,
        "case_ids": case_ids,
    }
