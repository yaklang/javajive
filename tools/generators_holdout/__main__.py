"""CLI: generate / morph / holdout / compilers."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from .generate import SAMPLES_PER_FAMILY, generate_all, generate_family, write_sources
from .holdout import inventory_json, split_holdout
from .identity import discover_compilers


def main() -> int:
    p = argparse.ArgumentParser(description="T28 generators / holdout")
    sub = p.add_subparsers(dest="cmd", required=True)
    g = sub.add_parser("generate")
    g.add_argument("--family")
    g.add_argument("--out", type=Path, required=True)
    g.add_argument("--count", type=int, default=SAMPLES_PER_FAMILY)
    sub.add_parser("compilers")
    args = p.parse_args()
    if args.cmd == "compilers":
        found = discover_compilers()
        print(json.dumps({k: (v.to_dict() if v else None) for k, v in found.items()}, indent=2))
        return 0
    if args.cmd == "generate":
        samples = generate_family(args.family) if args.family else generate_all(count=args.count)
        write_sources(samples, args.out)
        print(f"wrote {len(samples)} sources to {args.out}")
        return 0
    _ = split_holdout, inventory_json
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
