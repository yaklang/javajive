"""Holdout split by library/source/compiler family; near-duplicate leakage is an error."""

from __future__ import annotations

import hashlib
import json
from collections import defaultdict
from dataclasses import dataclass, field
from typing import Any, Iterable


class HoldoutLeakError(Exception):
    pass


@dataclass(frozen=True)
class CorpusItem:
    item_id: str
    group: str  # library+major lineage, e.g. "commons-foo"
    version: str
    compiler_family: str
    content_sha256: str
    normalized_sha256: str
    origin: str

    def freeze(self) -> dict[str, Any]:
        return {
            "item_id": self.item_id,
            "group": self.group,
            "version": self.version,
            "compiler_family": self.compiler_family,
            "content_sha256": self.content_sha256,
            "normalized_sha256": self.normalized_sha256,
            "origin": self.origin,
        }


@dataclass
class HoldoutSplit:
    train: list[CorpusItem]
    holdout: list[CorpusItem]
    duplicate_rate: float
    frozen_hashes: dict[str, str] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        return {
            "train_ids": [x.item_id for x in self.train],
            "holdout_ids": [x.item_id for x in self.holdout],
            "duplicate_rate": self.duplicate_rate,
            "frozen_hashes": self.frozen_hashes,
            "train_groups": sorted({x.group for x in self.train}),
            "holdout_groups": sorted({x.group for x in self.holdout}),
        }


def normalize_bytes(data: bytes) -> bytes:
    """Strip trivial debug-ish tail variation: keep magic+version+body without SourceFile UTF if present as whole-file.

    Near-dup is defined as identical normalized payload. Callers that want source-level
    near-dup should pass already-normalized bytes (e.g. stripped LineNumberTable).
    """
    # Drop trailing zeros and CR variance; keep binary otherwise.
    return data.replace(b"\r\n", b"\n").rstrip(b"\x00")


def content_hash(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def normalized_hash(data: bytes) -> str:
    return hashlib.sha256(normalize_bytes(data)).hexdigest()


def _duplicate_rate(items: list[CorpusItem]) -> float:
    if not items:
        return 0.0
    counts: dict[str, int] = defaultdict(int)
    for item in items:
        counts[item.normalized_sha256] += 1
    dup = sum(n - 1 for n in counts.values() if n > 1)
    return dup / len(items)


def split_holdout(
    items: Iterable[CorpusItem],
    *,
    holdout_groups: set[str],
    freeze: bool = True,
) -> HoldoutSplit:
    material = list(items)
    ids = [x.item_id for x in material]
    if len(ids) != len(set(ids)):
        raise HoldoutLeakError("duplicate item_id in corpus")
    train: list[CorpusItem] = []
    holdout: list[CorpusItem] = []
    for item in material:
        (holdout if item.group in holdout_groups else train).append(item)
    train_groups = {x.group for x in train}
    hold_groups = {x.group for x in holdout}
    leaked = train_groups & hold_groups
    if leaked:
        raise HoldoutLeakError(f"group leakage between train/holdout: {sorted(leaked)}")
    train_norm = {x.normalized_sha256 for x in train}
    leaked_dups = [x.item_id for x in holdout if x.normalized_sha256 in train_norm]
    if leaked_dups:
        raise HoldoutLeakError(f"near-duplicate leakage into holdout: {leaked_dups}")
    frozen = {}
    if freeze:
        frozen = {x.item_id: x.content_sha256 for x in material}
    return HoldoutSplit(
        train=train,
        holdout=holdout,
        duplicate_rate=_duplicate_rate(material),
        frozen_hashes=frozen,
    )


def assert_frozen(split: HoldoutSplit, items: Iterable[CorpusItem]) -> None:
    current = {x.item_id: x.content_sha256 for x in items}
    if current != split.frozen_hashes:
        raise HoldoutLeakError("holdout inventory hash changed; refusing to beautify metrics")


def inventory_json(split: HoldoutSplit) -> str:
    return json.dumps(split.to_dict(), indent=2, sort_keys=True) + "\n"
