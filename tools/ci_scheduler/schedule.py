"""Compare round-robin vs duration packing using measured durations."""

from __future__ import annotations

from typing import Any, Iterable

from .shard import TestItem, round_robin, shard_items


def _max_duration(shards) -> int:
    return max(s.duration_ns for s in shards)


def _total_duration(shards) -> int:
    # Wall-clock if shards run in parallel is max; machine-time is sum.
    return sum(s.duration_ns for s in shards)


def compare_schedules(items: Iterable[TestItem], n_shards: int) -> dict[str, Any]:
    material = list(items)
    packed = shard_items(material, n_shards)
    rr = round_robin(material, n_shards)
    packed_max = _max_duration(packed)
    rr_max = _max_duration(rr)
    return {
        "n_shards": n_shards,
        "n_items": len(material),
        "n_expanded": sum(i.repetitions for i in material),
        "packed": {
            "max_shard_ns": packed_max,
            "total_ns": _total_duration(packed),
            "shards": [s.to_dict() for s in packed],
        },
        "round_robin": {
            "max_shard_ns": rr_max,
            "total_ns": _total_duration(rr),
            "shards": [s.to_dict() for s in rr],
        },
        "max_shard_improvement_ns": rr_max - packed_max,
        "improvement_claimed_percent": None,  # uncalibrated; do not invent a CI time promise
        "measured": True,
    }
