"""Correctness ledger (strict gates) and defect ledger (known failures)."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any, Mapping

from .errors import ExpectationChangeError, PromotionError
from .observation import Observation, is_semantic_pass, parse_observation


def _now() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


@dataclass(frozen=True)
class ReviewRecord:
    case_id: str
    reviewer: str
    action: str
    from_status: str
    to_status: str
    reason: str
    timestamp: str
    revision: str
    input_hash: str = ""

    def validate(self) -> None:
        if self.action not in {"promotion", "expectation_change"}:
            raise PromotionError(f"unknown review action {self.action!r}")
        if not str(self.reviewer or "").strip():
            raise PromotionError("review record missing reviewer")
        if not str(self.reason or "").strip():
            raise PromotionError("review record missing reason")
        if not str(self.case_id or "").strip():
            raise PromotionError("review record missing case_id")
        if not str(self.revision or "").strip():
            raise PromotionError("review record missing revision")
        if len(self.revision) != 40:
            raise PromotionError("review record revision must be a 40-hex git SHA")

    def to_dict(self) -> dict[str, Any]:
        return {
            "case_id": self.case_id,
            "reviewer": self.reviewer,
            "action": self.action,
            "from_status": self.from_status,
            "to_status": self.to_status,
            "reason": self.reason,
            "timestamp": self.timestamp,
            "revision": self.revision,
            "input_hash": self.input_hash,
        }

    @classmethod
    def from_mapping(cls, data: Mapping[str, Any]) -> "ReviewRecord":
        rec = cls(
            case_id=str(data.get("case_id") or ""),
            reviewer=str(data.get("reviewer") or ""),
            action=str(data.get("action") or ""),
            from_status=str(data.get("from_status") or ""),
            to_status=str(data.get("to_status") or ""),
            reason=str(data.get("reason") or ""),
            timestamp=str(data.get("timestamp") or _now()),
            revision=str(data.get("revision") or ""),
            input_hash=str(data.get("input_hash") or ""),
        )
        rec.validate()
        return rec


class DefectLedger:
    """Known failures with history. A historical fail is never a pass."""

    def __init__(self) -> None:
        self._records: list[dict[str, Any]] = []

    def record(self, observation: Observation | Mapping[str, Any]) -> dict[str, Any]:
        obs = parse_observation(observation)
        if is_semantic_pass(obs):
            raise PromotionError("defect ledger refuses to record a semantic pass as a defect")
        row = {
            "case_id": obs.case_id,
            "input_hash": obs.evidence.input_hash,
            "status": obs.status,
            "revision": obs.evidence.revision,
            "recorded_at": _now(),
            "observation": obs.to_dict(),
            "historical_fail": obs.status == "fail"
            or (obs.historical_raw or {}).get("pass") is False,
        }
        self._records.append(row)
        return row

    def history(self, case_id: str, input_hash: str) -> list[dict[str, Any]]:
        return [
            dict(row)
            for row in self._records
            if row["case_id"] == case_id and row["input_hash"] == input_hash
        ]

    def has_historical_fail(self, case_id: str, input_hash: str) -> bool:
        return any(row.get("historical_fail") or row["status"] == "fail" for row in self.history(case_id, input_hash))

    def never_pass_records(self, case_id: str, input_hash: str) -> list[dict[str, Any]]:
        return [row for row in self.history(case_id, input_hash) if row["status"] != "pass"]

    def as_pass_count(self) -> int:
        # Historical fails must contribute zero passes.
        return 0

    def to_list(self) -> list[dict[str, Any]]:
        return [dict(row) for row in self._records]


class CorrectnessLedger:
    """Current strict gates. A pass here is a real pass."""

    def __init__(self, defects: DefectLedger | None = None) -> None:
        self.defects = defects if defects is not None else DefectLedger()
        self._gates: dict[tuple[str, str], dict[str, Any]] = {}
        self._reviews: list[ReviewRecord] = []
        self._expectations: dict[tuple[str, str], str] = {}

    def ingest(self, observation: Observation | Mapping[str, Any], review: ReviewRecord | Mapping[str, Any] | None = None) -> dict[str, Any]:
        obs = parse_observation(observation)
        key = (obs.case_id, obs.evidence.input_hash)
        rec = None
        if isinstance(review, Mapping):
            rec = ReviewRecord.from_mapping(review)
        elif isinstance(review, ReviewRecord):
            rec = review
            rec.validate()
        if not is_semantic_pass(obs):
            self.defects.record(obs)
            return {"action": "recorded_defect", "case_id": obs.case_id, "input_hash": obs.evidence.input_hash}
        if self.defects.has_historical_fail(*key):
            if rec is None:
                raise PromotionError(
                    "pass after historical fail requires an explicit reviewed promotion; silent flip refused"
                )
            return self.promote(obs, rec)
        self._gates[key] = {
            "observation": obs.to_dict(),
            "promoted": False,
            "gated_at": _now(),
        }
        self._expectations[key] = "pass"
        return {"action": "gated", "case_id": obs.case_id, "input_hash": obs.evidence.input_hash}

    def promote(self, observation: Observation | Mapping[str, Any], review: ReviewRecord | Mapping[str, Any]) -> dict[str, Any]:
        obs = parse_observation(observation)
        rec = review if isinstance(review, ReviewRecord) else ReviewRecord.from_mapping(review)
        rec.validate()
        if rec.action != "promotion":
            raise PromotionError("review action must be promotion")
        if rec.case_id != obs.case_id:
            raise PromotionError("review case_id does not match observation")
        if rec.input_hash and rec.input_hash != obs.evidence.input_hash:
            raise PromotionError("review input_hash does not match observation")
        if not is_semantic_pass(obs):
            raise PromotionError("promotion requires a real semantic pass")
        key = (obs.case_id, obs.evidence.input_hash)
        if not self.defects.has_historical_fail(*key):
            raise PromotionError("no historical fail to promote from")
        # Defect history is retained; contract becomes a strict gate.
        self._gates[key] = {
            "observation": obs.to_dict(),
            "promoted": True,
            "review": rec.to_dict(),
            "gated_at": _now(),
        }
        self._expectations[key] = "pass"
        self._reviews.append(rec)
        return {
            "action": "promoted",
            "case_id": obs.case_id,
            "input_hash": obs.evidence.input_hash,
            "defect_history_retained": True,
        }

    def change_expectation(
        self,
        case_id: str,
        input_hash: str,
        new_expected: str,
        review: ReviewRecord | Mapping[str, Any] | None,
    ) -> dict[str, Any]:
        if review is None:
            raise ExpectationChangeError(
                "expectation changes require an independent review record; silent flip refused"
            )
        rec = review if isinstance(review, ReviewRecord) else ReviewRecord.from_mapping(review)
        rec.validate()
        if rec.action != "expectation_change":
            raise ExpectationChangeError("review action must be expectation_change")
        key = (case_id, input_hash)
        previous = self._expectations.get(key, "unset")
        self._expectations[key] = new_expected
        self._reviews.append(rec)
        return {"action": "expectation_change", "from": previous, "to": new_expected, "review": rec.to_dict()}

    def is_gated(self, case_id: str, input_hash: str) -> bool:
        return (case_id, input_hash) in self._gates

    def gate(self, case_id: str, input_hash: str) -> dict[str, Any] | None:
        row = self._gates.get((case_id, input_hash))
        return dict(row) if row else None

    def was_always_pass(self, case_id: str, input_hash: str) -> bool:
        if self.defects.has_historical_fail(case_id, input_hash):
            return False
        gate = self._gates.get((case_id, input_hash))
        if not gate:
            return False
        return gate["observation"]["status"] == "pass" and not gate.get("promoted")

    def evaluate(self, observation: Observation | Mapping[str, Any]) -> str:
        obs = parse_observation(observation)
        if not is_semantic_pass(obs):
            return obs.status if obs.status != "pass" else "fail"
        return "pass"

    def to_dict(self) -> dict[str, Any]:
        return {
            "gates": [
                {"case_id": key[0], "input_hash": key[1], **value}
                for key, value in self._gates.items()
            ],
            "reviews": [rec.to_dict() for rec in self._reviews],
            "expectations": {f"{k[0]}::{k[1]}": v for k, v in self._expectations.items()},
        }
