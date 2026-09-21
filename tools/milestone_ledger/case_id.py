"""Stable case IDs: {sample}::{interface}::{mode}::{debug}."""

from __future__ import annotations

from .constants import API_MODES, CLI_MODES, DEBUGS, INTERFACES, MODES
from .errors import ObservationError


def interface_for_mode(mode: str) -> str:
    if mode in CLI_MODES:
        return "cli"
    if mode in API_MODES:
        return "api"
    raise ObservationError(f"MISSING_MODE: unknown mode {mode!r}")


def make_case_id(sample: str, interface: str, mode: str, debug: str) -> str:
    sample = str(sample or "").strip()
    interface = str(interface or "").strip()
    mode = str(mode or "").strip()
    debug = str(debug or "").strip()
    if not sample:
        raise ObservationError("missing sample for case id")
    if interface not in INTERFACES:
        raise ObservationError(f"invalid interface {interface!r}")
    if mode not in MODES:
        raise ObservationError(f"MISSING_MODE: {mode!r}")
    if debug not in DEBUGS:
        raise ObservationError(f"invalid debug {debug!r}")
    expected_interface = interface_for_mode(mode)
    if interface != expected_interface:
        raise ObservationError(
            f"interface {interface!r} does not match mode {mode!r} (expected {expected_interface})"
        )
    return f"{sample}::{interface}::{mode}::{debug}"


def parse_case_id(case_id: str) -> tuple[str, str, str, str]:
    parts = str(case_id or "").split("::")
    if len(parts) != 4:
        raise ObservationError(f"invalid case id {case_id!r}; expected sample::interface::mode::debug")
    sample, interface, mode, debug = parts
    rebuilt = make_case_id(sample, interface, mode, debug)
    if rebuilt != case_id:
        raise ObservationError(f"case id failed canonicalization: {case_id!r}")
    return sample, interface, mode, debug
