"""Regression tests for stable historical-JAR compiler diagnostic keys."""

from __future__ import annotations

import importlib.util
from pathlib import Path
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "historical_jar_audit", ROOT / "scripts/historical_jar_audit.py"
)
assert SPEC is not None and SPEC.loader is not None
HISTORICAL_AUDIT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(HISTORICAL_AUDIT)


class HistoricalDiagnosticKeys(unittest.TestCase):
    def test_capture_copy_ordinal_does_not_create_a_false_regression(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            log = Path(tmp) / "javac.log"
            log.write_text(
                "/sources/p/C.java:10: error: no suitable method for f(var2_f4)\n"
                "/sources/p/C.java:20: error: no suitable method for f(var2_f7)\n"
            )
            errors = HISTORICAL_AUDIT.compiler_errors(Path(tmp))
        self.assertEqual(errors[("p/C.java", "no suitable method for f(var2_f)")], 2)

    def test_real_source_local_name_remains_part_of_diagnostic(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            log = Path(tmp) / "javac.log"
            log.write_text(
                "/sources/p/C.java:10: error: cannot find symbol var2\n"
                "/sources/p/C.java:20: error: cannot find symbol var3\n"
            )
            errors = HISTORICAL_AUDIT.compiler_errors(Path(tmp))
        self.assertEqual(errors[("p/C.java", "cannot find symbol var2")], 1)
        self.assertEqual(errors[("p/C.java", "cannot find symbol var3")], 1)


class HistoricalWorkerObservation(unittest.TestCase):
    def test_isolation_completion_and_input_identity_are_independent(self):
        import hashlib
        import json
        from types import SimpleNamespace
        from unittest.mock import patch

        for scenario in ("ok", "backend", "timeout", "capped", "failed", "missing", "incomplete", "wrong hash", "wrong revision", "input changed", "dependency changed"):
            with self.subTest(scenario=scenario), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                cache = (root / "m2").resolve()
                cache.mkdir()
                data = b"pinned test artifact"
                (cache / "one.jar").write_bytes(data)
                digest = hashlib.sha256(data).hexdigest()
                lock = {"artifacts": {"one.jar": digest}, "jars": {"one": {"path": "one.jar", "deps": []}}}
                lock_path = root / "lock.json"
                lock_path.write_text(json.dumps(lock))
                manifest = {"artifactRoot": str(cache), "artifacts": lock["artifacts"], "jars": {
                    "one": {"path": str(cache / "one.jar"), "sha256": digest, "deps": []}}}
                if scenario == "dependency changed":
                    manifest["jars"]["one"]["deps"] = ["/ambient.jar"]
                manifest_path = root / "manifest.json"
                manifest_path.write_text(json.dumps(manifest))
                if scenario == "input changed":
                    (cache / "one.jar").write_bytes(b"changed")
                binary = root / "audit.test"
                binary.write_bytes(b"trusted test executable")
                artifact = root / "artifacts/result/one"
                artifact.mkdir(parents=True)
                if scenario != "missing":
                    (artifact / "observation.json").write_text(json.dumps({
                        "Revision": "2"*40 if scenario == "wrong revision" else "1"*40,
                        "Completed": scenario != "incomplete", "InputSHA256": "wrong" if scenario == "wrong hash" else digest}))
                observation = SimpleNamespace(status="ok", backend="docker", reason="", exit_code=1 if scenario == "failed" else 0,
                    timed_out=scenario == "timeout", output_capped=scenario == "capped", policy_digest="policy",
                    extra={"artifact_root": str(root / "artifacts")}, stdout=b"log", stderr=b"")
                job_seen = []
                worker = SimpleNamespace(backend="none" if scenario == "backend" else "docker",
                    run=lambda job: job_seen.append(job) or observation)
                args = SimpleNamespace(manifest=manifest_path, binary=binary, report=root / "candidate", revision="1"*40)
                with patch.object(HISTORICAL_AUDIT, "LOCK", lock_path), patch("tools.sandbox_worker.executor.SandboxWorker", return_value=worker):
                    if scenario == "ok":
                        self.assertEqual(HISTORICAL_AUDIT.observe(args), 0)
                        self.assertTrue((args.report / "one/observation.json").exists())
                        self.assertEqual(job_seen[0].env["HISTORICAL_JAR_REVISION"], args.revision)
                        self.assertFalse(job_seen[0].allow_network)
                        self.assertEqual(json.loads(job_seen[0].input_files["manifest.json"])["jars"]["one"]["path"], "/inputs/one.jar")
                        self.assertIn(b"-test.timeout=40m", job_seen[0].input_files["run.sh"])
                    else:
                        with self.assertRaises(ValueError):
                            HISTORICAL_AUDIT.observe(args)
                        self.assertFalse(args.report.exists())
                        if scenario in ("backend", "input changed", "dependency changed"):
                            self.assertEqual(job_seen, [])

if __name__ == "__main__":
    unittest.main()
