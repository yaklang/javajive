"""Trusted cache keys. PR-supplied baseline artifacts cannot reuse a trusted key."""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from typing import Any


@dataclass(frozen=True)
class CacheDecision:
    trusted: bool
    comparable: bool
    reason: str
    expected_key: str
    provided_key: str

    def to_dict(self) -> dict[str, Any]:
        return {
            "trusted": self.trusted,
            "comparable": self.comparable,
            "reason": self.reason,
            "expected_key": self.expected_key,
            "provided_key": self.provided_key,
            "status": "ok" if self.trusted and self.comparable else "incomparable",
        }


def cache_key(
    *,
    revision: str,
    harness_digest: str,
    compiler: str,
    flags: str,
    deps_digest: str,
    mode: str,
    policy_digest: str,
) -> str:
    payload = {
        "revision": revision,
        "harness_digest": harness_digest,
        "compiler": compiler,
        "flags": flags,
        "deps_digest": deps_digest,
        "mode": mode,
        "policy_digest": policy_digest,
    }
    blob = json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(blob).hexdigest()


def accept_baseline_artifact(
    *,
    expected_key: str,
    artifact_key: str,
    artifact_source: str,
    trusted_revisions: set[str],
    artifact_revision: str,
    harness_changed: bool,
) -> CacheDecision:
    if artifact_source == "pull_request":
        return CacheDecision(
            trusted=False,
            comparable=False,
            reason="pr_supplied_baseline_untrusted",
            expected_key=expected_key,
            provided_key=artifact_key,
        )
    if artifact_revision not in trusted_revisions:
        return CacheDecision(
            False, False, "revision_not_in_trust_domain", expected_key, artifact_key
        )
    if harness_changed and artifact_key == expected_key:
        return CacheDecision(
            False, False, "harness_changed_but_key_reused", expected_key, artifact_key
        )
    if artifact_key != expected_key:
        return CacheDecision(False, False, "cache_key_mismatch", expected_key, artifact_key)
    return CacheDecision(True, True, "trusted_baseline", expected_key, artifact_key)
