#!/usr/bin/env python3
"""T32 performance-baseline analysis. No uncalibrated percentage pass gates."""
from __future__ import annotations

import json
import random
from pathlib import Path

MIN_REPEATS = 10
KERNEL_STAGES = [
    "parse",
    "decode",
    "cfg_opcode_only",
    "semantic_cfg_build",
    "dataflow_stacksim_opcode_only",
    "t25_sparse_reaching",
    "t26_graph_cache",
    "region_and_statements_combined",
    "render_class_dump_reruns_decompile",
    "e2e_kernel",
]
HONEST_STAGE_MARKERS = {
    "cfg_opcode_only": "_opcode_only",
    "dataflow_stacksim_opcode_only": "_opcode_only",
    "region_and_statements_combined": "_combined",
    "render_class_dump_reruns_decompile": "_reruns_decompile",
}
SCALE_KINDS = ("loop", "handler_dense", "handler_sparse", "slot")
FORBIDDEN_STAGE_IDS = (
    "cfg_opcode_graph",
    "dataflow_stacksim",
    "render_class_dump_combined",
    "cfg",
    "dataflow",
    "region",
    "render",
)


def evidence_dir(repo: Path) -> Path:
    return repo / "tools" / "performance_baseline" / "evidence"


def load_summary(ev: Path) -> dict:
    return json.loads((ev / "summary.json").read_text(encoding="utf-8"))


def load_samples(ev: Path) -> list[dict]:
    path = ev / "samples.jsonl"
    rows = []
    for line in path.read_text(encoding="utf-8").splitlines():
        if line.strip():
            rows.append(json.loads(line))
    return rows


def measured_ns(group: dict) -> list[float]:
    out = []
    for s in group.get("samples") or []:
        if s.get("ok") and s.get("role") == "measured":
            out.append(float(s["ns"]))
    return out


def mean(vals: list[float]) -> float:
    return sum(vals) / len(vals) if vals else 0.0


def median(vals: list[float]) -> float:
    if not vals:
        return 0.0
    s = sorted(vals)
    n = len(s)
    mid = n // 2
    if n % 2 == 0:
        return (s[mid - 1] + s[mid]) / 2.0
    return s[mid]


def bootstrap_median_ratio(a: list[float], b: list[float], rng: random.Random, n: int = 2000) -> tuple[float, float]:
    ratios = []
    na, nb = len(a), len(b)
    for _ in range(n):
        aa = [a[rng.randrange(na)] for _ in range(na)]
        bb = [b[rng.randrange(nb)] for _ in range(nb)]
        am = median(aa)
        if am == 0:
            continue
        ratios.append(median(bb) / am)
    ratios.sort()
    if not ratios:
        return 0.0, 0.0
    lo = ratios[int(0.025 * len(ratios))]
    hi = ratios[min(len(ratios) - 1, int(0.975 * len(ratios)))]
    return lo, hi


def compare_distributions(baseline: list[float], candidate: list[float], seed: int = 32) -> dict:
    rng = random.Random(seed)
    bmed = median(baseline)
    cmed = median(candidate)
    ratio = (cmed / bmed) if bmed else 0.0
    lo, hi = bootstrap_median_ratio(baseline, candidate, rng)
    if lo <= 1 <= hi:
        verdict = "uncertain_not_improvement"
    elif ratio >= 1.8 and lo > 1:
        verdict = "clear_regression"
    elif lo > 1:
        verdict = "possible_regression_uncalibrated"
    else:
        verdict = "faster_uncalibrated_not_claimed"
    return {"ratio": ratio, "ci_low": lo, "ci_high": hi, "verdict": verdict}


def classify_semantic(baseline: dict, candidate: dict, oracle: dict | None = None) -> dict:
    """Independent classifier: skip/status/oracle, never Dump() hash as semantic fail."""
    oracle = oracle or {}
    if candidate.get("skipped_analysis") and not baseline.get("skipped_analysis"):
        return {
            "classification": "correctness_regression_not_perf_win",
            "correctness_regression": True,
            "recorded_as_perf_win": False,
            "source_hash_used_as_semantic_fail": False,
        }
    if (candidate.get("stub_method_count") or 0) > (baseline.get("stub_method_count") or 0):
        return {
            "classification": "correctness_regression_not_perf_win",
            "correctness_regression": True,
            "recorded_as_perf_win": False,
            "source_hash_used_as_semantic_fail": False,
        }
    rank = {"complete": 3, "partial": 2, "unsupported": 1, "error": 1, "invalid_input": 0}
    br = rank.get(baseline.get("decompile_status") or "", -1)
    cr = rank.get(candidate.get("decompile_status") or "", -1)
    if br >= 0 and cr >= 0 and cr < br:
        return {
            "classification": "correctness_regression_not_perf_win",
            "correctness_regression": True,
            "recorded_as_perf_win": False,
            "source_hash_used_as_semantic_fail": False,
        }
    if oracle.get("ran"):
        if not oracle.get("stdout_equal"):
            return {
                "classification": "correctness_regression_not_perf_win",
                "correctness_regression": True,
                "recorded_as_perf_win": False,
                "source_hash_used_as_semantic_fail": False,
            }
        return {
            "classification": "eligible_for_perf_compare",
            "correctness_regression": False,
            "recorded_as_perf_win": False,
            "source_hash_used_as_semantic_fail": False,
        }
    bhash = baseline.get("output_sha256") or ""
    chash = candidate.get("output_sha256") or ""
    if bhash and chash and bhash != chash:
        return {
            "classification": "uncertain_source_diff_not_semantic_fail",
            "correctness_regression": False,
            "recorded_as_perf_win": False,
            "source_hash_used_as_semantic_fail": False,
        }
    return {
        "classification": "eligible_for_perf_compare",
        "correctness_regression": False,
        "recorded_as_perf_win": False,
        "source_hash_used_as_semantic_fail": False,
    }


def check_m01(doc: dict) -> dict:
    groups = doc.get("groups") or []
    ok = 0
    bad = []
    for g in groups:
        samples = [s for s in g.get("samples") or [] if s.get("role") == "measured"]
        valid = [s for s in samples if s.get("ok")]
        complete = len(samples) > 0 and len(valid) == len(samples) and len(valid) >= MIN_REPEATS
        if complete:
            ok += 1
        else:
            bad.append(g.get("group_id"))
    return {
        "id": "T32-M01",
        "observed": f"{ok}/{len(groups)} groups with >=10 valid repeats and 100% raw completeness",
        "verdict": "pass" if groups and not bad else "fail",
        "bad": bad,
    }


def check_m02(doc: dict) -> dict:
    missing = []
    for g in doc.get("groups") or []:
        w = g.get("work") or {}
        if not g.get("samples"):
            missing.append((g.get("group_id"), "samples"))
            continue
        if w.get("input_sha256") in (None, ""):
            missing.append((g.get("group_id"), "input_sha256"))
        has_time = any(s.get("ns") is not None for s in g["samples"])
        has_alloc = any(s.get("b_op") is not None for s in g["samples"])
        if not has_time:
            missing.append((g.get("group_id"), "time"))
        if not has_alloc:
            missing.append((g.get("group_id"), "alloc"))
        if g.get("mode") == "cold" and g.get("independent_process_peak_rss_bytes") is None:
            if not any(s.get("peak_rss_bytes") for s in g["samples"]):
                missing.append((g.get("group_id"), "rss"))
        if g.get("stage") != "parse" and w.get("bytecode_bytes", 0) == 0 and w.get("opcode_count", 0) == 0:
            if g.get("stage") not in ("parse",):
                missing.append((g.get("group_id"), "work_counts"))
        for key in ("nodes", "edges", "def_merge", "region_roots", "pending_t24_t25_t26"):
            if key not in w:
                missing.append((g.get("group_id"), key))
    return {
        "id": "T32-M02",
        "observed": f"missing={len(missing)}",
        "verdict": "pass" if not missing else "fail",
        "missing": missing[:20],
    }


def check_m03(doc: dict) -> dict:
    gates = doc.get("uncalibrated_hard_percentage_gates", 1)
    masked = bool((doc.get("semantic") or {}).get("recorded_as_perf_win"))
    claimed = doc.get("claimed_speedup_percent") is not None or doc.get("speedup_percent") is not None
    return {
        "id": "T32-M03",
        "observed": f"uncalibrated_hard_percentage_gates={gates} semantic_masked={masked} claimed_speedup={claimed}",
        "verdict": "pass" if gates == 0 and not masked and not claimed else "fail",
    }


def check_c01(doc: dict) -> list[str]:
    errors = []
    seen = set()
    parse = e2e = None
    for g in doc.get("groups") or []:
        st = g.get("stage")
        if st in FORBIDDEN_STAGE_IDS:
            errors.append(f"forbidden over-claim stage id {st} on {g.get('group_id')}")
        if g.get("family_id") != "tiny_real/reviewed" or g.get("mode") != "warm":
            continue
        seen.add(st)
        if g.get("kernel_includes_javac"):
            errors.append(f"{g.get('group_id')} kernel_includes_javac")
        if st == "parse":
            parse = g
        if st == "e2e_kernel":
            e2e = g
    for st in KERNEL_STAGES:
        if st not in seen:
            errors.append(f"missing stage {st}")
    for st, marker in HONEST_STAGE_MARKERS.items():
        if marker not in st:
            errors.append(f"stage id {st} missing marker {marker}")
        if st not in seen:
            errors.append(f"honest label {st} not present in tiny_real warm groups")
    inv = {e.get("id"): e for e in (doc.get("stage_inventory") or [])}
    for st, marker in HONEST_STAGE_MARKERS.items():
        if st not in inv:
            errors.append(f"stage_inventory missing {st}")
    if parse is None or e2e is None:
        errors.append("need parse and e2e on same class")
    elif parse.get("input_sha256") != e2e.get("input_sha256"):
        errors.append("parse/e2e input hash mismatch")
    if e2e is not None:
        jn = e2e.get("javac_ns_separate")
        if jn and e2e.get("mean_ns") == jn and jn > 1e6:
            errors.append("e2e mean equals javac time")
    if doc.get("pending_t24_t25_t26"):
        errors.append("pending_t24_t25_t26 must be false: T24 SemanticCFG, T25 sparse, T26 cache are production-backed")
    split = " ".join(doc.get("stage_split_notes") or []).lower()
    if "semanticcfg" not in split and "semantic cfg" not in split:
        errors.append("stage_split_notes must mention SemanticCFG")
    return errors


def check_c02(doc: dict) -> list[str]:
    errors = []
    if not (doc.get("collect_nonce") or "").strip():
        errors.append("missing collect_nonce; evidence may be stale committed JSON")
    if not doc.get("collected_at_unix"):
        errors.append("missing collected_at_unix")
    cold = warm = None
    for g in doc.get("groups") or []:
        if g.get("family_id") == "tiny_real/reviewed" and g.get("stage") == "e2e_kernel":
            if g.get("mode") == "cold":
                cold = g
            if g.get("mode") == "warm":
                warm = g
    if not cold or not warm:
        return errors + ["missing cold/warm e2e groups"]
    if "process=fresh" not in (cold.get("cache_key") or ""):
        errors.append(f"cold cache key {cold.get('cache_key')}")
    if "process=shared" not in (warm.get("cache_key") or ""):
        errors.append(f"warm cache key {warm.get('cache_key')}")
    if cold.get("mode") != "cold" or warm.get("mode") != "warm":
        errors.append("inaccurate labels")
    if (warm.get("valid_repeats") or 0) < MIN_REPEATS:
        errors.append("warm repeats < 10")
    if (cold.get("valid_repeats") or 0) < MIN_REPEATS:
        errors.append("cold repeats < 10")
    best = warm.get("best_trial_ns_not_used_as_result")
    if best and warm.get("stdev_ns", 0) > 0 and warm.get("mean_ns") == best:
        errors.append("mean equals best trial")
    return errors


def check_c03(doc: dict) -> list[str]:
    errors = []
    groups = {(g.get("family_id"), g.get("stage"), g.get("mode")): g for g in doc.get("groups") or []}
    for kind in SCALE_KINDS:
        keys = [(f"{kind}/{lab}", "e2e_kernel", "warm") for lab in ("N", "2N", "4N")]
        got = [groups.get(k) for k in keys]
        if any(x is None for x in got):
            errors.append(f"{kind} missing N/2N/4N")
            continue
        n, n2, n4 = got
        hashes = {n["input_sha256"], n2["input_sha256"], n4["input_sha256"]}
        if len(hashes) != 3:
            errors.append(f"{kind} hashes not unique")
        b = [n["work"].get("bytecode_bytes", 0), n2["work"].get("bytecode_bytes", 0), n4["work"].get("bytecode_bytes", 0)]
        if not (b[2] >= b[1] >= b[0] and b[0] > 0):
            errors.append(f"{kind} bytecode did not grow: {b}")
        nodes = [n["work"].get("opcode_count", 0), n2["work"].get("opcode_count", 0), n4["work"].get("opcode_count", 0)]
        if nodes[0] and nodes[2] < nodes[0]:
            errors.append(f"{kind} opcode counts shrank: {nodes}")
        w = n["work"]
        if "nodes" not in w or "edges" not in w or "def_merge" not in w or "region_roots" not in w:
            errors.append(f"{kind} missing canonical T24/T25/T26 work fields")
        if w.get("pending_t24_t25_t26"):
            errors.append(f"{kind} pending_t24_t25_t26 still true")
    return errors


def check_c04(doc: dict) -> list[str]:
    n = doc.get("noise") or {}
    errors = []
    if n.get("jitter_called_improvement"):
        errors.append("jitter called improvement")
    if n.get("jitter_verdict") == "improvement":
        errors.append("jitter verdict is improvement")
    if not n.get("clear_2x_regression_detected"):
        errors.append(f"2x not detected: {n.get('regression_verdict')}")
    base = n.get("baseline_ns") or []
    jitter = n.get("jitter_ns") or []
    reg = n.get("regression_2x_ns") or []
    if len(base) >= MIN_REPEATS and len(jitter) >= MIN_REPEATS:
        j = compare_distributions(base, jitter)
        if j["verdict"] == "improvement":
            errors.append("recompute called jitter improvement")
        if j["verdict"] == "clear_regression":
            errors.append("recompute misclassified jitter as 2x")
    if len(base) >= MIN_REPEATS and len(reg) >= MIN_REPEATS:
        r = compare_distributions(base, reg)
        if r["verdict"] != "clear_regression":
            errors.append(f"recompute 2x verdict {r['verdict']} ratio={r['ratio']:.3f}")
    return errors


def check_c05(doc: dict) -> list[str]:
    s = doc.get("semantic") or {}
    errors = []
    if s.get("recorded_as_perf_win"):
        errors.append("stub recorded as perf win")
    if not s.get("correctness_regression"):
        errors.append("stub not classified as correctness regression")
    if not (s.get("faster_stub") or {}).get("skipped_analysis"):
        errors.append("fixture did not skip analysis")
    if s.get("classification") != "correctness_regression_not_perf_win":
        errors.append(f"classification {s.get('classification')}")
    if s.get("source_hash_used_as_semantic_fail"):
        errors.append("source hash inequality used as semantic fail")
    recomputed = classify_semantic(s.get("baseline") or {}, s.get("faster_stub") or {}, s.get("oracle") or {})
    if recomputed["classification"] != "correctness_regression_not_perf_win":
        errors.append(f"recompute classification {recomputed['classification']}")
    if recomputed["source_hash_used_as_semantic_fail"]:
        errors.append("recompute used source hash as semantic fail")
    return errors


def check_c06(doc: dict, ev: Path) -> list[str]:
    errors = []
    if doc.get("schema_version") != 1:
        errors.append(f"schema {doc.get('schema_version')}")
    cmds = doc.get("commands") or []
    if not cmds:
        errors.append("no commands")
    acc = [c for c in cmds if c.get("phase") == "acceptance"]
    if not acc or not acc[0].get("argv"):
        errors.append("acceptance argv missing")
    mach = doc.get("machine") or {}
    if not mach.get("os"):
        errors.append("machine os missing")
    note = mach.get("machine_difference_note") or ""
    if "difference" not in note.lower():
        errors.append("machine differences not explicit")
    fams = json.loads((ev / "families.json").read_text(encoding="utf-8"))
    for f in fams:
        p = ev / "families" / (f["id"].replace("/", "_") + ".class")
        if not p.is_file():
            errors.append(f"missing family bytes {f['id']}")
            continue
        import hashlib
        h = hashlib.sha256(p.read_bytes()).hexdigest()
        if h != f["sha256"]:
            errors.append(f"replay hash mismatch {f['id']}")
    return errors


def main() -> int:
    repo = Path(__file__).resolve().parents[2]
    ev = evidence_dir(repo)
    doc = load_summary(ev)
    report = {
        "T32-M01": check_m01(doc),
        "T32-M02": check_m02(doc),
        "T32-M03": check_m03(doc),
        "T32-C01": check_c01(doc),
        "T32-C02": check_c02(doc),
        "T32-C03": check_c03(doc),
        "T32-C04": check_c04(doc),
        "T32-C05": check_c05(doc),
        "T32-C06": check_c06(doc, ev),
    }
    print(json.dumps(report, indent=2))
    bad = []
    for k, v in report.items():
        if k.startswith("T32-M") and v.get("verdict") != "pass":
            bad.append(k)
        if k.startswith("T32-C") and v:
            bad.append(k)
    return 1 if bad else 0


if __name__ == "__main__":
    raise SystemExit(main())
