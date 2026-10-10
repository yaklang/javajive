"""CLI for shard planning and workflow inventory."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from .shard import TestItem, shard_items
from .workflow_inventory import required_coverage_snapshot


def main() -> int:
    p = argparse.ArgumentParser(description="T29 CI scheduler")
    sub = p.add_subparsers(dest="cmd", required=True)
    s = sub.add_parser("shard")
    s.add_argument("--n", type=int, default=8)
    s.add_argument("--manifest", type=Path, required=True)
    inv = sub.add_parser("inventory")
    inv.add_argument("--repo", type=Path, required=True)
    args = p.parse_args()
    if args.cmd == "inventory":
        print(json.dumps(required_coverage_snapshot(args.repo), indent=2))
        return 0
    data = json.loads(args.manifest.read_text(encoding="utf-8"))
    items = [TestItem(**row) for row in data]
    shards = shard_items(items, args.n)
    print(json.dumps([sh.to_dict() for sh in shards], indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
