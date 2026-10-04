"""Deterministic duration-aware shards with exact coverage of items and repetitions."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Iterable


class ShardError(Exception):
    """Inventory is not conserved, names collide, or a shard is empty."""


@dataclass(frozen=True)
class TestItem:
    name: str
    duration_ns: int
    repetitions: int = 1
    kind: str = "test"

    def expanded_ids(self) -> list[str]:
        if self.repetitions < 1:
            raise ShardError(f"{self.name}: repetitions must be >= 1")
        if self.repetitions == 1:
            return [self.name]
        return [f"{self.name}#rep{i}" for i in range(self.repetitions)]


@dataclass
class Shard:
    index: int
    items: list[str] = field(default_factory=list)
    duration_ns: int = 0

    def to_dict(self) -> dict[str, Any]:
        return {"index": self.index, "items": list(self.items), "duration_ns": self.duration_ns}


def expand_manifest(items: Iterable[TestItem]) -> list[str]:
    names = [it.name for it in items]
    if len(names) != len(set(names)):
        dup = sorted({n for n in names if names.count(n) > 1})
        raise ShardError(f"duplicate input names: {dup}")
    expanded: list[str] = []
    for item in items:
        expanded.extend(item.expanded_ids())
    if len(expanded) != len(set(expanded)):
        raise ShardError("expanded inventory has duplicate ids")
    return expanded


def shard_items(
    items: Iterable[TestItem],
    n_shards: int,
    *,
    tie_break: str = "name",
) -> list[Shard]:
    material = list(items)
    if n_shards < 1:
        raise ShardError("n_shards must be >= 1")
    expanded = expand_manifest(material)
    if not expanded:
        raise ShardError("empty inventory")
    # Map expanded id -> duration (each repetition inherits parent duration).
    duration = {}
    for item in material:
        for eid in item.expanded_ids():
            duration[eid] = item.duration_ns
    # Longest-processing-time first; tie-break by name for determinism.
    order = sorted(expanded, key=lambda n: (-duration[n], n if tie_break == "name" else n))
    shards = [Shard(index=i) for i in range(n_shards)]
    for name in order:
        lightest = min(shards, key=lambda s: (s.duration_ns, s.index, s.items[0] if s.items else ""))
        lightest.items.append(name)
        lightest.duration_ns += duration[name]
    for shard in shards:
        if not shard.items:
            raise ShardError(f"empty shard {shard.index}")
        shard.items.sort()
    # Conservation: every expanded id exactly once.
    seen: list[str] = []
    for shard in shards:
        seen.extend(shard.items)
    if sorted(seen) != sorted(expanded):
        missing = sorted(set(expanded) - set(seen))
        extra = sorted(set(seen) - set(expanded))
        raise ShardError(f"inventory not conserved missing={missing} extra={extra}")
    if len(seen) != len(set(seen)):
        raise ShardError("duplicate assignment across shards")
    return shards


def round_robin(items: Iterable[TestItem], n_shards: int) -> list[Shard]:
    material = list(items)
    expanded = expand_manifest(material)
    duration = {}
    for item in material:
        for eid in item.expanded_ids():
            duration[eid] = item.duration_ns
    shards = [Shard(index=i) for i in range(n_shards)]
    for i, name in enumerate(sorted(expanded)):
        shard = shards[i % n_shards]
        shard.items.append(name)
        shard.duration_ns += duration[name]
    for shard in shards:
        if not shard.items:
            raise ShardError(f"empty shard {shard.index}")
        shard.items.sort()
    return shards
