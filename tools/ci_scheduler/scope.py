"""Decide whether a diff can change the classparser adversarial suite.

The suite starts the classparser test binary and its repository-local
dependencies. A diff outside that closure cannot change the binary or its
fixtures, so running the suite would repeat a deterministic result. The
decision is advisory for CI topology only: the full suite still runs on every
push that touches the closure, and the ci/full label or a manual dispatch
forces it for any diff.
"""

from __future__ import annotations

import os
import subprocess
import sys

# These paths change how the suite is built, scheduled, or fed, even though
# they are not Go imports of the classparser package.
ALWAYS_FULL_PREFIXES = (
    ".github/workflows",
    "tools/ci_scheduler",
    "ci/task_t29",
    "test",
)
ALWAYS_FULL_FILES = ("go.mod", "go.sum")
FULL_LABEL = "ci/full"


def requires_classparser(changed: list[str], dependency_dirs: set[str]) -> bool:
    """Return True when the classparser suite must run for this diff.

    Unknown or empty diffs fail closed: the caller cannot prove the suite is
    unaffected, so it runs. Directory membership is checked on path
    components, not string prefixes, so ``classparser2`` is not inside
    ``classparser``.
    """
    if not changed:
        return True
    prefixes = set(dependency_dirs) | set(ALWAYS_FULL_PREFIXES)
    for path in changed:
        if path in ALWAYS_FULL_FILES:
            return True
        for prefix in prefixes:
            if path == prefix or path.startswith(prefix + "/"):
                return True
    return False


def repository_dependency_dirs(package: str = "./classparser") -> set[str]:
    """Repository-local directories imported by package and its tests."""
    result = subprocess.run(
        [
            "go",
            "list",
            "-deps",
            "-test",
            "-f",
            '{{if .Module}}{{if eq .Module.Path "github.com/yaklang/javajive"}}{{.Dir}}{{end}}{{end}}',
            package,
        ],
        check=True,
        capture_output=True,
        text=True,
    )
    root = os.getcwd() + os.sep
    dirs = set()
    for line in result.stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        dirs.add(line[len(root):] if line.startswith(root) else line)
    return dirs


def changed_files(base: str, head: str) -> list[str]:
    """Paths changed between base and head, or an empty list when unknown."""
    if not base or set(base) == {"0"}:
        return []
    subprocess.run(
        ["git", "fetch", "--no-tags", "--depth=1", "origin", base],
        check=False,
        capture_output=True,
        text=True,
    )
    result = subprocess.run(
        ["git", "diff", "--name-only", base, head],
        check=True,
        capture_output=True,
        text=True,
    )
    return [line for line in result.stdout.splitlines() if line]


def decide(event_name: str, base: str, head: str, labels: str) -> bool:
    """Return True when the classparser suite is required."""
    if event_name == "workflow_dispatch" or FULL_LABEL in labels.split(","):
        return True
    try:
        changed = changed_files(base, head)
        return requires_classparser(changed, repository_dependency_dirs())
    except (OSError, subprocess.SubprocessError, ValueError):
        return True


def main() -> int:
    full = decide(
        os.environ.get("EVENT_NAME", ""),
        (os.environ.get("BASE_SHA") or "").strip(),
        (os.environ.get("HEAD_SHA") or "HEAD").strip(),
        os.environ.get("PR_LABELS", ""),
    )
    print(f"classparser={'true' if full else 'false'}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
