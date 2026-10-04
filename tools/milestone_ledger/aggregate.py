"""Inventory conservation: missing IDs are errors; equal counts cannot hide swaps."""

from __future__ import annotations

from collections import Counter
from dataclasses import dataclass, field
from typing import Any, Mapping, Sequence

from .case_id import interface_for_mode, make_case_id
from .constants import NEVER_PASS_STATUSES
from .errors import CorruptJSONError, InventoryError, ObservationError
from .observation import (
    Observation,
    extract_observation_rows,
    is_semantic_pass,
    parse_observation,
)


def expand_manifest(manifest: Mapping[str, Any] | Sequence[str]) -> list[str]:
    if isinstance(manifest, Sequence) and not isinstance(manifest, (str, bytes, Mapping)):
        ids = [str(item) for item in manifest]
        if not ids:
            raise InventoryError("EMPTY_RESULTS: manifest has no expected IDs")
        return ids
    if not isinstance(manifest, Mapping):
        raise InventoryError("manifest must be an object or list of expected IDs")
    if manifest.get("expected_ids"):
        ids = [str(item) for item in manifest["expected_ids"]]
        if not ids:
            raise InventoryError("EMPTY_RESULTS: manifest expected_ids is empty")
        return ids
    samples = list(manifest.get("samples") or manifest.get("cases") or [])
    modes = list(manifest.get("modes") or [])
    debugs = list(manifest.get("debugs") or [])
    if not samples or not modes or not debugs:
        raise InventoryError("manifest requires samples, modes, and debugs (or expected_ids)")
    ids: list[str] = []
    interfaces_modes = manifest.get("interfaces_modes")
    if interfaces_modes:
        for sample in samples:
            for pair in interfaces_modes:
                if isinstance(pair, Mapping):
                    iface, mode = pair["interface"], pair["mode"]
                else:
                    iface, mode = pair
                for debug in debugs:
                    ids.append(make_case_id(str(sample), str(iface), str(mode), str(debug)))
        return ids
    interface = manifest.get("interface")
    for sample in samples:
        for mode in modes:
            iface = str(interface) if interface else interface_for_mode(str(mode))
            for debug in debugs:
                ids.append(make_case_id(str(sample), iface, str(mode), str(debug)))
    return ids


@dataclass
class AggregateResult:
    ok: bool
    expected_ids: list[str]
    observed_ids: list[str]
    missing_ids: list[str]
    duplicate_ids: list[str]
    unexpected_ids: list[str]
    errors: list[str] = field(default_factory=list)
    pass_ids: list[str] = field(default_factory=list)
    miscounted_as_pass: list[str] = field(default_factory=list)
    missing_evidence: list[str] = field(default_factory=list)
    observations: list[Observation] = field(default_factory=list)
    metrics: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        return {
            "ok": self.ok,
            "expected_ids": list(self.expected_ids),
            "observed_ids": list(self.observed_ids),
            "missing_ids": list(self.missing_ids),
            "duplicate_ids": list(self.duplicate_ids),
            "unexpected_ids": list(self.unexpected_ids),
            "errors": list(self.errors),
            "pass_ids": list(self.pass_ids),
            "miscounted_as_pass": list(self.miscounted_as_pass),
            "missing_evidence": list(self.missing_evidence),
            "metrics": dict(self.metrics),
            "coverage": (self.metrics.get("T01-M01") or {}).get("observed", {}).get("coverage"),
        }


def _metric_block(verdict: str, **observed: Any) -> dict[str, Any]:
    return {"verdict": verdict, "observed": observed}


def aggregate(
    manifest: Mapping[str, Any] | Sequence[str],
    observations: Any,
    *,
    strict: bool = True,
) -> AggregateResult:
    errors: list[str] = []
    expected = expand_manifest(manifest)
    parsed: list[Observation] = []
    missing_evidence: list[str] = []
    rows: list[Any] = []

    if observations in (None, "", [], {}):
        errors.append("EMPTY_RESULTS: no observations")
    else:
        try:
            rows = list(observations) if isinstance(observations, list) else extract_observation_rows(observations)
        except CorruptJSONError as exc:
            errors.append(str(exc))
            rows = []
        except ObservationError as exc:
            errors.append(str(exc))
            rows = []

    for index, item in enumerate(rows):
        try:
            parsed.append(parse_observation(item))
        except CorruptJSONError as exc:
            errors.append(str(exc))
        except ObservationError as exc:
            msg = str(exc)
            errors.append(msg)
            if "MISSING_MODE" in msg:
                errors.append(f"MISSING_MODE: index={index}")
            if "missing evidence" in msg:
                missing_evidence.append(f"index={index}")
            if "EMPTY_OBSERVATION" in msg:
                errors.append(f"EMPTY_OBSERVATION: index={index}")

    observed_ids = [obs.case_id for obs in parsed]
    counts = Counter(observed_ids)
    duplicate_ids = sorted([cid for cid, n in counts.items() if n > 1])
    unique_observed = set(counts)
    expected_set = set(expected)
    missing_ids = sorted(expected_set - unique_observed)
    unexpected_ids = sorted(unique_observed - expected_set)

    for cid in missing_ids:
        errors.append(f"MISSING ID: {cid}")
    for cid in duplicate_ids:
        errors.append(f"DUPLICATE ID: {cid}")
    if not missing_ids and not duplicate_ids and unexpected_ids and len(observed_ids) == len(expected):
        errors.append(
            "COUNT_EQUAL_BUT_ID_SWAPPED: equal counts hide missing "
            + ", ".join(f"MISSING ID: {cid}" for cid in sorted(expected_set - unique_observed) or missing_ids)
            + ("; unexpected " + ", ".join(unexpected_ids) if unexpected_ids else "")
        )
        # Name each expected ID that is absent even if the branch above already listed them.
        for cid in sorted(expected_set - unique_observed):
            if f"MISSING ID: {cid}" not in errors:
                errors.append(f"MISSING ID: {cid}")
    elif unexpected_ids:
        for cid in unexpected_ids:
            errors.append(f"UNEXPECTED ID: {cid}")

    if len(parsed) == 0 and expected:
        errors.append("EMPTY_RESULTS: observation list is empty")

    pass_ids: list[str] = []
    miscounted: list[str] = []
    for obs in parsed:
        naive_pass = obs.status == "pass" or obs.xfail or obs.skip or obs.status in {"complete", "xfail"}
        real_pass = is_semantic_pass(obs)
        if real_pass:
            pass_ids.append(obs.case_id)
        if obs.xfail and (obs.status == "pass" or naive_pass):
            errors.append(f"XFAIL_COUNTED_AS_PASS: {obs.case_id}")
            miscounted.append(obs.case_id)
        if obs.skip and obs.status == "pass":
            errors.append(f"SKIP_COUNTED_AS_PASS: {obs.case_id}")
            miscounted.append(obs.case_id)
        if obs.status in NEVER_PASS_STATUSES and obs.status == "pass":
            miscounted.append(obs.case_id)
        if obs.status in NEVER_PASS_STATUSES and naive_pass and not real_pass:
            if obs.case_id not in miscounted:
                miscounted.append(obs.case_id)
            errors.append(f"NON_PASS_COUNTED_AS_PASS: {obs.case_id} status={obs.status}")
        if obs.status in {"fail", "unsupported", "infra_error"} and real_pass:
            miscounted.append(obs.case_id)
            errors.append(f"SEMANTIC_FAIL_COUNTED_AS_PASS: {obs.case_id}")

    coverage = (len(expected_set & unique_observed) / len(expected)) if expected else 0.0
    m01_ok = coverage == 1.0 and not duplicate_ids and not missing_ids and not unexpected_ids
    m02_ok = not missing_evidence
    m03_ok = not miscounted

    metrics = {
        "T01-M01": _metric_block(
            "pass" if m01_ok else "fail",
            coverage=coverage,
            denominator=len(expected),
            matched=len(expected_set & unique_observed),
            duplicates=len(duplicate_ids),
            missing=len(missing_ids),
        ),
        "T01-M02": _metric_block(
            "pass" if m02_ok else "fail",
            missing_evidence=len(missing_evidence),
            total=len(parsed),
            required=["input", "toolchain", "mode", "harness", "revision"],
        ),
        "T01-M03": _metric_block(
            "pass" if m03_ok else "fail",
            miscounted_as_pass=len(miscounted),
            pass_ids=list(pass_ids),
        ),
    }

    ok = not errors and m01_ok and m02_ok and m03_ok
    result = AggregateResult(
        ok=ok,
        expected_ids=list(expected),
        observed_ids=observed_ids,
        missing_ids=missing_ids,
        duplicate_ids=duplicate_ids,
        unexpected_ids=unexpected_ids,
        errors=errors,
        pass_ids=pass_ids,
        miscounted_as_pass=miscounted,
        missing_evidence=missing_evidence,
        observations=parsed,
        metrics=metrics,
    )
    if strict and not ok:
        raise InventoryError(
            "; ".join(errors) if errors else "inventory aggregation failed",
            missing_ids=missing_ids,
            duplicate_ids=duplicate_ids,
            unexpected_ids=unexpected_ids,
            details=result.to_dict(),
        )
    return result
