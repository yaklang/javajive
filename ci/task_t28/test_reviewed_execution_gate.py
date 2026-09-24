"""Reviewed delta: malformed metadata, staging limits and positive cleanup gates."""
import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from test_classfile_metadata import fixture
from test_t28_untrusted_gate import _obs
from tools.generators_holdout.pipeline import run_pipeline_from_class_bytes, _isolation_failure_class
from tools.sandbox_worker.untrusted_exec import _rel_files, run_untrusted_class_dir, StagingError


class TestReviewedExecutionGate(unittest.TestCase):
    def test_malformed_metadata_never_reaches_worker(self):
        with tempfile.TemporaryDirectory() as raw, mock.patch("tools.generators_holdout.pipeline.verify_and_run") as run:
            result = run_pipeline_from_class_bytes("public class Foo {}", bytes.fromhex("cafebabe0000003400020700010000000100000000000000000000"), None, Path(raw), class_name="Foo")
            self.assertEqual(result.failure_class, "invalid_input")
            run.assert_not_called()

    def test_absent_cleanup_proof_cannot_be_success(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "Main.class").write_bytes(b"fixture")
            for extra in ({"leftover_verified": False}, {"leftover_verified": None}):
                worker = mock.Mock()
                worker.run.return_value = _obs(extra=extra)
                result = run_untrusted_class_dir(root, "Main", worker=worker)
                self.assertEqual(result["stage"], "leftover_query_failed", result)
                self.assertFalse(result["verified_and_ran"])
            worker.run.return_value = _obs()
            self.assertTrue(run_untrusted_class_dir(root, "Main", worker=worker)["verified_and_ran"])

    def test_pipeline_requires_positive_execution_proof(self):
        for ran in (
            {"stage": "run_ok", "verified_and_ran": False},
            {"stage": "run_ok", "verified_and_ran": True, "host_java": False, "did_execute": True},
            {"stage": "run_fail", "leftover_containers": ["leaked"]},
        ):
            self.assertEqual(_isolation_failure_class(ran), "infra_error")

    def test_invalid_rebuilt_source_is_behavior_not_invalid_fixture(self):
        # Synthetic observations test classification only, not Java equivalence.
        observed = {"stage": "run_ok", "rc": 0, "stdout": "7\n", "verified_and_ran": True,
            "host_java": False, "did_execute": True, "leftover_verified": True}
        with tempfile.TemporaryDirectory() as raw, \
             mock.patch("tools.generators_holdout.pipeline.verify_and_run", return_value=observed), \
             mock.patch("tools.generators_holdout.pipeline.java_command", return_value="java"), \
             mock.patch("tools.generators_holdout.pipeline.decompile_class", return_value={
                 "rc": 0, "status": "complete", "source": "public class 1Bad {}"}):
            result = run_pipeline_from_class_bytes("package demo; public class Foo {}", fixture(),
                None, Path(raw), class_name="demo.Foo", trusted=False)
        self.assertEqual(result.failure_class, "behavior")
        self.assertEqual(result.stages["rebuild_compile"].status, "invalid_source")

    def test_dependency_cannot_supply_missing_rebuilt_main(self):
        observed = {"stage": "run_ok", "rc": 0, "stdout": "7\n", "verified_and_ran": True,
            "host_java": False, "did_execute": True, "leftover_verified": True}
        with tempfile.TemporaryDirectory() as raw, \
             mock.patch("tools.generators_holdout.pipeline.verify_and_run", return_value=observed) as run, \
             mock.patch("tools.generators_holdout.pipeline.java_command", return_value="java"), \
             mock.patch("tools.generators_holdout.pipeline.compile_sources", return_value={"rc": 0}), \
             mock.patch("tools.generators_holdout.pipeline.decompile_class", return_value={
                 "rc": 0, "status": "complete", "source": "package demo; public class Foo {}"}):
            result = run_pipeline_from_class_bytes("package demo; public class Foo {}", fixture(),
                None, Path(raw), class_name="demo.Foo", trusted=False)
        self.assertEqual(result.failure_class, "behavior")
        self.assertEqual(result.stages["rebuild_compile"].status, "missing_class")
        self.assertEqual(run.call_count, 1, "missing rebuilt class must not reach a worker")

    def test_staging_has_disjoint_roots_and_pre_read_limits(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            app, lib = root / "app", root / "lib"
            (app / "extra0" / "pkg").mkdir(parents=True)
            (lib / "pkg").mkdir(parents=True)
            (app / "extra0" / "pkg" / "X.class").write_bytes(b"APP")
            (lib / "pkg" / "X.class").write_bytes(b"LIB")
            files, cp = _rel_files(app, [lib])
            self.assertEqual(files["main/extra0/pkg/X.class"], b"APP")
            self.assertEqual(files["extra0/pkg/X.class"], b"LIB")
            self.assertEqual(cp, ["/inputs/main", "/inputs/extra0"])
            for limits in ({"max_bytes": 5}, {"max_files": 1}):
                with self.assertRaises(StagingError) as caught:
                    _rel_files(app, [lib], **limits)
                self.assertEqual(caught.exception.stage, "resource_limit")

    def test_nonregular_input_and_option_name_do_not_start_worker(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            os.mkfifo(root / "Blocked.class")
            worker = mock.Mock()
            result = run_untrusted_class_dir(root, "Main", worker=worker)
            self.assertEqual(result["stage"], "invalid_input")
            worker.run.assert_not_called()
            result = run_untrusted_class_dir(root, "-version", worker=worker)
            self.assertEqual(result["stage"], "invalid_input")
            worker.run.assert_not_called()


if __name__ == "__main__":
    unittest.main()
