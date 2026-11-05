#!/usr/bin/env python3
"""Enforce the measured DEX translator statement and function baseline."""

from __future__ import annotations

import argparse
import json
import pathlib
import re
import subprocess
import sys
import tempfile
from dataclasses import dataclass


PROFILE_LINE = re.compile(r"^(?P<location>\S+) (?P<statements>\d+) (?P<count>\d+)$")
FUNCTION_LINE = re.compile(
    r"^(?P<file>\S+\.go):\d+:\s+(?P<function>\S+)\s+(?P<percent>\d+(?:\.\d+)?)%$"
)


@dataclass(frozen=True)
class Coverage:
    covered: int
    statements: int

    @property
    def percent(self) -> float:
        return 100.0 * self.covered / self.statements if self.statements else 100.0


def parse_profile(path: pathlib.Path, module: str) -> dict[str, Coverage]:
    lines = path.read_text(encoding="utf-8").splitlines()
    if not lines or lines[0] not in {"mode: set", "mode: count", "mode: atomic"}:
        raise ValueError(f"{path}: missing or unsupported coverage mode")

    # -coverpkg emits the same instrumented block once per test binary. Merge
    # identical locations, counting a block covered if any binary executed it.
    blocks: dict[tuple[str, int], int] = {}
    prefix = module + "/"
    for line_number, line in enumerate(lines[1:], 2):
        match = PROFILE_LINE.fullmatch(line)
        if not match:
            raise ValueError(f"{path}:{line_number}: malformed coverage record")
        location = match.group("location")
        statements = int(match.group("statements"))
        count = int(match.group("count"))
        key = (location, statements)
        blocks[key] = max(blocks.get(key, 0), count)

    totals: dict[str, list[int]] = {}
    for (location, statements), count in blocks.items():
        filename = location.split(":", 1)[0]
        if not filename.startswith(prefix):
            raise ValueError(f"coverage file outside module {module!r}: {filename}")
        package = filename.rsplit("/", 1)[0]
        values = totals.setdefault(package, [0, 0])
        values[0] += statements if count else 0
        values[1] += statements
    return {package: Coverage(*values) for package, values in totals.items()}


def function_identity(module: str, filename: str, function: str) -> str:
    prefix = module + "/"
    if not filename.startswith(prefix):
        raise ValueError(f"function file outside module {module!r}: {filename}")
    return f"{filename[len(prefix):]}:{function}"


def zero_functions(
    profile: pathlib.Path, module: str, go: str, module_root: pathlib.Path
) -> set[str]:
    result = subprocess.run(
        [go, "tool", "cover", f"-func={profile}"],
        check=True,
        capture_output=True,
        text=True,
        cwd=module_root,
    )
    uncovered: set[str] = set()
    for line in result.stdout.splitlines():
        if line.startswith("total:"):
            continue
        match = FUNCTION_LINE.fullmatch(line)
        if not match:
            raise ValueError(f"unrecognized go tool cover output: {line!r}")
        if float(match.group("percent")) == 0.0:
            uncovered.add(
                function_identity(module, match.group("file"), match.group("function"))
            )
    return uncovered


def ratio_at_least(actual: Coverage, baseline: Coverage) -> bool:
    if actual.statements == 0 or baseline.statements == 0:
        return actual.statements == baseline.statements
    return actual.covered * baseline.statements >= baseline.covered * actual.statements


def check(
    profile: pathlib.Path,
    baseline_path: pathlib.Path,
    summary_path: pathlib.Path,
    go: str,
) -> int:
    baseline = json.loads(baseline_path.read_text(encoding="utf-8"))
    module = baseline["module"]
    actual = parse_profile(profile, module)
    expected_packages = {
        package: Coverage(values["covered"], values["statements"])
        for package, values in baseline["packages"].items()
    }
    errors: list[str] = []

    missing = sorted(set(expected_packages) - set(actual))
    unexpected = sorted(set(actual) - set(expected_packages))
    if missing:
        errors.append("missing package coverage: " + ", ".join(missing))
    if unexpected:
        errors.append("unbaselined package coverage: " + ", ".join(unexpected))

    for package in sorted(set(expected_packages) & set(actual)):
        if not ratio_at_least(actual[package], expected_packages[package]):
            errors.append(
                f"package regression {package}: {actual[package].covered}/"
                f"{actual[package].statements} ({actual[package].percent:.6f}%) < "
                f"{expected_packages[package].covered}/{expected_packages[package].statements} "
                f"({expected_packages[package].percent:.6f}%)"
            )

    total = Coverage(
        sum(value.covered for value in actual.values()),
        sum(value.statements for value in actual.values()),
    )
    expected_total = Coverage(
        baseline["total"]["covered"], baseline["total"]["statements"]
    )
    if not ratio_at_least(total, expected_total):
        errors.append(
            f"total regression: {total.covered}/{total.statements} ({total.percent:.6f}%) < "
            f"{expected_total.covered}/{expected_total.statements} "
            f"({expected_total.percent:.6f}%)"
        )

    module_root = baseline_path.resolve().parents[1]
    uncovered = zero_functions(profile.resolve(), module, go, module_root)
    allowed = set(baseline["zero_covered_functions"])
    new_uncovered = sorted(uncovered - allowed)
    if new_uncovered:
        errors.append("new 0%-covered functions: " + ", ".join(new_uncovered))

    summary = {
        "baseline_revision": baseline["measured_revision"],
        "passed": not errors,
        "total": {
            "covered": total.covered,
            "statements": total.statements,
            "percent": round(total.percent, 6),
        },
        "packages": {
            package: {
                "covered": value.covered,
                "statements": value.statements,
                "percent": round(value.percent, 6),
            }
            for package, value in sorted(actual.items())
        },
        "zero_covered_functions": sorted(uncovered),
        "errors": errors,
    }
    summary_path.parent.mkdir(parents=True, exist_ok=True)
    summary_path.write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")
    for error in errors:
        print(f"coverage contract: {error}", file=sys.stderr)
    if not errors:
        print(
            f"DEX coverage contract passed: {total.covered}/{total.statements} "
            f"({total.percent:.6f}%)"
        )
    return int(bool(errors))


def self_test() -> int:
    sample = """mode: atomic
example.test/mod/a/a.go:1.1,2.2 2 0
example.test/mod/a/a.go:1.1,2.2 2 3
example.test/mod/b/b.go:3.1,4.2 1 0
"""
    with tempfile.TemporaryDirectory() as directory:
        profile = pathlib.Path(directory) / "cover.out"
        profile.write_text(sample, encoding="utf-8")
        got = parse_profile(profile, "example.test/mod")
    want = {
        "example.test/mod/a": Coverage(2, 2),
        "example.test/mod/b": Coverage(0, 1),
    }
    if got != want:
        raise AssertionError(f"profile merge: got {got!r}, want {want!r}")
    if not ratio_at_least(Coverage(3, 4), Coverage(6, 8)):
        raise AssertionError("equal ratios must pass")
    if ratio_at_least(Coverage(2, 3), Coverage(3, 4)):
        raise AssertionError("lower ratios must fail")
    identity = function_identity("example.test/mod", "example.test/mod/a/a.go", "newThing")
    if identity != "a/a.go:newThing":
        raise AssertionError(f"function identity = {identity!r}")
    print("coverage checker self-test passed")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--profile", type=pathlib.Path)
    parser.add_argument("--baseline", type=pathlib.Path)
    parser.add_argument("--summary", type=pathlib.Path)
    parser.add_argument("--go", default="go")
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        return self_test()
    if not all((args.profile, args.baseline, args.summary)):
        parser.error("--profile, --baseline, and --summary are required")
    try:
        return check(args.profile, args.baseline, args.summary, args.go)
    except (KeyError, OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"coverage contract: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
