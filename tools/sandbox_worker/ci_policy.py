"""Fail-closed GitHub Actions pin + permission review (no PyYAML)."""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from pathlib import Path

SHA40 = re.compile(r"^[0-9a-fA-F]{40}$")
USES_RE = re.compile(r"^(\s*)(?:-\s*)?uses:\s*(.+)$")
PERM_WRITE = re.compile(r"^\s+([A-Za-z0-9_-]+)\s*:\s*(write|admin)\s*$", re.I)
CHECKOUT_RE = re.compile(r"(^|/)actions/checkout@", re.I)

WRITE_PERM_NAMES = {
    "contents",
    "packages",
    "pull-requests",
    "id-token",
    "actions",
    "attestations",
    "security-events",
    "deployments",
    "issues",
    "checks",
    "repository-projects",
    "discussions",
    "statuses",
}


@dataclass
class WorkflowReview:
    path: str
    ok: bool
    errors: list[str] = field(default_factory=list)
    uses: list[str] = field(default_factory=list)
    pinned_uses: int = 0
    unpinned_uses: int = 0
    permissions_present: bool = False
    has_pull_request: bool = False
    has_pull_request_target: bool = False
    checkout_steps: int = 0
    checkout_persist_false: int = 0


def _strip_value(raw: str) -> str:
    raw = raw.strip()
    if not raw:
        return ""
    if raw[0] in {'"', "'"}:
        q = raw[0]
        end = raw.find(q, 1)
        if end > 0:
            return raw[1:end]
    if " #" in raw:
        raw = raw.split(" #", 1)[0]
    return raw.strip().strip("'\"")


def _header_and_jobs(text: str) -> tuple[str, str]:
    lines = text.splitlines()
    idx = None
    for i, line in enumerate(lines):
        if re.match(r"^jobs\s*:", line):
            idx = i
            break
    if idx is None:
        return text, ""
    return "\n".join(lines[:idx]), "\n".join(lines[idx:])


def _uncommented_lines(text: str) -> list[tuple[int, str]]:
    out: list[tuple[int, str]] = []
    for line in text.splitlines():
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        out.append((len(line) - len(line.lstrip(" ")), line.rstrip()))
    return out


def classify_uses(value: str) -> str:
    if value.startswith("./") or value.startswith(".\\"):
        return "local"
    if value.startswith("docker://"):
        return "pinned_digest" if "@sha256:" in value else "unpinned_docker"
    if "@" not in value:
        return "unpinned"
    ref = value.rsplit("@", 1)[1]
    if SHA40.fullmatch(ref):
        return "pinned_sha"
    return "unpinned"


def review_workflow_text(text: str, path: str = "<memory>") -> WorkflowReview:
    review = WorkflowReview(path=path, ok=True)
    header, _jobs = _header_and_jobs(text)
    header_nc = "\n".join(
        ln for ln in header.splitlines() if ln.strip() and not ln.lstrip().startswith("#")
    )
    review.has_pull_request_target = bool(re.search(r"\bpull_request_target\b", header_nc))
    review.has_pull_request = bool(re.search(r"\bpull_request\b", header_nc))
    if review.has_pull_request_target:
        review.errors.append("pull_request_target is forbidden for untrusted workflows")

    lines = text.splitlines()
    workflow_perm = False
    job_perm_jobs = 0
    job_count = 0
    in_permissions = False
    perm_indent = 0
    perm_scope_pr = False
    current_step_indent: int | None = None
    current_is_checkout = False
    current_with: dict[str, str] = {}
    current_uses: str | None = None

    def flush_step() -> None:
        nonlocal current_is_checkout, current_with, current_uses, current_step_indent
        if current_uses:
            review.uses.append(current_uses)
            kind = classify_uses(current_uses)
            if kind in {"pinned_sha", "pinned_digest", "local"}:
                review.pinned_uses += 1
            else:
                review.unpinned_uses += 1
                review.errors.append(f"unpinned uses: {current_uses}")
            if current_is_checkout:
                review.checkout_steps += 1
                persist = current_with.get("persist-credentials", "")
                if persist.lower() in {"false", '"false"', "'false'"}:
                    review.checkout_persist_false += 1
                else:
                    review.errors.append(
                        f"checkout persist-credentials is not false ({persist!r}) for {current_uses}"
                    )
        current_is_checkout = False
        current_with = {}
        current_uses = None
        current_step_indent = None

    i = 0
    while i < len(lines):
        raw = lines[i]
        stripped = raw.strip()
        if not stripped or stripped.startswith("#"):
            i += 1
            continue
        indent = len(raw) - len(raw.lstrip(" "))
        # workflow-level permissions
        if indent == 0 and re.match(r"^permissions\s*:", stripped):
            workflow_perm = True
            in_permissions = True
            perm_indent = 0
            perm_scope_pr = review.has_pull_request
            val = stripped.split(":", 1)[1].strip()
            if val in {"write-all", "read-all"}:
                if val == "write-all" or (review.has_pull_request and val == "read-all"):
                    review.errors.append(f"permissions: {val} is not least privilege")
            i += 1
            continue
        if indent == 0 and re.match(r"^jobs\s*:", stripped):
            in_permissions = False
        if in_permissions and indent > perm_indent:
            m = PERM_WRITE.match(raw)
            if m and review.has_pull_request:
                review.errors.append(f"pull_request grants {m.group(1)}: {m.group(2)}")
            i += 1
            continue
        if in_permissions and indent <= perm_indent:
            in_permissions = False

        # job name
        if indent == 2 and re.match(r"^[A-Za-z0-9_-]+\s*:", stripped) and "jobs:" in "\n".join(lines[: i + 1]):
            # approximate: job keys live at indent 2 under jobs:
            pass
        if indent == 2 and i > 0 and re.search(r"^\s{2}[A-Za-z0-9_-]+\s*:", raw):
            # could be a job
            job_count += 1 if re.search(r"^jobs\s*:", "\n".join(x.strip() and x or "" for x in lines)) else 0

        if re.match(r"^\s{2}[A-Za-z0-9_-]+\s*:", raw) and not stripped.startswith("-"):
            # possible job-level permissions next
            pass

        if re.match(r"^\s{4}permissions\s*:", raw):
            job_perm_jobs += 1
            in_permissions = True
            perm_indent = 4
            perm_scope_pr = review.has_pull_request
            i += 1
            continue

        um = USES_RE.match(raw)
        if um:
            flush_step()
            # Step boundary is the list item '-', which may be a parent of `uses:`.
            dash_indent = indent
            for j in range(i, -1, -1):
                prev = lines[j]
                if not prev.strip() or prev.lstrip().startswith("#"):
                    continue
                pindent = len(prev) - len(prev.lstrip(" "))
                if prev.lstrip().startswith("- "):
                    dash_indent = pindent
                    break
                if pindent < indent and not prev.lstrip().startswith("-"):
                    dash_indent = pindent
                    break
            current_step_indent = dash_indent
            current_uses = _strip_value(um.group(2))
            current_is_checkout = bool(CHECKOUT_RE.search(current_uses))
            i += 1
            while i < len(lines):
                nxt = lines[i]
                if not nxt.strip() or nxt.lstrip().startswith("#"):
                    i += 1
                    continue
                nindent = len(nxt) - len(nxt.lstrip(" "))
                ns = nxt.strip()
                if nindent <= dash_indent:
                    break
                if ns.startswith("- "):
                    break
                if ns.startswith("with:"):
                    i += 1
                    continue
                if ":" in ns:
                    k, v = ns.split(":", 1)
                    current_with[k.strip()] = v.strip()
                i += 1
            continue
        i += 1
    flush_step()

    review.permissions_present = workflow_perm or job_perm_jobs > 0
    if not review.permissions_present:
        review.errors.append("missing permissions: fail closed")
    if review.has_pull_request and not review.permissions_present:
        review.errors.append("pull_request workflow missing permissions")
    if review.checkout_steps == 0 and review.has_pull_request:
        # not always an error, but untrusted PR without checkout is unusual
        pass
    if review.unpinned_uses:
        review.errors.append(f"{review.unpinned_uses} unpinned actions")
    review.ok = not review.errors
    return review


def review_workflows_dir(root: Path) -> list[WorkflowReview]:
    reviews: list[WorkflowReview] = []
    if not root.is_dir():
        r = WorkflowReview(path=str(root), ok=False, errors=["workflows directory missing"])
        return [r]
    files = sorted(list(root.glob("*.yml")) + list(root.glob("*.yaml")))
    if not files:
        return [WorkflowReview(path=str(root), ok=False, errors=["no workflow files"])]
    for path in files:
        reviews.append(review_workflow_text(path.read_text(encoding="utf-8"), str(path)))
    return reviews


def review_ok(reviews: list[WorkflowReview]) -> tuple[bool, list[str]]:
    errors: list[str] = []
    for r in reviews:
        for e in r.errors:
            errors.append(f"{r.path}: {e}")
    return (not errors, errors)
