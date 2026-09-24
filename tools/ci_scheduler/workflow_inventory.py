"""Read-only inventory of existing GitHub workflow jobs. Never deletes required checks."""

from __future__ import annotations

import re
from pathlib import Path
from typing import Any


JOB_NAME = re.compile(r"^  ([A-Za-z0-9_-]+):\s*$", re.M)
USES = re.compile(r"uses:\s*(\S+)")
PERM = re.compile(r"^permissions:\s*$", re.M)
PINNED = re.compile(r"^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}$")


def list_jobs(workflow: Path) -> list[str]:
    text = workflow.read_text(encoding="utf-8")
    jobs_at = text.find("\njobs:")
    if jobs_at < 0:
        jobs_at = text.find("\njobs:\n")
    region = text[jobs_at:] if jobs_at >= 0 else text
    jobs = []
    for line in region.splitlines():
        if re.fullmatch(r"  [A-Za-z0-9_-]+:", line):
            jobs.append(line.strip().rstrip(":"))
    return jobs


def required_coverage_snapshot(repo_root: Path) -> dict[str, Any]:
    ci = repo_root / ".github" / "workflows" / "ci.yml"
    pages = repo_root / ".github" / "workflows" / "deploy-pages.yml"
    jobs = {
        "ci.yml": list_jobs(ci) if ci.is_file() else [],
        "deploy-pages.yml": list_jobs(pages) if pages.is_file() else [],
    }
    uses = []
    for path in (ci, pages):
        if not path.is_file():
            continue
        for match in USES.finditer(path.read_text(encoding="utf-8")):
            uses.append({"file": path.name, "uses": match.group(1)})
    return {
        "jobs": jobs,
        "uses": uses,
        "job_count": sum(len(v) for v in jobs.values()),
    }


# The pull-request CI stays a single fast algorithm-regression gate. Full task
# contracts, platform builds, and historical-JAR audits remain available through
# the explicit task-gates / extended / untrusted-oracle workflows.
BASELINE_CI_JOBS = ("regression",)


def action_pins(workflow: Path) -> list[dict[str, Any]]:
    rows = []
    for match in USES.finditer(workflow.read_text(encoding="utf-8")):
        spec = match.group(1).strip().strip("'\"")
        if spec.startswith("./") or spec.startswith("docker://"):
            continue
        pinned = bool(PINNED.fullmatch(spec))
        rows.append({"file": workflow.name, "uses": spec, "pinned": pinned})
    return rows


def unpinned_third_party(repo_root: Path) -> list[dict[str, Any]]:
    bad = []
    for path in sorted((repo_root / ".github" / "workflows").glob("*.yml")):
        for row in action_pins(path):
            if not row["pinned"]:
                bad.append(row)
    return bad
