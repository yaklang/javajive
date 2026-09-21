"""Load historical next-stage CLI / JAR observations without rewriting raw fields."""

from __future__ import annotations

import copy
from pathlib import Path
from typing import Any, Mapping

from .case_id import make_case_id
from .constants import (
    API_MODES,
    HISTORICAL_CLI_THIRTEEN_FIELDS,
    MILESTONE_SHA,
    SCHEMA_VERSION,
)
from .errors import MissingEvidenceError, ObservationError
from .observation import (
    Observation,
    json_equal,
    load_json_path,
    parse_observation,
)
from .toolchain import next_stage_harness_digest, normalize_compiler_version, require_git_sha, require_sha256


def package_historical_cli_path() -> Path:
    return Path(__file__).resolve().parent / "evidence" / "historical_cli_seeds.json"


def evidence_defaults_from_cli_report(report: Mapping[str, Any]) -> dict[str, Any]:
    metadata = report.get("metadata") or {}
    jdk = metadata.get("jdk") or report.get("jdk") or {}
    stdout = str(jdk.get("stdout") or "")
    stderr = str(jdk.get("stderr") or "")
    compiler = normalize_compiler_version(stdout + stderr)
    revision = str(report.get("baseline_sha") or MILESTONE_SHA)
    require_git_sha("revision", revision)
    lock = metadata.get("executable_sha256") or report.get("executable_sha256")
    if not lock:
        raise MissingEvidenceError("missing evidence: historical executable/dependency digest")
    harness = report.get("source_report_sha256") or next_stage_harness_digest()
    require_sha256("dependency_lock_digest", str(lock))
    require_sha256("harness_digest", str(harness))
    return {
        "compiler_version": compiler,
        "dependency_lock_digest": str(lock).lower(),
        "harness_digest": str(harness).lower(),
        "revision": revision.lower(),
    }


def historical_cli_fields_preserved(original: Mapping[str, Any], wrapped: Observation) -> list[str]:
    """Return names of 13-check fields that are not JSON-equal to the source record."""
    mismatches: list[str] = []
    raw = wrapped.historical_raw or {}
    for key in HISTORICAL_CLI_THIRTEEN_FIELDS:
        if key not in original:
            mismatches.append(key)
            continue
        if not json_equal(original[key], raw.get(key)):
            mismatches.append(key)
    # Any extra original keys must also survive.
    for key, value in original.items():
        if not json_equal(value, raw.get(key)):
            if key not in mismatches:
                mismatches.append(key)
    return mismatches


def load_historical_cli_report(
    path: Path | str | None = None,
    *,
    synthesize_api_placeholders: bool = True,
) -> dict[str, Any]:
    """Load the 4-seed CLI excerpt and optionally emit API not_run placeholders."""
    path = Path(path) if path is not None else package_historical_cli_path()
    report = load_json_path(path)
    if not isinstance(report, Mapping):
        raise ObservationError("historical CLI report must be a JSON object")
    defaults = evidence_defaults_from_cli_report(report)
    cli_obs: list[Observation] = []
    originals: list[dict[str, Any]] = []
    for row in report.get("observations") or []:
        original = copy.deepcopy(dict(row))
        originals.append(original)
        obs = parse_observation(row, evidence_defaults=defaults, source="historical-cli")
        lost = historical_cli_fields_preserved(original, obs)
        if lost:
            raise ObservationError("historical raw fields mutated: " + ", ".join(lost))
        cli_obs.append(obs)

    api_obs: list[Observation] = []
    if synthesize_api_placeholders:
        seen: set[tuple[str, str]] = set()
        for obs in cli_obs:
            key = (obs.sample, obs.evidence.debug)
            if key in seen:
                continue
            seen.add(key)
            for mode in sorted(API_MODES):
                placeholder = {
                    "schema_version": SCHEMA_VERSION,
                    "sample": obs.sample,
                    "interface": "api",
                    "status": "not_run",
                    "source": "api-not-executed",
                    "notes": (
                        "Precision/compatibility API was not executed in historical "
                        "compatibility-cli evidence; status stays not_run/unknown"
                    ),
                    "evidence": {
                        **obs.evidence.to_dict(),
                        "mode": mode,
                        "output_target": "not_run",
                    },
                    "execution_evidence": None,
                    "historical_raw": None,
                }
                api_obs.append(parse_observation(placeholder))

    return {
        "report": report,
        "defaults": defaults,
        "cli": cli_obs,
        "api_placeholders": api_obs,
        "originals": originals,
        "observations": cli_obs + api_obs,
    }


def make_api_placeholder(
    sample: str,
    mode: str,
    debug: str,
    *,
    evidence: Mapping[str, Any],
    status: str = "not_run",
) -> Observation:
    payload = {
        "sample": sample,
        "interface": "api",
        "status": status,
        "source": "api-not-executed",
        "notes": "API mode not executed",
        "evidence": {**dict(evidence), "mode": mode, "debug": debug, "output_target": "not_run"},
        "execution_evidence": None,
        "historical_raw": None,
    }
    obs = parse_observation(payload)
    expected = make_case_id(sample, "api", mode, debug)
    if obs.case_id != expected:
        raise ObservationError("placeholder case id mismatch")
    return obs
