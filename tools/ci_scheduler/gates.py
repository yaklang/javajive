"""Local repair gates vs full inventory. Observe is not pass. Promotion needs review."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Iterable


ALLOWED_STATUS = {
    "pass",
    "fail",
    "unsupported",
    "infra_error",
    "not_run",
    "observe",
    "invalid_input",
    "partial",
    "budget",
    "incomparable",
}


@dataclass(frozen=True)
class CaseObservation:
    case_id: str
    task_id: str
    status: str
    gate: bool = False


@dataclass
class GateReport:
    local_gate_pass: bool
    overall_pass: bool
    local_failures: list[str] = field(default_factory=list)
    reported_failures: list[str] = field(default_factory=list)
    hidden_failures: list[str] = field(default_factory=list)
    observe_counted_as_pass: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return {
            "local_gate_pass": self.local_gate_pass,
            "overall_pass": self.overall_pass,
            "local_failures": self.local_failures,
            "reported_failures": self.reported_failures,
            "hidden_failures": self.hidden_failures,
            "observe_counted_as_pass": self.observe_counted_as_pass,
        }


def evaluate_local_gates(
    observations: Iterable[CaseObservation],
    repaired_tasks: set[str],
) -> GateReport:
    obs = list(observations)
    ids = [o.case_id for o in obs]
    if len(ids) != len(set(ids)):
        raise ValueError(f"duplicate case ids: {ids}")
    for o in obs:
        if o.status not in ALLOWED_STATUS:
            raise ValueError(f"{o.case_id}: unknown status {o.status}")
    local_failures = []
    reported = []
    observe_as_pass = []
    for o in obs:
        is_success = o.status == "pass"
        if o.status == "observe":
            observe_as_pass.append(o.case_id)
            is_success = False
        if not is_success:
            reported.append(o.case_id)
            if o.task_id in repaired_tasks:
                local_failures.append(o.case_id)
    hidden = []
    # Failures outside repaired tasks must remain visible on the full report.
    for o in obs:
        if o.status not in {"pass"} and o.case_id not in reported:
            hidden.append(o.case_id)
    overall = not reported and not observe_as_pass
    local_ok = not local_failures
    return GateReport(
        local_gate_pass=local_ok,
        overall_pass=overall,
        local_failures=local_failures,
        reported_failures=reported,
        hidden_failures=hidden,
        observe_counted_as_pass=observe_as_pass,
    )


def promote_capability(
    case_id: str,
    *,
    from_status: str,
    to_status: str,
    review_id: str | None,
    inventory: set[str],
) -> dict[str, Any]:
    if case_id not in inventory:
        return {"ok": False, "reason": "missing_case", "case_id": case_id}
    if to_status == "pass" and from_status in {"observe", "unsupported", "fail"}:
        if not review_id:
            return {"ok": False, "reason": "promotion_requires_review", "case_id": case_id}
    if to_status == "observe" and from_status == "pass" and not review_id:
        return {"ok": False, "reason": "rollback_requires_review", "case_id": case_id}
    return {
        "ok": True,
        "case_id": case_id,
        "from": from_status,
        "to": to_status,
        "review_id": review_id,
    }
