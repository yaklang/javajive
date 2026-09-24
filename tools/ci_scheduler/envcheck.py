"""Missing JDK or zero go-test matches are infra_error, never skip/pass."""

from __future__ import annotations

from dataclasses import dataclass


class EnvError(Exception):
    def __init__(self, status: str, reason: str):
        super().__init__(reason)
        self.status = status
        self.reason = reason


@dataclass(frozen=True)
class EnvVerdict:
    status: str  # pass | infra_error
    reason: str
    job_green: bool


def _parse_jdk(version: str) -> int:
    version = version.strip()
    if version.startswith("1."):
        return int(version.split(".")[1])
    return int(version.split(".")[0].split("_")[0])


def require_jdk(required: int, actual: str | None) -> EnvVerdict:
    if not actual:
        return EnvVerdict("infra_error", "jdk_missing", False)
    try:
        got = _parse_jdk(actual)
    except (TypeError, ValueError):
        return EnvVerdict("infra_error", f"jdk_unparseable:{actual}", False)
    if got < required:
        return EnvVerdict("infra_error", f"jdk{got}_running_case_requires_{required}", False)
    return EnvVerdict("pass", f"jdk{got}", True)


def require_go_matches(match_count: int, pattern: str) -> EnvVerdict:
    if match_count == 0:
        return EnvVerdict("infra_error", f"go_test_zero_match:{pattern}", False)
    if match_count < 0:
        return EnvVerdict("infra_error", "go_test_match_unreadable", False)
    return EnvVerdict("pass", f"matched:{match_count}", True)
