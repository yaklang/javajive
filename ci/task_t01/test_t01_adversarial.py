"""Adversarial extras: empty, skip, compiler --release rejection, historical JAR Completed≠pass."""

from __future__ import annotations

import unittest

from common import PR_BASE, ROOT, digest, eight_api_pass, observation_dict, obs
from tools.milestone_ledger import aggregate, parse_observation
from tools.milestone_ledger.constants import HISTORICAL_JAR_RAW_FIELDS
from tools.milestone_ledger.errors import ObservationError
from tools.milestone_ledger.observation import wrap_historical_jar_dict
from tools.milestone_ledger.toolchain import normalize_compiler_version


class TestTaskT01AdversarialProtocol(unittest.TestCase):
    def test_T01_C01_skip_is_not_pass(self) -> None:
        rows, manifest = eight_api_pass()
        bad = observation_dict(sample="CaseA", mode="precision", debug="debug", status="pass", skip=True)
        result = aggregate(manifest, [bad] + [r.to_dict() for r in rows[1:]], strict=False)
        self.assertFalse(result.ok, "T01-C01")
        self.assertNotIn("CaseA::api::precision::debug", result.pass_ids, "T01-C01 skip is not pass")

    def test_T01_C01_empty_observation_object_fails(self) -> None:
        rows, manifest = eight_api_pass()
        result = aggregate(manifest, [{}] + [r.to_dict() for r in rows[1:]], strict=False)
        self.assertFalse(result.ok, "T01-C01")
        self.assertTrue(any("EMPTY" in err or "MISSING" in err for err in result.errors), "T01-C01")

    def test_compiler_version_rejects_release_flag(self) -> None:
        with self.assertRaises(ObservationError):
            normalize_compiler_version("--release 8")
        with self.assertRaises(ObservationError):
            normalize_compiler_version("8")
        self.assertEqual(normalize_compiler_version("javac 21.0.11\n"), "javac 21.0.11")
        self.assertEqual(normalize_compiler_version("javac 21.0.11"), "javac 21.0.11")

    def test_historical_jar_completed_is_not_pass(self) -> None:
        raw = {
            "Jar": "commons-io",
            "Revision": PR_BASE,
            "InputSHA256": digest(b"jar-bytes"),
            "Compiler": "javac 21.0.11",
            "Completed": True,
            "CompileSucceeded": True,
            "CompilerErrors": 0,
            "StubMethods": 0,
            "DecompileFailures": 0,
            "InputClasses": 1,
            "SourceUnits": 1,
            "RebuiltVerifyOK": 1,
            "RebuiltVerifyFail": 0,
            "RebuiltClasses": ["a.class"],
            "StubUnits": {},
            "SourceHashes": {"a.java": digest(b"src")},
            "CompilePasses": [],
            "Seconds": 1.0,
            "OriginalVerifyOK": 1,
            "OriginalVerifyFail": 0,
        }
        wrapped = parse_observation(
            wrap_historical_jar_dict(
                raw,
                evidence_defaults={
                    "dependency_lock_digest": digest(b"lock"),
                    "harness_digest": digest(b"harness"),
                    "release": 8,
                },
            )
        )
        self.assertNotEqual(wrapped.status, "pass")
        self.assertEqual(wrapped.status, "observe")
        for field in ("Completed", "InputSHA256", "Compiler", "CompileSucceeded"):
            self.assertEqual(wrapped.historical_raw[field], raw[field])
        incomplete = dict(raw)
        incomplete["Completed"] = False
        broken = parse_observation(
            wrap_historical_jar_dict(
                incomplete,
                evidence_defaults={
                    "dependency_lock_digest": digest(b"lock"),
                    "harness_digest": digest(b"harness"),
                },
            )
        )
        self.assertEqual(broken.status, "infra_error")

    def test_production_historical_jar_fields_present_in_audit_script(self) -> None:
        text = (ROOT / "scripts" / "historical_jar_audit.py").read_text(encoding="utf-8")
        for field in ("InputSHA256", "Compiler", "Completed", "CompileSucceeded"):
            self.assertIn(field, text)
            self.assertIn(field, HISTORICAL_JAR_RAW_FIELDS)

    def test_fail_unsupported_infra_never_semantic_pass(self) -> None:
        for status in ("fail", "unsupported", "infra_error"):
            item = obs(sample="Zed", status=status, debug="nodebug")
            self.assertNotEqual(item.status, "pass")
            from tools.milestone_ledger.observation import is_semantic_pass

            self.assertFalse(is_semantic_pass(item), status)


if __name__ == "__main__":
    unittest.main()
