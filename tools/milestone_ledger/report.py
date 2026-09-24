"""Observe-mode process success is not a semantic pass."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Mapping

from .errors import CorruptJSONError
from .observation import load_json_bytes, load_json_path
from pathlib import Path


def _extract_failed(report: Mapping[str, Any]) -> int:
    if "failed" in report and isinstance(report["failed"], int):
        return int(report["failed"])
    summary = report.get("summary") or {}
    if isinstance(summary, Mapping) and "failed" in summary:
        return int(summary["failed"])
    rows = list(report.get("observations") or []) + list(report.get("parser_observations") or [])
    if rows:
        return sum(1 for row in rows if isinstance(row, Mapping) and not row.get("pass"))
    return 0


def _extract_passed(report: Mapping[str, Any]) -> int:
    if "passed" in report and isinstance(report["passed"], int):
        return int(report["passed"])
    summary = report.get("summary") or {}
    if isinstance(summary, Mapping) and "passed" in summary:
        return int(summary["passed"])
    rows = list(report.get("observations") or []) + list(report.get("parser_observations") or [])
    if rows:
        return sum(1 for row in rows if isinstance(row, Mapping) and row.get("pass") is True)
    return 0


@dataclass(frozen=True)
class TaskReportOutcome:
    status: str
    is_success: bool
    process_exit_code: int
    failed: int
    passed: int
    policy: str | None
    reasons: tuple[str, ...]

    def to_dict(self) -> dict[str, Any]:
        return {
            "status": self.status,
            "is_success": self.is_success,
            "process_exit_code": self.process_exit_code,
            "failed": self.failed,
            "passed": self.passed,
            "policy": self.policy,
            "reasons": list(self.reasons),
        }


def classify_task_report(process_exit_code: int, report: Mapping[str, Any] | str | Path | None) -> TaskReportOutcome:
    """Map an observe/gate runner report to a ledger task status.

    Script exit 0 is never a semantic pass. failed>0 stays fail or observe,
    never complete/pass.
    """
    reasons: list[str] = []
    parsed: Mapping[str, Any] | None
    if report is None:
        return TaskReportOutcome(
            status="infra_error",
            is_success=False,
            process_exit_code=process_exit_code,
            failed=-1,
            passed=0,
            policy=None,
            reasons=("missing report",),
        )
    try:
        if isinstance(report, (str, Path)):
            path = Path(report)
            if path.is_file():
                loaded = load_json_path(path)
            else:
                loaded = load_json_bytes(str(report), origin="report")
        else:
            loaded = report
    except CorruptJSONError as exc:
        return TaskReportOutcome(
            status="infra_error",
            is_success=False,
            process_exit_code=process_exit_code,
            failed=-1,
            passed=0,
            policy=None,
            reasons=(f"CORRUPT_JSON: {exc}",),
        )
    if not isinstance(loaded, Mapping):
        return TaskReportOutcome(
            status="infra_error",
            is_success=False,
            process_exit_code=process_exit_code,
            failed=-1,
            passed=0,
            policy=None,
            reasons=("report is not an object",),
        )
    parsed = loaded
    failed = _extract_failed(parsed)
    passed = _extract_passed(parsed)
    policy = parsed.get("policy") if isinstance(parsed.get("policy"), str) else None
    claimed = str(parsed.get("status") or parsed.get("task_status") or "")

    if failed > 0:
        reasons.append(f"report.failed={failed}")
        if process_exit_code == 0:
            reasons.append("process exit 0 is not a semantic pass")
        status = "observe" if policy == "observe" else "fail"
        if claimed in {"pass", "complete", "success", "ok"}:
            reasons.append(f"refusing claimed status {claimed!r} while failed={failed}")
        return TaskReportOutcome(
            status=status,
            is_success=False,
            process_exit_code=process_exit_code,
            failed=failed,
            passed=passed,
            policy=policy,
            reasons=tuple(reasons),
        )

    if process_exit_code != 0:
        status = "infra_error" if process_exit_code == 2 else "fail"
        reasons.append(f"process exit {process_exit_code}")
        return TaskReportOutcome(
            status=status,
            is_success=False,
            process_exit_code=process_exit_code,
            failed=failed,
            passed=passed,
            policy=policy,
            reasons=tuple(reasons),
        )

    rows = list(parsed.get("observations") or []) + list(parsed.get("parser_observations") or [])
    checks = (parsed.get("summary") or {}).get("checks") if isinstance(parsed.get("summary"), Mapping) else None
    if (checks == 0) or (not rows and not passed):
        return TaskReportOutcome(
            status="fail",
            is_success=False,
            process_exit_code=process_exit_code,
            failed=failed,
            passed=passed,
            policy=policy,
            reasons=("empty results are not a pass",),
        )

    if claimed in {"complete"}:
        # complete describes decompilation, not ledger success
        reasons.append("decompiler complete is not a ledger pass; treating as observe without failed counts")
        return TaskReportOutcome(
            status="observe",
            is_success=False,
            process_exit_code=process_exit_code,
            failed=failed,
            passed=passed,
            policy=policy,
            reasons=tuple(reasons),
        )

    return TaskReportOutcome(
        status="pass",
        is_success=True,
        process_exit_code=process_exit_code,
        failed=failed,
        passed=passed,
        policy=policy,
        reasons=("no failed observations and nonempty inventory",),
    )
