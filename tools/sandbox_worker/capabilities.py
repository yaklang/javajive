"""Which isolation backends can actually enforce requested hard limits.

Timeout and output caps are not a memory/process sandbox. Darwin seatbelt cannot
set RLIMIT_AS/DATA; Darwin/Linux nproc is user-wide, not a cgroup. Docker/podman
cgroup memory + pids-limit are the verified hard-cap backends on this host.
"""

from __future__ import annotations

from typing import Iterable

from .policy import Limits

# cgroup-backed. Timeout/output-only backends must not claim these.
HARD_MEMORY_BACKENDS = frozenset({"docker", "podman"})
HARD_PIDS_BACKENDS = frozenset({"docker", "podman"})
NETWORK_BACKENDS = frozenset({"docker", "podman", "bwrap", "unshare", "sandbox-exec"})
MOUNT_BACKENDS = frozenset({"docker", "podman", "bwrap", "unshare", "sandbox-exec"})

REASON_CAPABILITY_UNSUPPORTED = "capability_unsupported"

PREFERENCE = ("docker", "podman", "bwrap", "unshare", "sandbox-exec")


def required_hard_caps(limits: Limits) -> frozenset[str]:
    needed: set[str] = set()
    if int(limits.memory_bytes or 0) > 0:
        needed.add("memory")
    if int(limits.pids or 0) > 0:
        needed.add("pids")
    return frozenset(needed)


def backend_hard_caps(name: str | None) -> frozenset[str]:
    if name in HARD_MEMORY_BACKENDS:
        return frozenset({"memory", "pids"})
    return frozenset()


def missing_hard_caps(name: str | None, limits: Limits) -> list[str]:
    return sorted(required_hard_caps(limits) - backend_hard_caps(name))


def can_enforce(name: str | None, limits: Limits) -> bool:
    return not missing_hard_caps(name, limits)


def choose_backend(
    available: Iterable[str],
    limits: Limits,
    *,
    forced: str | None = None,
) -> tuple[str | None, str]:
    """Return (backend, note). Forced backend is kept only if it can enforce."""
    avail = {n for n in available}
    if forced:
        if forced not in avail:
            return None, f"forced backend {forced} unavailable"
        if can_enforce(forced, limits):
            return forced, f"forced {forced}"
        for name in PREFERENCE:
            if name in avail and can_enforce(name, limits):
                return name, f"forced {forced} cannot enforce {missing_hard_caps(forced, limits)}; selected {name}"
        return None, (
            f"forced {forced} cannot enforce {missing_hard_caps(forced, limits)} "
            "and no capable backend is available"
        )
    for name in PREFERENCE:
        if name in avail and can_enforce(name, limits):
            return name, f"selected {name} for hard caps {sorted(required_hard_caps(limits))}"
    return None, "no backend can enforce requested hard resource limits"
