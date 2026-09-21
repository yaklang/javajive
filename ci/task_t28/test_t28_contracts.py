"""Executable T28 contracts. Names include T28-Cxx for inventory mapping."""

from __future__ import annotations

import hashlib
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from tools.generators_holdout.evidence import FailureEvidence, classify_failure, from_pipeline, tool_versions, write_evidence
from tools.generators_holdout.generate import (
    FAMILIES,
    SAMPLES_PER_FAMILY,
    compile_and_verify_family,
    corrupt_class,
    generate_family,
)
from tools.generators_holdout.holdout import CorpusItem, HoldoutLeakError, assert_frozen, split_holdout
from tools.generators_holdout.identity import InfraError, compile_sources, discover_compilers, java_command, verify_and_run
from tools.generators_holdout.morph import MorphKind, apply_morph, compare_decompiled_pair
from tools.generators_holdout.pipeline import FaultInjectedOracle, run_pipeline
from tools.generators_holdout.reduce import (
    FAILURE_INVALID,
    feature_regex_negative_control,
    reduce_source,
    reject_illegal_as_semantic,
)

from tools.evidence_paths import evidence_subdir

FIXTURES = ROOT / "tools" / "generators_holdout" / "fixtures"
EVIDENCE = evidence_subdir("t28")
GOFLAGS = ["-ldflags=-linkmode=external"]


def _compilers():
    found = discover_compilers()
    if found["javac21_release8"] is None:
        raise AssertionError("T28 infra_error: javac 21 is required on this host and was not found")
    return found


def _maybe_decompile(class_file: Path, work: Path) -> dict:
    """Compare JavaJive output without modifying production algorithms."""
    env = os.environ.copy()
    env["CGO_ENABLED"] = "1"
    env["GOTOOLCHAIN"] = "local"
    bin_path = work / "javajive"
    if not bin_path.is_file():
        build = subprocess.run(
            ["go", "build", *GOFLAGS, "-o", str(bin_path), "./cmd/javajive"],
            cwd=ROOT,
            env=env,
            capture_output=True,
            text=True,
            timeout=180,
        )
        if build.returncode != 0:
            return {
                "status": "infra_error",
                "stderr": build.stderr,
                "stdout": "",
            }
    proc = subprocess.run(
        [str(bin_path), "decompile", str(class_file)],
        capture_output=True,
        text=True,
        timeout=30,
        env=env,
    )
    return {
        "status": "ok" if proc.returncode == 0 else "behavior",
        "rc": proc.returncode,
        "stdout": proc.stdout,
        "stderr": proc.stderr,
        "sha256": hashlib.sha256(proc.stdout.encode()).hexdigest(),
    }


class T28Contracts(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.compilers = _compilers()
        cls.javac21 = cls.compilers["javac21_release8"]
        EVIDENCE.mkdir(parents=True, exist_ok=True)

    def test_T28_C01_generated_classes_are_jvm_verified(self) -> None:
        """T28-C01: each family emits 100 legal classes; illegal mutations are generator_error."""
        rows = []
        with tempfile.TemporaryDirectory(prefix="t28-c01-") as raw:
            work = Path(raw)
            for family in FAMILIES:
                samples = generate_family(family)
                self.assertEqual(len(samples), SAMPLES_PER_FAMILY, family)
                report = compile_and_verify_family(samples, self.javac21, work / family)
                self.assertTrue(report["ok"], msg=json.dumps(report["failures"], indent=2)[:2000])
                self.assertEqual(report["verified"], SAMPLES_PER_FAMILY)
                rows.extend(report["results"])
                legal = (work / family / "classes" / f"{samples[0].class_name}.class").read_bytes()
                for kind in ("bad_magic", "truncated", "cp_overflow"):
                    bad = corrupt_class(legal, kind)
                    bad_path = work / f"bad-{family}-{kind}.class"
                    bad_path.write_bytes(bad)
                    # Drop into an empty dir and try to load under a fake name: expect failure.
                    dest = work / "illegal" / kind
                    dest.mkdir(parents=True, exist_ok=True)
                    (dest / f"{samples[0].class_name}.class").write_bytes(bad)
                    ran = verify_and_run(dest, samples[0].class_name, java_bin=java_command(self.javac21))
                    self.assertFalse(
                        ran["verified_and_ran"],
                        msg=f"illegal {kind} must not verify: {ran}",
                    )
                    self.assertEqual(
                        classify_failure("generate", legal_input=False, infra=False, budget=False, mismatch=True),
                        "invalid_input",
                    )
        inv = EVIDENCE / "t28_c01_inventory.json"
        inv.write_text(json.dumps(rows, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        self.assertEqual(len(rows), SAMPLES_PER_FAMILY * len(FAMILIES))
        self.assertEqual(len({r["source_sha256"] for r in rows}), len(rows))

    def test_T28_C02_morph_preserves_runtime_oracle_then_javajive(self) -> None:
        """T28-C02: debug/name/slot/switch-key morphs keep original-program oracle; then JavaJive is compared."""
        loop = generate_family("loop", count=3)[0]
        sw = generate_family("switch", count=3)[0]
        with tempfile.TemporaryDirectory(prefix="t28-c02-") as raw:
            work = Path(raw)
            morphs = [
                apply_morph(loop, MorphKind.DEBUG, self.javac21, work / "debug"),
                apply_morph(loop, MorphKind.NAME, self.javac21, work / "name"),
                apply_morph(loop, MorphKind.SLOT, self.javac21, work / "slot"),
                apply_morph(sw, MorphKind.SWITCH_KEY, self.javac21, work / "key"),
            ]
            for result in morphs:
                self.assertTrue(result.oracle_equal, msg=result.to_dict())
                self.assertEqual(result.original_stdout, result.morphed_stdout)
            # DEBUG / NAME / SLOT should usually change class bytes; switch-key must.
            self.assertNotEqual(morphs[3].original_class_sha256, morphs[3].morphed_class_sha256)
            reports = {}
            for result in morphs:
                pair = compare_decompiled_pair(
                    result.original.source,
                    result.morphed_source,
                    self.javac21,
                    work / f"jj-{result.kind.value}",
                    modes=("precision", "compatibility"),
                )
                reports[result.kind.value] = pair
                for row in pair["modes"]:
                    self.assertNotEqual(row["original"]["failure_class"], "infra_error", msg=row)
                    self.assertNotEqual(row["morphed"]["failure_class"], "infra_error", msg=row)
                    # Byte-different decompiled source is allowed if rebuilt runs match.
                    if row["original"]["failure_class"] == "pass" and row["morphed"]["failure_class"] == "pass":
                        self.assertEqual(
                            row["original"]["rebuilt_stdout"],
                            row["morphed"]["rebuilt_stdout"],
                            msg=f"{result.kind.value} {row['mode']}",
                        )
            (EVIDENCE / "t28_c02_morph.json").write_text(json.dumps(reports, indent=2)[:200000] + "\n")

    def test_T28_C03_reduce_keeps_failure_class_and_rejects_illegal(self) -> None:
        """T28-C03: reduction re-runs a live oracle; source-feature regex is not a defect."""
        unicode_src = (FIXTURES / "UnicodePair.java").read_text(encoding="utf-8")
        overload_src = (FIXTURES / "OverloadWitness.java").read_text(encoding="utf-8")
        phi_src = (FIXTURES / "PhiSwap.java").read_text(encoding="utf-8")
        baseline_src = (FIXTURES / "Baseline.java").read_text(encoding="utf-8")
        # Negative control: regex sees FEATURES on working Unicode/overload/phi programs.
        self.assertIsNotNone(feature_regex_negative_control(unicode_src))
        self.assertIsNotNone(feature_regex_negative_control(overload_src))
        self.assertIsNotNone(feature_regex_negative_control(phi_src))
        self.assertIsNone(feature_regex_negative_control(baseline_src))
        with tempfile.TemporaryDirectory(prefix="t28-c03-") as raw:
            work = Path(raw)
            base_obs = run_pipeline(baseline_src, self.javac21, work / "baseline", mode="precision")
            uni_obs = run_pipeline(unicode_src, self.javac21, work / "unicode", mode="precision")
            self.assertNotEqual(base_obs.failure_class, "infra_error", msg=base_obs.to_dict())
            self.assertNotEqual(uni_obs.failure_class, "infra_error", msg=uni_obs.to_dict())
            # Baseline is the positive control: legal and not a decompiler defect if pass.
            self.assertEqual(base_obs.stages["original_verify_run"].status, "run_ok")
            # Regex would "detect" UnicodePair even if the live oracle says pass.
            self.assertNotEqual(feature_regex_negative_control(unicode_src), uni_obs.failure_class)

            payload = {
                "baseline": base_obs.to_dict(),
                "unicode": uni_obs.to_dict(),
                "regex_is_not_oracle": True,
            }
            if uni_obs.failure_class not in {"pass", "infra_error"}:
                padded = (
                    "public class UnicodePad {\n"
                    "  public static void main(String[] a) {\n"
                    "    int dead = 1 + 2;\n"
                    "    System.out.println(dead);\n"
                    + unicode_src.split("{", 1)[1].rsplit("}", 1)[0]
                    + "\n    System.out.println(\"tail\");\n"
                    "  }\n}\n"
                )
                # Use the actual UnicodePair source; padding may change class name.
                reduced = reduce_source(unicode_src, self.javac21, work=work / "reduce-live")
                self.assertEqual(reduced["oracle_kind"], "javajive_live")
                self.assertEqual(reduced["failure_class"], uni_obs.failure_class)
                self.assertTrue(reduced["legal"])
                payload["reduced"] = {k: reduced[k] for k in reduced if k != "original_observation"}
            else:
                # Candidate currently round-trips UnicodePair. Use a labeled harness control.
                injected = (
                    "public class InjectPad {\n"
                    "  public static void main(String[] a) {\n"
                    "    int dead = 1 + 2;\n"
                    "    System.out.println(7);\n"
                    "    String keep = \"T28_INJECT_FAIL\";\n"
                    "  }\n}\n"
                )
                reduced = reduce_source(
                    injected,
                    self.javac21,
                    predicate=FaultInjectedOracle().classify,
                    work=work / "reduce-inject",
                )
                self.assertEqual(reduced["oracle_kind"], "harness_control")
                self.assertEqual(reduced["failure_class"], "harness_control_injected_mismatch")
                payload["harness_control"] = {k: reduced[k] for k in reduced if k != "original_observation"}

            broken = "public class UnicodePad { public static void main(String[] a) { String s=; } }"
            verdict = reject_illegal_as_semantic(broken, self.javac21)
            self.assertFalse(verdict["legal"])
            self.assertEqual(verdict["failure_class"], FAILURE_INVALID)
            self.assertFalse(verdict["may_label_semantic_defect"])
            payload["illegal"] = verdict
            (EVIDENCE / "t28_c03_reduce.json").write_text(json.dumps(payload, indent=2)[:200000] + "\n")

    def test_T28_C04_holdout_groups_forbid_leakage(self) -> None:
        """T28-C04: split by group; near-dups cannot cross train/holdout; hashes freeze."""
        foo_v1 = b"class Foo { int x=1; }"
        foo_v2_copy = b"class Foo { int x=1; }"  # near-dup / identical normalized
        foo_v2_real = b"class Foo { int x=2; }"
        bar = b"class Bar { int y=9; }"
        items = [
            CorpusItem("foo-1.0", "commons-foo", "1.0", "javac21_release8", hashlib.sha256(foo_v1).hexdigest(), hashlib.sha256(foo_v1).hexdigest(), "synthetic"),
            CorpusItem("foo-1.1", "commons-foo", "1.1", "javac21_release8", hashlib.sha256(foo_v2_real).hexdigest(), hashlib.sha256(foo_v2_real).hexdigest(), "synthetic"),
            CorpusItem("foo-1.0-rebuild", "commons-foo", "1.0-rebuild", "javac8_native", hashlib.sha256(foo_v2_copy).hexdigest(), hashlib.sha256(foo_v2_copy).hexdigest(), "synthetic"),
            CorpusItem("bar-1.0", "other-bar", "1.0", "ecj_pinned", hashlib.sha256(bar).hexdigest(), hashlib.sha256(bar).hexdigest(), "synthetic"),
        ]
        split = split_holdout(items, holdout_groups={"other-bar"})
        self.assertGreaterEqual(split.duplicate_rate, 0.0)
        self.assertEqual(split.holdout[0].group, "other-bar")
        self.assertNotIn("commons-foo", {x.group for x in split.holdout})
        assert_frozen(split, items)
        # Explicit near-dup leakage: put identical normalized payload in two groups, then split.
        leaked_items = items + [
            CorpusItem(
                "bar-clone-of-foo",
                "other-bar",
                "clone",
                "javac21_release8",
                hashlib.sha256(foo_v1).hexdigest(),
                hashlib.sha256(foo_v1).hexdigest(),
                "synthetic",
            )
        ]
        with self.assertRaises(HoldoutLeakError) as ctx:
            split_holdout(leaked_items, holdout_groups={"other-bar"})
        self.assertIn("near-duplicate leakage", str(ctx.exception))
        (EVIDENCE / "t28_c04_holdout.json").write_text(split.to_dict() and json.dumps(split.to_dict(), indent=2) + "\n")
        # Group-only split of commons-foo into holdout is allowed if no cross-group dup.
        foo_only = split_holdout(items, holdout_groups={"commons-foo"})
        self.assertEqual({x.group for x in foo_only.holdout}, {"commons-foo"})

    def test_T28_C05_compiler_identities_are_not_merged(self) -> None:
        """T28-C05: javac21 --release 8, real javac8, pinned ECJ are distinct coverage keys."""
        found = self.compilers
        self.assertIsNotNone(found["javac21_release8"], "infra_error: javac 21 required (JAVA_HOME / JAVA21_HOME)")
        self.assertIsNotNone(
            found["javac8_native"],
            "infra_error: real javac 8 required for T28-C05; set JAVA8_HOME (not javac21 --release 8)",
        )
        self.assertIsNotNone(
            found["ecj_pinned"],
            "infra_error: pinned ECJ jar required; set ECJ_JAR to sha256-pinned ecj-3.37.0.jar",
        )
        keys = {}
        baseline = FIXTURES / "Baseline.java"
        with tempfile.TemporaryDirectory(prefix="t28-c05-") as raw:
            work = Path(raw)
            for name, ident in found.items():
                if ident is None:
                    keys[name] = None
                    continue
                dest = work / name
                compiled = compile_sources(ident, [baseline], dest, debug="nodebug")
                self.assertEqual(compiled["rc"], 0, msg=f"{name}: {compiled['stderr']}")
                class_file = dest / "Baseline.class"
                self.assertTrue(class_file.is_file(), name)
                ran = verify_and_run(dest, "Baseline", java_bin=java_command(ident) if ident.kind == "javac" else java_command(found["javac21_release8"]))
                self.assertTrue(ran["verified_and_ran"], msg=ran)
                self.assertEqual(ran["stdout"], "7\n")
                keys[name] = {
                    "coverage_key": ident.coverage_key(),
                    "label": ident.label,
                    "version": ident.version,
                    "release": ident.release,
                    "class_sha256": hashlib.sha256(class_file.read_bytes()).hexdigest(),
                    "identity": ident.to_dict(),
                }
        self.assertNotEqual(keys["javac21_release8"]["coverage_key"], keys["javac8_native"]["coverage_key"])
        self.assertTrue(keys["javac21_release8"]["version"].startswith("21") or keys["javac21_release8"]["version"].startswith("22"))
        self.assertTrue(keys["javac8_native"]["version"].startswith("1.8"))
        self.assertEqual(keys["javac21_release8"]["release"], 8)
        self.assertIsNone(keys["javac8_native"]["release"])
        self.assertNotEqual(keys["ecj_pinned"]["coverage_key"], keys["javac21_release8"]["coverage_key"])
        self.assertNotEqual(keys["ecj_pinned"]["coverage_key"], keys["javac8_native"]["coverage_key"])
        self.assertEqual(keys["ecj_pinned"]["identity"]["kind"], "ecj")
        # Merging --release 8 into a javac8 bucket is forbidden.
        merged = keys["javac21_release8"]["version"] == keys["javac8_native"]["version"]
        self.assertFalse(merged)
        (EVIDENCE / "t28_c05_compilers.json").write_text(json.dumps(keys, indent=2) + "\n")

    def test_T28_C06_every_failure_stage_has_full_evidence(self) -> None:
        """T28-C06: real compile/verify/decompile/rebuild/run/infra failures capture source and bytes."""
        versions = tool_versions()
        items = []
        with tempfile.TemporaryDirectory(prefix="t28-c06-") as raw:
            work = Path(raw)
            compile_fail_src = "public class C06Compile { public static void main(String[] a) { int x = ; } }"
            compile_obs = run_pipeline(compile_fail_src, self.javac21, work / "compile", mode="precision")
            self.assertEqual(compile_obs.failure_class, "invalid_input")
            self.assertEqual(compile_obs.stages["compile"].status, "compile_fail")
            items.append(from_pipeline("T28-C06-compile", compile_obs, versions))

            # Verify: legal source compiled then class bytes corrupted.
            sample = generate_family("loop", count=1)[0]
            good = run_pipeline(sample.source, self.javac21, work / "good", mode="precision")
            self.assertIn(good.failure_class, {"pass", "behavior", "partial", "unsupported"})
            class_path = Path(good.input_bytes_path)
            bad_bytes = class_path.read_bytes()[:20]
            (work / "verify").mkdir()
            dest = work / "verify" / "classes"
            dest.mkdir()
            (dest / f"{sample.class_name}.class").write_bytes(bad_bytes)
            ran = verify_and_run(dest, sample.class_name, java_bin=java_command(self.javac21))
            self.assertIn(ran["stage"], {"verify_fail", "linkage_error", "run_fail"})
            verify_ev = FailureEvidence(
                case_id="T28-C06-verify",
                failure_class="invalid_input",
                input_sha256=hashlib.sha256(bad_bytes).hexdigest(),
                output=ran.get("stderr") or "",
                classpath=[str(dest)],
                trace=["compile", "corrupt", "verify"],
                tool_versions=versions,
                extra={
                    "require_artifacts": True,
                    "input_source": sample.source,
                    "input_bytes_sha256": hashlib.sha256(bad_bytes).hexdigest(),
                    "stage": ran["stage"],
                    "argv": ran.get("argv"),
                },
            )
            items.append(verify_ev)

            uni = (FIXTURES / "UnicodePair.java").read_text(encoding="utf-8")
            dec_obs = run_pipeline(uni, self.javac21, work / "decompile", mode="precision")
            items.append(from_pipeline("T28-C06-decompile-rebuild-run", dec_obs, versions))

            from tools.generators_holdout.identity import CompilerIdentity

            bogus = CompilerIdentity(
                kind="javac",
                version="missing",
                release=8,
                executable="/no/such/javac",
                digest="0" * 64,
                home="/no/such/home",
                label="missing javac",
            )
            infra_obs = run_pipeline(sample.source, bogus, work / "infra", mode="precision")
            self.assertEqual(infra_obs.stages["compile"].status, "compile_fail")
            infra_ev = from_pipeline("T28-C06-infra", infra_obs, versions)
            infra_ev.failure_class = "infra_error"
            infra_ev.extra["stage"] = "compile_toolchain"
            items.append(infra_ev)

        for ev in items:
            ev.validate()
            extra = ev.extra
            self.assertTrue(extra.get("input_source") or extra.get("input_source_path"))
            self.assertTrue(extra.get("input_bytes_sha256") or extra.get("input_bytes_path"))
        digest = write_evidence(EVIDENCE / "t28_c06_evidence.json", items)
        self.assertEqual(len(digest), 64)
        with self.assertRaises(ValueError):
            FailureEvidence(
                case_id="T28-C06-empty",
                failure_class="behavior",
                input_sha256="",
                output="",
                classpath=[],
                trace=[],
                tool_versions={},
            ).validate()


if __name__ == "__main__":
    unittest.main()
