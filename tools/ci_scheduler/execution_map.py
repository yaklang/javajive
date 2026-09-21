"""Command/execution-event mapping for the 187 pack cases.

Coverage is granted only when a log contains a matching execution event for the
exact required test name. String mention of a case ID, truncated Go names, or
an unexecuted `go test -run` zero-match is NOT coverage. Missing/unrun is
fail-closed (NOT_RUN), never PASS.
"""

from __future__ import annotations

import json
import re
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any, Mapping, Sequence

# 187 pack case IDs (T03/T08/T14/T26/T27 have five cases). Do not invent extras.
PACK_CASE_IDS: tuple[str, ...] = (
    tuple(f"T01-C0{i}" for i in range(1, 7))
    + tuple(f"T02-C0{i}" for i in range(1, 7))
    + tuple(f"T03-C0{i}" for i in range(1, 6))
    + tuple(f"T04-C0{i}" for i in range(1, 7))
    + tuple(f"T05-C0{i}" for i in range(1, 7))
    + tuple(f"T06-C0{i}" for i in range(1, 7))
    + tuple(f"T07-C0{i}" for i in range(1, 7))
    + tuple(f"T08-C0{i}" for i in range(1, 6))
    + tuple(f"T09-C0{i}" for i in range(1, 7))
    + tuple(f"T10-C0{i}" for i in range(1, 7))
    + tuple(f"T11-C0{i}" for i in range(1, 7))
    + tuple(f"T12-C0{i}" for i in range(1, 7))
    + tuple(f"T13-C0{i}" for i in range(1, 7))
    + tuple(f"T14-C0{i}" for i in range(1, 6))
    + tuple(f"T15-C0{i}" for i in range(1, 7))
    + tuple(f"T16-C0{i}" for i in range(1, 7))
    + tuple(f"T17-C0{i}" for i in range(1, 7))
    + tuple(f"T18-C0{i}" for i in range(1, 7))
    + tuple(f"T19-C0{i}" for i in range(1, 7))
    + tuple(f"T20-C0{i}" for i in range(1, 7))
    + tuple(f"T21-C0{i}" for i in range(1, 7))
    + tuple(f"T22-C0{i}" for i in range(1, 7))
    + tuple(f"T23-C0{i}" for i in range(1, 7))
    + tuple(f"T24-C0{i}" for i in range(1, 7))
    + tuple(f"T25-C0{i}" for i in range(1, 7))
    + tuple(f"T26-C0{i}" for i in range(1, 6))
    + tuple(f"T27-C0{i}" for i in range(1, 6))
    + tuple(f"T28-C0{i}" for i in range(1, 7))
    + tuple(f"T29-C0{i}" for i in range(1, 7))
    + tuple(f"T30-C0{i}" for i in range(1, 7))
    + tuple(f"T31-C0{i}" for i in range(1, 7))
    + tuple(f"T32-C0{i}" for i in range(1, 7))
)

if len(PACK_CASE_IDS) != 187 or len(set(PACK_CASE_IDS)) != 187:
    raise RuntimeError(f"PACK_CASE_IDS must be 187 unique ids, got {len(PACK_CASE_IDS)}")

GO_RUN = re.compile(r"^=== RUN\s+(\S+)\s*$", re.M)
GO_RESULT = re.compile(r"^--- (PASS|FAIL|SKIP):\s+(\S+)", re.M)
# unittest -v: "test_foo (mod.Class) ... ok"
PY_V = re.compile(
    r"^(test_\S+) \([^)]+\) \.\.\. (ok|FAIL|ERROR|skipped)",
    re.M,
)
# unittest failure header: "FAIL: test_foo (mod.Class)"
PY_FAIL_HDR = re.compile(r"^FAIL: (test_\S+) \(", re.M)
@dataclass(frozen=True)
class RequiredCommand:
    """One executable required for a pack case."""

    kind: str  # go_test | python_unittest
    test_name: str  # exact Go TestX or Python test_ method
    package: str = ""  # go package or python module path (informational)
    argv: tuple[str, ...] = ()

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


def _py(module: str, name: str) -> RequiredCommand:
    return RequiredCommand(
        kind="python_unittest",
        test_name=name,
        package=module,
        argv=("python3", "-m", "unittest", f"{module}.{name}"),
    )


def _go(package: str, name: str) -> RequiredCommand:
    return RequiredCommand(
        kind="go_test",
        test_name=name,
        package=package,
        argv=("go", "test", package, "-count=1", "-run", f"^{name}$"),
    )


# Explicit required commands for this evidence owner's cases. Unmapped cases
# stay NOT_MAPPED (fail-closed), never PASS by filename mention.
EVIDENCE_COMMANDS: dict[str, tuple[RequiredCommand, ...]] = {
    "T01-C01": (
        _py("ci.task_t01.test_t01_c01_inventory", "test_T01_C01_drop_one_names_missing_id"),
        _py("ci.task_t01.test_t01_c01_inventory", "test_T01_C01_duplicate_id_fails"),
        _py("ci.task_t01.test_t01_adversarial", "test_T01_C01_skip_is_not_pass"),
    ),
    "T01-C02": (
        _py("ci.task_t01.test_t01_c02_dual_ledger", "test_T01_C02_fail_then_pass_retains_defect_and_promotes_gate"),
    ),
    "T01-C03": (
        _py("ci.task_t01.test_t01_c03_two_anchors", "test_T01_C03_base_equals_candidate_but_milestone_capability_lost"),
    ),
    "T01-C04": (
        _py("ci.task_t01.test_t01_c04_comparability", "test_T01_C04_different_harness_digest_incomparable"),
        _py("ci.task_t01.test_t01_c04_comparability", "test_T01_C04_broken_run_is_infra_error_not_equal"),
    ),
    "T01-C05": (
        _py("ci.task_t01.test_t01_c05_observe", "test_T01_C05_observe_rc0_failed_9_is_not_pass_or_complete"),
        _py("ci.task_t01.test_t01_c05_observe", "test_T01_C05_gate_policy_failed_is_fail"),
    ),
    "T01-C06": (
        _py("ci.task_t01.test_t01_c06_interface_split", "test_T01_C06_precision_unknown_when_not_executed"),
        _py("ci.task_t01.test_t01_c06_interface_split", "test_T01_C06_mutate_precision_pass_without_evidence_rejected"),
    ),
    "T28-C01": (_py("ci.task_t28.test_t28_contracts", "test_T28_C01_generated_classes_are_jvm_verified"),),
    "T28-C02": (_py("ci.task_t28.test_t28_contracts", "test_T28_C02_morph_preserves_runtime_oracle_then_javajive"),),
    "T28-C03": (_py("ci.task_t28.test_t28_contracts", "test_T28_C03_reduce_keeps_failure_class_and_rejects_illegal"),),
    "T28-C04": (_py("ci.task_t28.test_t28_contracts", "test_T28_C04_holdout_groups_forbid_leakage"),),
    "T28-C05": (_py("ci.task_t28.test_t28_contracts", "test_T28_C05_compiler_identities_are_not_merged"),),
    "T28-C06": (_py("ci.task_t28.test_t28_contracts", "test_T28_C06_every_failure_stage_has_full_evidence"),),
    "T29-C01": (_py("ci.task_t29.test_t29_contracts", "test_T29_C01_shard_inventory_is_conserved"),),
    "T29-C02": (_py("ci.task_t29.test_t29_contracts", "test_T29_C02_duration_schedule_uses_measured_times"),),
    "T29-C03": (_py("ci.task_t29.test_t29_contracts", "test_T29_C03_local_gate_does_not_hide_unrepaired_failures"),),
    "T29-C04": (_py("ci.task_t29.test_t29_contracts", "test_T29_C04_pr_baseline_cache_is_untrusted"),),
    "T29-C05": (_py("ci.task_t29.test_t29_contracts", "test_T29_C05_missing_jdk_or_zero_matches_are_infra_error"),),
    "T29-C06": (_py("ci.task_t29.test_t29_contracts", "test_T29_C06_observe_to_gate_requires_review"),),
    "T30-C01": (_py("ci.task_t30.test_t30_c01_credentials", "test_T30_C01_host_canary_invisible_and_mount_inventory"),),
    "T30-C02": (_py("ci.task_t30.test_t30_c02_network", "test_T30_C02_policy_blocks_live_local_tcp"),),
    "T30-C03": (_py("ci.task_t30.test_t30_c03_resources", "test_T30_C03_limits_or_kill_and_no_leftover_children"),),
    "T30-C04": (_py("ci.task_t30.test_t30_c04_artifacts", "test_T30_C04_path_escape_symlink_and_byte_cap"),),
    "T30-C05": (_py("ci.task_t30.test_t30_c05_ci_permissions", "test_T30_C05_pins_permissions_and_fail_closed"),),
    "T30-C06": (_py("ci.task_t30.test_t30_c06_baseline", "test_T30_C06_baseline_compile_verify_run_in_worker"),),
    "T32-C01": (_py("ci.task_t32.test_t32_c01_stage_isolation", "test_T32_C01_stage_isolation_same_class_per_stage_vs_e2e"),),
    "T32-C02": (_py("ci.task_t32.test_t32_c02_cold_warm", "test_T32_C02_cold_vs_warm_labels_and_cache_keys"),),
    "T32-C03": (_py("ci.task_t32.test_t32_c03_scale_curve", "test_T32_C03_scale_curve_hashes_and_node_edge_counts"),),
    "T32-C04": (_py("ci.task_t32.test_t32_c04_noise", "test_T32_C04_noise_vs_2x_regression"),),
    "T32-C05": (_py("ci.task_t32.test_t32_c05_stub_skip", "test_T32_C05_faster_stub_is_correctness_regression"),),
    "T32-C06": (_py("ci.task_t32.test_t32_c06_replay", "test_T32_C06_replay_command_and_stats_format"),),
}


@dataclass
class Event:
    test_name: str
    result: str  # PASS | FAIL | SKIP | ERROR | RUN
    kind: str


def parse_execution_events(text: str) -> list[Event]:
    """Parse go test / unittest execution events. Mentions are ignored."""
    events: list[Event] = []
    for match in GO_RUN.finditer(text):
        events.append(Event(match.group(1), "RUN", "go_test"))
    for match in GO_RESULT.finditer(text):
        events.append(Event(match.group(2), match.group(1), "go_test"))
    for match in PY_V.finditer(text):
        status = {"ok": "PASS", "FAIL": "FAIL", "ERROR": "ERROR", "skipped": "SKIP"}[match.group(2)]
        events.append(Event(match.group(1), status, "python_unittest"))
    for match in PY_FAIL_HDR.finditer(text):
        events.append(Event(match.group(1), "FAIL", "python_unittest"))
    return events


def _name_exact(got: str, required: str) -> bool:
    """Exact test identity. Prefix/truncation is not a match."""
    return got == required


def command_outcome(cmd: RequiredCommand, events: Sequence[Event]) -> str:
    """PASS only if this exact test ran and passed. SKIP/FAIL/missing stay fail-closed."""
    ran = False
    passed = False
    failed = False
    skipped = False
    for ev in events:
        if not _name_exact(ev.test_name, cmd.test_name):
            continue
        ran = True
        if ev.result == "PASS":
            passed = True
        elif ev.result in {"FAIL", "ERROR"}:
            failed = True
        elif ev.result == "SKIP":
            skipped = True
    if failed:
        return "FAIL"
    if skipped and not passed:
        return "SKIP"
    if passed and ran:
        return "PASS"
    return "NOT_RUN"


def evaluate_case(
    case_id: str,
    events: Sequence[Event],
    *,
    commands: Mapping[str, Sequence[RequiredCommand]] | None = None,
) -> dict[str, Any]:
    mapping = commands if commands is not None else EVIDENCE_COMMANDS
    required = list(mapping.get(case_id) or ())
    if not required:
        return {
            "case_id": case_id,
            "status": "NOT_MAPPED",
            "pass": False,
            "reason": "no required command mapped; string mention is not coverage",
            "commands": [],
        }
    rows = []
    statuses = []
    for cmd in required:
        st = command_outcome(cmd, events)
        statuses.append(st)
        rows.append({"command": cmd.to_dict(), "status": st})
    if any(s == "FAIL" for s in statuses):
        overall = "FAIL"
    elif any(s in {"NOT_RUN", "SKIP"} for s in statuses):
        overall = "NOT_RUN"
    elif all(s == "PASS" for s in statuses):
        overall = "PASS"
    else:
        overall = "NOT_RUN"
    return {
        "case_id": case_id,
        "status": overall,
        "pass": overall == "PASS",
        "contract_proven": False,
        "reason": None if overall == "PASS" else "missing/unrun/fail is fail-closed",
        "note": "execution event only; not pack-oracle completeness",
        "commands": rows,
    }


def evaluate_inventory(
    log_text: str,
    *,
    case_ids: Sequence[str] | None = None,
    commands: Mapping[str, Sequence[RequiredCommand]] | None = None,
) -> dict[str, Any]:
    ids = list(case_ids or PACK_CASE_IDS)
    events = parse_execution_events(log_text)
    cases = [evaluate_case(cid, events, commands=commands) for cid in ids]
    counts = {
        "total": len(cases),
        "pass": sum(1 for c in cases if c["status"] == "PASS"),
        "fail": sum(1 for c in cases if c["status"] == "FAIL"),
        "not_run": sum(1 for c in cases if c["status"] == "NOT_RUN"),
        "not_mapped": sum(1 for c in cases if c["status"] == "NOT_MAPPED"),
    }
    mapping = commands if commands is not None else EVIDENCE_COMMANDS
    mapping_complete = all(cid in mapping and len(mapping[cid]) > 0 for cid in ids)
    # A mapped unit PASS is an execution event. It does not prove the pack oracle.
    pack_contracts_proven = False
    overall = (
        mapping_complete
        and counts["total"] == 187
        and counts["pass"] == 187
        and counts["fail"] == 0
        and counts["not_run"] == 0
        and counts["not_mapped"] == 0
        and pack_contracts_proven
    )
    return {
        "schema_version": 1,
        "rule": "execution-event exact name; mention/truncation/unrun is not PASS",
        "counts": counts,
        "overall_pass": overall,
        "mapping_complete": mapping_complete,
        "pack_contracts_proven": pack_contracts_proven,
        "note": (
            "Mapped unittest/go-test PASS is not pack-oracle completeness. "
            "Unmapped cases stay NOT_MAPPED. This helper never claims all 187 contracts."
        ),
        "cases": cases,
        "event_count": len(events),
    }


def mention_is_not_coverage(log_text: str, case_id: str) -> bool:
    """True when the case id appears as text but has no execution event."""
    if case_id not in log_text:
        return True
    events = parse_execution_events(log_text)
    mapped = EVIDENCE_COMMANDS.get(case_id) or ()
    if not mapped:
        return True
    return all(command_outcome(cmd, events) == "NOT_RUN" for cmd in mapped)


def load_pack_case_ids(pack_root: Path | None) -> tuple[str, ...]:
    if pack_root is None or not Path(pack_root).is_dir():
        return PACK_CASE_IDS
    ids: list[str] = []
    for path in sorted(Path(pack_root).glob("T*/tests/cases.json")):
        data = json.loads(path.read_text(encoding="utf-8"))
        for row in data:
            cid = str(row.get("id") or "")
            if cid:
                ids.append(cid)
    if len(ids) != 187:
        raise ValueError(f"pack cases.json yielded {len(ids)} ids, expected 187")
    if tuple(ids) != PACK_CASE_IDS:
        raise ValueError("pack case id list drifted from PACK_CASE_IDS")
    return tuple(ids)
