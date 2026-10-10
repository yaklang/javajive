"""stdlib CLI: validate, compare, aggregate."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

from .aggregate import aggregate, expand_manifest
from .compare import compare_anchors
from .constants import MILESTONE_SHA, PR_BASE_SHA, SCHEMA_VERSION
from .errors import CorruptJSONError, InventoryError, LedgerError
from .historical import load_historical_cli_report
from .observation import load_json_path, parse_observation, parse_observation_list
from .report import classify_task_report


def _dump(payload: Any) -> str:
    return json.dumps(payload, ensure_ascii=False, indent=2) + "\n"


def _load(path: str) -> Any:
    return load_json_path(path)


def cmd_validate(args: argparse.Namespace) -> int:
    errors: list[str] = []
    observations: list[Any] = []
    if args.historical:
        loaded = load_historical_cli_report(args.historical if args.historical != "-" else None)
        observations = loaded["observations"]
        print(_dump({"historical_cli": len(loaded["cli"]), "api_placeholders": len(loaded["api_placeholders"])}))
    if args.observations:
        try:
            observations = parse_observation_list(args.observations)
        except (CorruptJSONError, LedgerError) as exc:
            print(str(exc), file=sys.stderr)
            return 2
    parsed = []
    for item in observations:
        try:
            obs = item if hasattr(item, "case_id") else parse_observation(item)
            parsed.append(obs)
            print(f"OK {obs.case_id} status={obs.status} mode={obs.evidence.mode}")
        except LedgerError as exc:
            errors.append(str(exc))
            print(f"ERROR {exc}", file=sys.stderr)
    if not parsed and not errors:
        print("EMPTY_RESULTS: no observations", file=sys.stderr)
        return 1
    print(_dump({"schema_version": SCHEMA_VERSION, "count": len(parsed), "errors": errors}))
    return 1 if errors else 0


def cmd_compare(args: argparse.Namespace) -> int:
    try:
        result = compare_anchors(
            args.candidate,
            args.pr_base,
            args.milestone,
            milestone_sha=args.milestone_sha,
            pr_base_sha=args.pr_base_sha,
        )
    except (CorruptJSONError, LedgerError) as exc:
        print(str(exc), file=sys.stderr)
        return 2 if isinstance(exc, CorruptJSONError) else 1
    text = _dump(result)
    if args.out:
        Path(args.out).write_text(text, encoding="utf-8")
    print(text, end="")
    verdict = result.get("overall_verdict")
    if verdict in {"infra_error"}:
        return 2
    if verdict in {"incomparable", "long_term_drift", "regression"}:
        return 1
    return 0


def cmd_aggregate(args: argparse.Namespace) -> int:
    try:
        manifest = _load(args.manifest) if not args.manifest.endswith(".ids") else {
            "expected_ids": Path(args.manifest).read_text(encoding="utf-8").split()
        }
        if args.expected_ids:
            manifest = {"expected_ids": list(args.expected_ids)}
        result = aggregate(manifest, args.observations, strict=False)
    except CorruptJSONError as exc:
        print(str(exc), file=sys.stderr)
        return 2
    except InventoryError as exc:
        print(str(exc), file=sys.stderr)
        for cid in exc.missing_ids:
            print(f"MISSING ID: {cid}", file=sys.stderr)
        return 1
    except LedgerError as exc:
        print(str(exc), file=sys.stderr)
        return 1
    printed: set[str] = set()
    for err in result.errors:
        print(err, file=sys.stderr)
        printed.add(err)
    for cid in result.missing_ids:
        line = f"MISSING ID: {cid}"
        if line not in printed:
            print(line, file=sys.stderr)
    text = _dump(result.to_dict())
    if args.out:
        Path(args.out).write_text(text, encoding="utf-8")
    print(text, end="")
    return 0 if result.ok else 1


def cmd_observe_report(args: argparse.Namespace) -> int:
    report = _load(args.report)
    outcome = classify_task_report(args.exit_code, report)
    print(_dump(outcome.to_dict()), end="")
    if outcome.is_success:
        return 0
    return 1


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="milestone_ledger",
        description="T01 inventory / comparability / dual-anchor / dual-ledger protocol",
    )
    sub = parser.add_subparsers(dest="command", required=True)

    p = sub.add_parser("validate", help="parse and validate observations")
    p.add_argument("--observations", help="JSON file of observations or a report")
    p.add_argument("--historical", nargs="?", const="-", help="load packaged historical CLI excerpt")
    p.set_defaults(func=cmd_validate)

    p = sub.add_parser("compare", help="candidate vs PR base and vs milestone")
    p.add_argument("--candidate", required=True)
    p.add_argument("--pr-base", required=True)
    p.add_argument("--milestone", required=True)
    p.add_argument("--out")
    p.add_argument("--milestone-sha", default=MILESTONE_SHA)
    p.add_argument("--pr-base-sha", default=PR_BASE_SHA)
    p.set_defaults(func=cmd_compare)

    p = sub.add_parser("aggregate", help="inventory conservation over a manifest")
    p.add_argument("--manifest", required=True)
    p.add_argument("--observations", required=True)
    p.add_argument("--expected-ids", nargs="*")
    p.add_argument("--out")
    p.set_defaults(func=cmd_aggregate)

    p = sub.add_parser("observe-report", help="classify an observe/gate runner report")
    p.add_argument("--report", required=True)
    p.add_argument("--exit-code", type=int, required=True)
    p.set_defaults(func=cmd_observe_report)
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    try:
        return int(args.func(args))
    except BrokenPipeError:
        return 0
