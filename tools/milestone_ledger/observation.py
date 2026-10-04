"""Observation schema: next-stage CLI fields plus required evidence envelope."""

from __future__ import annotations

import copy
import json
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Mapping

from .case_id import interface_for_mode, make_case_id, parse_case_id
from .constants import (
    DEBUGS,
    HISTORICAL_CLI_THIRTEEN_FIELDS,
    HISTORICAL_JAR_RAW_FIELDS,
    MODES,
    REQUIRED_EVIDENCE_FIELDS,
    RESULT_STATUSES,
    SCHEMA_VERSION,
    SOURCES,
    SUCCESS_STATUSES,
)
from .errors import CorruptJSONError, MissingEvidenceError, ObservationError
from .toolchain import normalize_compiler_version, require_git_sha, require_sha256


def canonical_json(value: Any) -> str:
    return json.dumps(value, sort_keys=True, ensure_ascii=False, separators=(",", ":"))


def json_equal(left: Any, right: Any) -> bool:
    return canonical_json(left) == canonical_json(right)


def load_json_bytes(raw: str | bytes, *, origin: str = "input") -> Any:
    try:
        if isinstance(raw, bytes):
            text = raw.decode("utf-8")
        else:
            text = raw
        return json.loads(text)
    except (UnicodeDecodeError, json.JSONDecodeError, TypeError, ValueError) as exc:
        raise CorruptJSONError(f"CORRUPT_JSON: {origin}: {exc}") from exc


def load_json_path(path: Path | str) -> Any:
    path = Path(path)
    try:
        text = path.read_text(encoding="utf-8")
    except OSError as exc:
        raise CorruptJSONError(f"CORRUPT_JSON: cannot read {path}: {exc}") from exc
    return load_json_bytes(text, origin=str(path))


def looks_like_historical_cli(data: Mapping[str, Any]) -> bool:
    if not isinstance(data, Mapping):
        return False
    if "evidence" in data or "case_id" in data:
        return False
    return "case" in data and "original_compile" in data and "mode" in data


def looks_like_historical_jar(data: Mapping[str, Any]) -> bool:
    if not isinstance(data, Mapping):
        return False
    if "evidence" in data or "case_id" in data:
        return False
    return "Completed" in data and "InputSHA256" in data and "Compiler" in data


@dataclass(frozen=True)
class Evidence:
    input_hash: str
    compiler_version: str
    release: int
    debug: str
    mode: str
    output_target: str
    dependency_lock_digest: str
    harness_digest: str
    revision: str

    def to_dict(self) -> dict[str, Any]:
        return {
            "input_hash": self.input_hash,
            "compiler_version": self.compiler_version,
            "release": self.release,
            "debug": self.debug,
            "mode": self.mode,
            "output_target": self.output_target,
            "dependency_lock_digest": self.dependency_lock_digest,
            "harness_digest": self.harness_digest,
            "revision": self.revision,
        }

    def axis(self, name: str) -> str:
        value = getattr(self, name)
        return str(value)


def validate_evidence_mapping(raw: Mapping[str, Any]) -> Evidence:
    if not isinstance(raw, Mapping):
        raise MissingEvidenceError("missing evidence: evidence object is required")
    missing = [key for key in REQUIRED_EVIDENCE_FIELDS if key not in raw or raw[key] in (None, "")]
    if missing:
        raise MissingEvidenceError(
            "missing evidence: " + ", ".join(missing) + " (input/toolchain/mode/harness/revision required)"
        )
    mode = str(raw["mode"]).strip()
    if mode not in MODES:
        raise ObservationError(f"MISSING_MODE: {mode!r}")
    debug = str(raw["debug"]).strip()
    if debug not in DEBUGS:
        raise ObservationError(f"invalid debug {debug!r}")
    compiler = normalize_compiler_version(str(raw["compiler_version"]))
    release_raw = raw["release"]
    try:
        release = int(release_raw)
    except (TypeError, ValueError) as exc:
        raise ObservationError(f"invalid release {release_raw!r}") from exc
    output_target = str(raw["output_target"]).strip()
    if not output_target:
        raise MissingEvidenceError("missing evidence: output_target")
    return Evidence(
        input_hash=require_sha256("input_hash", str(raw["input_hash"])),
        compiler_version=compiler,
        release=release,
        debug=debug,
        mode=mode,
        output_target=output_target,
        dependency_lock_digest=require_sha256("dependency_lock_digest", str(raw["dependency_lock_digest"])),
        harness_digest=require_sha256("harness_digest", str(raw["harness_digest"])),
        revision=require_git_sha("revision", str(raw["revision"])),
    )


@dataclass(frozen=True)
class Observation:
    case_id: str
    sample: str
    interface: str
    status: str
    evidence: Evidence
    source: str = "synthetic"
    historical_raw: dict[str, Any] | None = None
    execution_evidence: dict[str, Any] | None = None
    xfail: bool = False
    skip: bool = False
    empty: bool = False
    notes: str = ""
    extras: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        payload: dict[str, Any] = {
            "schema_version": SCHEMA_VERSION,
            "case_id": self.case_id,
            "sample": self.sample,
            "interface": self.interface,
            "status": self.status,
            "source": self.source,
            "evidence": self.evidence.to_dict(),
            "historical_raw": copy.deepcopy(self.historical_raw),
            "execution_evidence": copy.deepcopy(self.execution_evidence),
            "xfail": self.xfail,
            "skip": self.skip,
            "empty": self.empty,
            "notes": self.notes,
        }
        if self.extras:
            payload["extras"] = copy.deepcopy(self.extras)
        return payload

    def historical_field(self, name: str) -> Any:
        if not self.historical_raw:
            raise ObservationError(f"no historical_raw for field {name}")
        return copy.deepcopy(self.historical_raw.get(name))

    def historical_thirteen(self) -> dict[str, Any]:
        if not self.historical_raw:
            raise ObservationError("no historical_raw to extract 13-check fields")
        return {key: copy.deepcopy(self.historical_raw.get(key)) for key in HISTORICAL_CLI_THIRTEEN_FIELDS}


def is_semantic_pass(obs: Observation) -> bool:
    if obs.xfail or obs.skip or obs.empty:
        return False
    if obs.status not in SUCCESS_STATUSES:
        return False
    raw = obs.historical_raw or {}
    if raw.get("pass") is False:
        return False
    if obs.source == "api-not-executed":
        return False
    return True


def _truthy_flag(value: Any) -> bool:
    return value is True or value in ("true", "True", 1)


def _coerce_mapping(data: Any, *, origin: str = "observation") -> dict[str, Any]:
    if isinstance(data, Observation):
        return data.to_dict()
    if isinstance(data, Path):
        loaded = load_json_path(data)
        if not isinstance(loaded, Mapping):
            raise ObservationError(f"{origin} JSON must be an object")
        return dict(loaded)
    if isinstance(data, (bytes, bytearray)):
        loaded = load_json_bytes(bytes(data), origin=origin)
        if not isinstance(loaded, Mapping):
            raise ObservationError(f"{origin} JSON must be an object")
        return dict(loaded)
    if isinstance(data, str):
        text = data.strip()
        if text.startswith("{") or text.startswith("["):
            loaded = load_json_bytes(text, origin=origin)
            if not isinstance(loaded, Mapping):
                raise ObservationError(f"{origin} JSON must be an object")
            return dict(loaded)
        path = Path(text)
        if path.is_file():
            loaded = load_json_path(path)
            if not isinstance(loaded, Mapping):
                raise ObservationError(f"{origin} JSON must be an object")
            return dict(loaded)
        raise CorruptJSONError(f"CORRUPT_JSON: not an observation object or JSON file: {text[:80]!r}")
    if isinstance(data, Mapping):
        return dict(data)
    raise ObservationError(f"cannot parse observation from {type(data).__name__}")


def _output_target_from_historical(raw: Mapping[str, Any], mode: str) -> str:
    rebuilt = raw.get("rebuilt_compile")
    if isinstance(rebuilt, Mapping):
        argv = rebuilt.get("argv") or []
        if isinstance(argv, list) and "-d" in argv:
            idx = argv.index("-d")
            if idx + 1 < len(argv):
                return str(argv[idx + 1])
    return str(mode)


def wrap_historical_cli_dict(
    raw: Mapping[str, Any],
    *,
    evidence_defaults: Mapping[str, Any] | None = None,
    source: str = "historical-cli",
) -> dict[str, Any]:
    defaults = dict(evidence_defaults or {})
    missing_hist = [key for key in ("case", "mode", "debug") if key not in raw or raw[key] in (None, "")]
    if missing_hist:
        if "mode" in missing_hist:
            raise ObservationError("MISSING_MODE: historical CLI observation lacks mode")
        raise ObservationError("historical CLI observation missing " + ", ".join(missing_hist))
    mode = str(raw["mode"])
    debug = str(raw["debug"])
    sample = str(raw["case"])
    interface = interface_for_mode(mode)
    input_hash = raw.get("input_sha256") or defaults.get("input_hash")
    compiler = defaults.get("compiler_version")
    if not compiler:
        raise MissingEvidenceError("missing evidence: compiler_version (real javac -version)")
    lock = defaults.get("dependency_lock_digest")
    harness = defaults.get("harness_digest")
    revision = defaults.get("revision")
    release = raw.get("release", defaults.get("release"))
    output_target = defaults.get("output_target") or _output_target_from_historical(raw, mode)
    status = defaults.get("status")
    if status is None:
        if raw.get("pass") is True:
            status = "pass"
        elif raw.get("failure") == "original_fixture_failure":
            status = "invalid_input"
        else:
            status = "fail"
    notes = defaults.get("notes", "")
    execution = None
    if raw.get("decompile") is not None or raw.get("decompile_result") is not None:
        execution = {
            "executed": True,
            "kind": "historical-cli-13-check",
            "has_decompile": raw.get("decompile") is not None,
        }
    return {
        "schema_version": SCHEMA_VERSION,
        "sample": sample,
        "interface": interface,
        "status": status,
        "source": source,
        "notes": notes,
        "historical_raw": copy.deepcopy(dict(raw)),
        "execution_evidence": execution,
        "evidence": {
            "input_hash": input_hash,
            "compiler_version": compiler,
            "release": release,
            "debug": debug,
            "mode": mode,
            "output_target": output_target,
            "dependency_lock_digest": lock,
            "harness_digest": harness,
            "revision": revision,
        },
    }


def wrap_historical_jar_dict(
    raw: Mapping[str, Any],
    *,
    evidence_defaults: Mapping[str, Any] | None = None,
    source: str = "historical-jar",
) -> dict[str, Any]:
    defaults = dict(evidence_defaults or {})
    sample = str(raw.get("Jar") or defaults.get("sample") or "")
    if not sample:
        raise ObservationError("historical JAR observation missing Jar")
    mode = str(defaults.get("mode") or "compatibility-cli")
    debug = str(defaults.get("debug") or "nodebug")
    compiler = raw.get("Compiler") or defaults.get("compiler_version")
    if compiler and not str(compiler).startswith("javac "):
        compiler = f"javac {compiler}" if str(compiler)[0].isdigit() else compiler
    revision = raw.get("Revision") or defaults.get("revision")
    input_hash = raw.get("InputSHA256") or defaults.get("input_hash")
    # Completed is not acceptance; CompileSucceeded is not a semantic pass.
    if not raw.get("Completed"):
        status = "infra_error"
        notes = "incomplete historical JAR observation; Completed is not a pass"
    else:
        status = "observe"
        notes = "historical JAR observation complete is not round-trip acceptance"
    return {
        "schema_version": SCHEMA_VERSION,
        "sample": sample,
        "interface": interface_for_mode(mode),
        "status": status,
        "source": source,
        "notes": notes,
        "historical_raw": copy.deepcopy(dict(raw)),
        "execution_evidence": {"executed": True, "kind": "historical-jar-audit", "completed": bool(raw.get("Completed"))},
        "evidence": {
            "input_hash": input_hash,
            "compiler_version": compiler,
            "release": defaults.get("release", 8),
            "debug": debug,
            "mode": mode,
            "output_target": defaults.get("output_target") or sample,
            "dependency_lock_digest": defaults.get("dependency_lock_digest"),
            "harness_digest": defaults.get("harness_digest"),
            "revision": revision,
        },
        "extras": {key: copy.deepcopy(raw.get(key)) for key in HISTORICAL_JAR_RAW_FIELDS if key in raw},
    }


_EXECUTION_KINDS = frozenset(
    {"synthetic", "api-live", "historical-cli-13-check", "historical-jar-audit"}
)


def _has_execution_evidence(payload: Mapping[str, Any], evidence: Evidence) -> bool:
    historical_raw = payload.get("historical_raw")
    execution = payload.get("execution_evidence")
    if isinstance(historical_raw, Mapping) and historical_raw.get("mode") == evidence.mode:
        if historical_raw.get("decompile") is not None or historical_raw.get("decompile_result") is not None:
            return True
    if isinstance(execution, Mapping) and execution.get("executed"):
        if execution.get("kind") in _EXECUTION_KINDS:
            return True
    return False


def _validate_pass_claim(payload: Mapping[str, Any], evidence: Evidence) -> None:
    status = payload["status"]
    source = str(payload.get("source") or "synthetic")
    historical_raw = payload.get("historical_raw")
    if status != "pass":
        return
    if _truthy_flag(payload.get("xfail")) or _truthy_flag(payload.get("skip")) or _truthy_flag(payload.get("empty")):
        raise ObservationError("xfail/skip/empty cannot be recorded as pass")
    if source == "api-not-executed":
        raise ObservationError(
            "cannot invent API pass without execution evidence (Precision/compatibility not executed)"
        )
    if isinstance(historical_raw, Mapping) and historical_raw.get("pass") is False:
        raise ObservationError("historical fail is never a pass")
    if isinstance(historical_raw, Mapping):
        hist_mode = historical_raw.get("mode")
        if hist_mode not in (None, "", evidence.mode):
            raise ObservationError(
                f"cannot invent {evidence.mode} pass from {hist_mode} historical record"
            )
    if not _has_execution_evidence(payload, evidence):
        raise ObservationError(f"cannot invent {evidence.mode} pass without execution evidence")


def _validate_historical_raw(raw: Mapping[str, Any] | None, evidence: Evidence) -> dict[str, Any] | None:
    if raw is None:
        return None
    if not isinstance(raw, Mapping):
        raise ObservationError("historical_raw must be a JSON object")
    preserved = copy.deepcopy(dict(raw))
    if "mode" in preserved and preserved["mode"] not in (None, "") and preserved["mode"] != evidence.mode:
        # Allowed: envelope mode is the ledger mode; raw mode stays byte-for-byte for CLI records
        # attached only to the matching CLI observation.
        pass
    return preserved


def parse_observation(
    data: Observation | Mapping[str, Any] | str | Path | bytes,
    *,
    evidence_defaults: Mapping[str, Any] | None = None,
    source: str | None = None,
    _wrapped: bool = False,
) -> Observation:
    """Parse one observation. Missing evidence/mode is an error, not a default."""
    if isinstance(data, Observation) and _wrapped:
        return data
    payload = _coerce_mapping(data)
    if not payload:
        raise ObservationError("EMPTY_OBSERVATION: empty object")
    if payload.get("empty") is True:
        raise ObservationError("EMPTY_OBSERVATION")
    if not _wrapped and looks_like_historical_cli(payload):
        wrapped = wrap_historical_cli_dict(
            payload, evidence_defaults=evidence_defaults, source=source or "historical-cli"
        )
        return parse_observation(wrapped, _wrapped=True)
    if not _wrapped and looks_like_historical_jar(payload):
        wrapped = wrap_historical_jar_dict(
            payload, evidence_defaults=evidence_defaults, source=source or "historical-jar"
        )
        return parse_observation(wrapped, _wrapped=True)

    if "mode" not in (payload.get("evidence") or {}) and "mode" not in payload:
        raise ObservationError("MISSING_MODE: observation lacks mode")

    evidence_raw = payload.get("evidence")
    if evidence_raw is None:
        # Allow flattened evidence keys at top level for synthetic tests.
        flat = {key: payload.get(key) for key in REQUIRED_EVIDENCE_FIELDS if key in payload}
        if evidence_defaults:
            for key, value in evidence_defaults.items():
                flat.setdefault(key, value)
        for key in ("debug", "mode", "release"):
            if key in payload:
                flat.setdefault(key, payload[key])
        evidence_raw = flat
    elif evidence_defaults:
        merged = dict(evidence_defaults)
        merged.update(dict(evidence_raw))
        evidence_raw = merged

    if isinstance(evidence_raw, Mapping) and "mode" not in evidence_raw:
        raise ObservationError("MISSING_MODE: evidence lacks mode")

    evidence = validate_evidence_mapping(evidence_raw if isinstance(evidence_raw, Mapping) else {})

    sample = str(payload.get("sample") or payload.get("case") or "").strip()
    interface = str(payload.get("interface") or interface_for_mode(evidence.mode)).strip()
    if not sample:
        if payload.get("case_id"):
            sample, interface, _mode, _debug = parse_case_id(str(payload["case_id"]))
        else:
            raise ObservationError("missing sample/case")
    case_id = str(payload.get("case_id") or "").strip() or make_case_id(
        sample, interface, evidence.mode, evidence.debug
    )
    expected_id = make_case_id(sample, interface, evidence.mode, evidence.debug)
    if case_id != expected_id:
        raise ObservationError(f"case_id {case_id!r} does not match {expected_id!r}")

    status = str(payload.get("status") or "").strip()
    if not status:
        raise ObservationError("MISSING_STATUS: observation lacks status")
    if status == "complete":
        raise ObservationError("decompiler complete is not a ledger pass; refused as status")
    if status not in RESULT_STATUSES:
        raise ObservationError(f"unknown status {status!r}")

    src = str(source or payload.get("source") or "synthetic").strip()
    if src not in SOURCES:
        raise ObservationError(f"unknown observation source {src!r}")

    payload_for_pass = dict(payload)
    payload_for_pass["status"] = status
    payload_for_pass["source"] = src
    _validate_pass_claim(payload_for_pass, evidence)

    historical_raw = _validate_historical_raw(payload.get("historical_raw"), evidence)
    execution = payload.get("execution_evidence")
    if execution is not None and not isinstance(execution, Mapping):
        raise ObservationError("execution_evidence must be an object")
    execution_copy = copy.deepcopy(dict(execution)) if isinstance(execution, Mapping) else None

    obs = Observation(
        case_id=case_id,
        sample=sample,
        interface=interface,
        status=status,
        evidence=evidence,
        source=src,
        historical_raw=historical_raw,
        execution_evidence=execution_copy,
        xfail=_truthy_flag(payload.get("xfail")),
        skip=_truthy_flag(payload.get("skip")),
        empty=_truthy_flag(payload.get("empty")),
        notes=str(payload.get("notes") or ""),
        extras=copy.deepcopy(dict(payload.get("extras") or {})),
    )
    if is_semantic_pass(obs) is False and obs.status == "pass":
        raise ObservationError("pass claim failed semantic pass checks")
    return obs


def parse_observation_list(
    data: Any,
    *,
    evidence_defaults: Mapping[str, Any] | None = None,
) -> list[Observation]:
    rows = extract_observation_rows(data)
    return [parse_observation(item, evidence_defaults=evidence_defaults) for item in rows]


def extract_observation_rows(data: Any) -> list[Any]:
    if isinstance(data, Observation):
        return [data]
    if isinstance(data, Path) or (isinstance(data, str) and Path(data).is_file()):
        data = load_json_path(data)
    elif isinstance(data, str):
        stripped = data.strip()
        if stripped.startswith("{") or stripped.startswith("["):
            data = load_json_bytes(stripped, origin="observations")
        else:
            raise CorruptJSONError("CORRUPT_JSON: observations value is not JSON")
    if isinstance(data, list):
        return list(data)
    if isinstance(data, Mapping):
        if "observations" in data:
            rows = list(data.get("observations") or [])
            extra = list(data.get("parser_observations") or [])
            return rows + extra
        if "case_id" in data or "evidence" in data or looks_like_historical_cli(data) or looks_like_historical_jar(data):
            return [data]
    raise ObservationError("observations must be a list or report object")
