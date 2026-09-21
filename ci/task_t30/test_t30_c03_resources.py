"""T30-C03: small budget limits fork/output/memory; leftover cleanup must fail the observation."""

from __future__ import annotations

import subprocess
import tempfile
import unittest
import uuid
from pathlib import Path
from unittest.mock import patch

from harness import obs_dict, worker, write_evidence
from tools.sandbox_worker.backends.docker import image_exists
from tools.sandbox_worker.constants import ALPINE_IMAGE, LABEL_ID, LABEL_JOB, STATUS_OK, TOOLCHAIN_IMAGE
from tools.sandbox_worker.executor import apply_leftover_status
from tools.sandbox_worker.job import UntrustedJob
from tools.sandbox_worker.policy import Limits
from tools.sandbox_worker.procutil import (
    LeftoverQueryError,
    cleanup_job_containers,
    docker_leftovers,
    force_rm_containers,
    snapshot_pids,
)


def _assert_leftovers_fail_status(test: unittest.TestCase, obs) -> None:
    if obs.leftover_host_pids or obs.leftover_containers or obs.extra.get("leftover_query_failed"):
        test.assertNotEqual(obs.status, STATUS_OK)
        test.assertNotEqual(obs.status, "ok")


def _assert_cleanup_verified(test: unittest.TestCase, obs) -> None:
    test.assertFalse(obs.extra.get("leftover_query_failed"), obs.extra)
    test.assertEqual(obs.leftover_host_pids, [])
    test.assertEqual(obs.leftover_containers, [])
    if obs.backend in {"docker", "podman"}:
        test.assertTrue(obs.extra.get("leftover_verified"), obs.extra)
    _assert_leftovers_fail_status(test, obs)


def _write_fake_engine(dirpath: Path, body: str) -> Path:
    path = Path(dirpath) / "docker"
    path.write_text("#!/bin/sh\n" + body, encoding="utf-8")
    path.chmod(0o755)
    return path


def _hard_limits(**kwargs) -> Limits:
    base = dict(
        timeout_seconds=8,
        memory_bytes=32 * 1024 * 1024,
        pids=16,
        output_bytes=8192,
        artifact_bytes=8192,
        fsize_bytes=8192,
    )
    base.update(kwargs)
    return Limits(**base)


class TestTaskT30C03Resources(unittest.TestCase):
    def test_T30_C03_limits_or_kill_and_no_leftover_children(self) -> None:
        w = worker()
        limits = Limits(
            timeout_seconds=6,
            memory_bytes=32 * 1024 * 1024,
            pids=8,
            output_bytes=16 * 1024,
            fsize_bytes=32 * 1024,
            artifact_bytes=32 * 1024,
            cpus="0.25",
            cpu_seconds=4,
        )
        if w.backend is None:
            obs = w.probe_resources(limits)
            self.assertEqual(obs.reason, "isolation_unavailable")
            self.assertFalse(obs.did_execute)
            write_evidence(
                "T30-C03",
                {"backend": None, "fail_closed": True, "observation": obs_dict(obs)},
            )
            return

        obs = w.probe_resources(limits)
        self.assertTrue(obs.did_execute)
        self.assertIsNotNone(obs.policy_digest)
        self.assertTrue(obs.resource_limits)
        self.assertIn("memory_bytes", obs.resource_limits)
        self.assertEqual(obs.resource_limits.get("memory_bytes"), limits.memory_bytes)
        self.assertEqual(obs.resource_limits.get("pids"), limits.pids)
        # Limited or predictably terminated — must not silently survive the full abuse.
        limited = (
            obs.timed_out
            or obs.output_capped
            or bool(obs.extra.get("killed"))
            or obs.status in {"timeout", "resource_limit", "blocked", "infra_error"}
            or (obs.exit_code not in (0, None))
        )
        self.assertTrue(limited, f"worker was not limited: status={obs.status} exit={obs.exit_code} out={obs.stdout[-400:]}")
        _assert_cleanup_verified(self, obs)
        write_evidence(
            "T30-C03",
            {
                "backend": w.backend,
                "policy_digest": obs.policy_digest,
                "resource_limits": obs.resource_limits,
                "leftover_host_pids": obs.leftover_host_pids,
                "leftover_containers": obs.leftover_containers,
                "leftover_verified": obs.extra.get("leftover_verified"),
                "leftover_query_failed": obs.extra.get("leftover_query_failed"),
                "timed_out": obs.timed_out,
                "output_capped": obs.output_capped,
                "observation": obs_dict(obs),
            },
        )

    def test_timeout_unmarked_inner_children_engine_rm_verified(self) -> None:
        w = worker()
        if w.backend not in {"docker", "podman"}:
            self.fail("timeout leftover verification requires docker/podman")
        marker = "T30UNMARKED-" + uuid.uuid4().hex[:10]
        # Inner argv is `sleep`; host ps cannot prove the container child via T30_MARKER.
        obs = w.run(
            UntrustedJob(
                argv=["sh", "-c", "unset T30_MARKER; export T30_MARKER=; exec sleep 20"],
                limits=_hard_limits(timeout_seconds=2, pids=16),
                marker=marker,
            )
        )
        self.assertTrue(obs.did_execute, obs.as_dict())
        self.assertTrue(obs.timed_out, obs.as_dict())
        self.assertNotEqual(obs.status, "ok")
        _assert_cleanup_verified(self, obs)
        self.assertEqual(snapshot_pids(marker), [])
        write_evidence(
            "T30-C03-timeout-unmarked",
            {
                "status": obs.status,
                "timed_out": obs.timed_out,
                "leftover_host_pids": obs.leftover_host_pids,
                "leftover_containers": obs.leftover_containers,
                "leftover_verified": obs.extra.get("leftover_verified"),
                "marker": marker,
            },
        )

    def test_engine_query_sees_unmarked_container_host_ps_marker_cannot(self) -> None:
        w = worker()
        if w.backend not in {"docker", "podman"}:
            self.fail("engine leftover query requires docker/podman")
        engine = w.engine_path() or w.backend
        image = None
        for candidate in (ALPINE_IMAGE, TOOLCHAIN_IMAGE, "alpine:3.20"):
            if image_exists(engine, candidate):
                image = candidate
                break
        self.assertIsNotNone(image, "need a local alpine/toolchain image; refusing to pull")
        job_id = uuid.uuid4().hex
        marker = "T30_MARKER_ABSENT_" + job_id[:12]
        name = f"jvt30-{job_id[:12]}"
        argv = [
            engine,
            "run",
            "-d",
            "--pull=never",
            "--name",
            name,
            "--network=none",
            f"--label={LABEL_JOB}=1",
            f"--label={LABEL_ID}={job_id}",
            image,
            "sleep",
            "25",
        ]
        proc = subprocess.run(argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, check=False)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        try:
            self.assertEqual(snapshot_pids(marker), [], "host ps must not see a marker the child never inherited")
            found = docker_leftovers(engine, job_id)
            self.assertTrue(found, "engine ps -aq --filter label= must see the leftover container")
        finally:
            force_rm_containers(engine, [name])
            verified = docker_leftovers(engine, job_id)
            self.assertEqual(verified, [], "leftover_containers empty only after verified rm")
        write_evidence(
            "T30-C03-engine-vs-host-ps",
            {"job_id": job_id, "marker_on_host": [], "engine_saw_before_rm": True, "after_rm": verified},
        )

    def test_leftover_query_failure_is_fail_closed_not_empty_success(self) -> None:
        w = worker()
        if w.backend not in {"docker", "podman"}:
            self.fail("leftover query fail-closed requires docker/podman")

        def boom(*_a, **_k):
            raise LeftoverQueryError("simulated leftover query failure")

        with patch("tools.sandbox_worker.executor.docker_leftovers", side_effect=boom):
            obs = w.run(
                UntrustedJob(
                    argv=["/bin/echo", "leftover-query-probe"],
                    limits=_hard_limits(),
                    marker="T30QFAIL",
                )
            )
        self.assertNotEqual(obs.status, "ok")
        self.assertEqual(obs.status, "infra_error")
        self.assertEqual(obs.reason, "leftover_query_failed")
        self.assertEqual(obs.leftover_containers, [])
        self.assertTrue(obs.extra.get("leftover_query_failed"))
        self.assertFalse(obs.extra.get("leftover_verified"))
        _assert_leftovers_fail_status(self, obs)

    def test_leftover_containers_make_status_not_ok(self) -> None:
        w = worker()
        if w.backend not in {"docker", "podman"}:
            self.fail("leftover container status requires docker/podman")
        with patch("tools.sandbox_worker.executor.docker_leftovers", return_value=["deadbeefcafebabe"]):
            obs = w.run(
                UntrustedJob(
                    argv=["/bin/echo", "leftover-container-probe"],
                    limits=_hard_limits(),
                    marker="T30CLEFTOVER",
                )
            )
        self.assertNotEqual(obs.status, "ok")
        self.assertEqual(obs.reason, "leftover_process")
        self.assertEqual(obs.leftover_containers, ["deadbeefcafebabe"])
        self.assertFalse(obs.extra.get("leftover_verified"))
        _assert_leftovers_fail_status(self, obs)

    def test_leftover_host_pids_make_status_not_ok(self) -> None:
        w = worker()
        if w.backend not in {"docker", "podman"}:
            self.fail("leftover host pid status requires docker/podman")
        calls = {"n": 0}

        def fake_ps(_needle: str) -> list[int]:
            calls["n"] += 1
            if calls["n"] <= 1:
                return []
            return [424242]

        with patch("tools.sandbox_worker.executor.snapshot_pids", side_effect=fake_ps):
            obs = w.run(
                UntrustedJob(
                    argv=["/bin/echo", "leftover-pid-probe"],
                    limits=_hard_limits(),
                    marker="T30PLEFTOVER",
                )
            )
        self.assertNotEqual(obs.status, "ok")
        self.assertEqual(obs.reason, "leftover_process")
        self.assertEqual(obs.leftover_host_pids, [424242])
        _assert_leftovers_fail_status(self, obs)

    def test_apply_leftover_status_overrides_ok_only(self) -> None:
        status, reason = apply_leftover_status("ok", "ok", [], [], True)
        self.assertEqual(status, "infra_error")
        self.assertEqual(reason, "leftover_query_failed")
        status, reason = apply_leftover_status("ok", "ok", [1], [], False)
        self.assertEqual(status, "infra_error")
        self.assertEqual(reason, "leftover_process")
        status, reason = apply_leftover_status("ok", "ok", [], ["cid"], False)
        self.assertEqual(status, "infra_error")
        self.assertEqual(reason, "leftover_process")
        status, reason = apply_leftover_status("timeout", "timeout", [], ["cid"], False)
        self.assertEqual(status, "timeout")
        self.assertEqual(reason, "leftover_process")
        status, reason = apply_leftover_status("ok", "ok", [], [], False)
        self.assertEqual(status, "ok")
        self.assertEqual(reason, "ok")
        status, reason = apply_leftover_status("ok", "ok", [], [], False, leftover_rm_failed=True)
        self.assertEqual(status, "infra_error")
        self.assertEqual(reason, "leftover_process")


class TestEngineLeftoverQueryFailClosed(unittest.TestCase):
    def test_docker_leftovers_raises_when_engine_ps_fails(self) -> None:
        with tempfile.TemporaryDirectory(prefix="t30-fake-docker-") as tmp:
            engine = _write_fake_engine(
                Path(tmp),
                'if [ "$1" = "ps" ]; then echo "cannot talk to daemon" >&2; exit 1; fi\nexit 0\n',
            )
            with self.assertRaises(LeftoverQueryError):
                docker_leftovers(str(engine), "job-fail")

    def test_empty_stdout_with_nonzero_rc_is_not_success(self) -> None:
        with tempfile.TemporaryDirectory(prefix="t30-fake-docker-") as tmp:
            engine = _write_fake_engine(Path(tmp), 'exit 2\n')
            with self.assertRaises(LeftoverQueryError):
                docker_leftovers(str(engine), "job-empty-fail")

    def test_ps_argv_uses_aq_and_label_filter(self) -> None:
        with tempfile.TemporaryDirectory(prefix="t30-fake-docker-") as tmp:
            log = Path(tmp) / "argv.log"
            engine = _write_fake_engine(
                Path(tmp),
                f'echo "$@" > "{log}"\nexit 0\n',
            )
            self.assertEqual(docker_leftovers(str(engine), "abc123"), [])
            logged = log.read_text(encoding="utf-8")
            self.assertIn("ps", logged)
            self.assertIn("-aq", logged)
            self.assertIn(f"--filter label={LABEL_JOB}=1", logged)
            self.assertIn(f"--filter label={LABEL_ID}=abc123", logged)

    def test_snapshot_pids_fail_closed_on_ps_error(self) -> None:
        with self.assertRaises(LeftoverQueryError):
            snapshot_pids("")
        with patch("tools.sandbox_worker.procutil.subprocess.run") as run:
            run.return_value = type("P", (), {"returncode": 3, "stdout": "", "stderr": "ps: error"})()
            with self.assertRaises(LeftoverQueryError):
                snapshot_pids("T30JOB")
        with patch("tools.sandbox_worker.procutil.subprocess.run", side_effect=OSError("no ps")):
            with self.assertRaises(LeftoverQueryError):
                snapshot_pids("T30JOB")

    def test_leftover_verified_false_if_host_pids(self) -> None:
        status, reason = apply_leftover_status("ok", "ok", [11], [], False, False)
        self.assertNotEqual(status, "ok")
        self.assertEqual(reason, "leftover_process")

    def test_cleanup_query_failure_is_not_verified_empty(self) -> None:
        with tempfile.TemporaryDirectory(prefix="t30-fake-docker-") as tmp:
            engine = _write_fake_engine(
                Path(tmp),
                'if [ "$1" = "ps" ]; then echo fail >&2; exit 1; fi\nexit 0\n',
            )
            info = cleanup_job_containers(str(engine), "job-x", ["jvt30-x"])
            self.assertFalse(info["verified"])
            self.assertIsNotNone(info["query_error"])
            self.assertEqual(info["leftover_containers"], [])

    def test_cleanup_leftover_empty_only_after_verified_rm(self) -> None:
        with tempfile.TemporaryDirectory(prefix="t30-fake-docker-") as tmp:
            state = Path(tmp) / "state"
            state.write_text("cid1\n", encoding="utf-8")
            engine = _write_fake_engine(
                Path(tmp),
                f"""
if [ "$1" = "ps" ]; then
  cat "{state}" 2>/dev/null || true
  exit 0
fi
if [ "$1" = "rm" ]; then
  : > "{state}"
  exit 0
fi
exit 0
""",
            )
            before = docker_leftovers(str(engine), "job-rm")
            self.assertEqual(before, ["cid1"])
            info = cleanup_job_containers(str(engine), "job-rm", ["cid1"])
            self.assertTrue(info["verified"])
            self.assertIsNone(info["query_error"])
            self.assertEqual(info["leftover_containers"], [])

    def test_cleanup_rm_failure_leaves_nonempty_and_unverified(self) -> None:
        with tempfile.TemporaryDirectory(prefix="t30-fake-docker-") as tmp:
            engine = _write_fake_engine(
                Path(tmp),
                """
if [ "$1" = "ps" ]; then
  echo stuckcid
  exit 0
fi
if [ "$1" = "rm" ]; then
  exit 1
fi
exit 0
""",
            )
            info = cleanup_job_containers(str(engine), "job-stuck", ["stuckcid"])
            self.assertEqual(info["leftover_containers"], ["stuckcid"])
            self.assertTrue(info["rm_failed"])
            self.assertFalse(info["verified"])
            self.assertIsNone(info["query_error"])
            status, reason = apply_leftover_status("ok", "ok", [], info["leftover_containers"], False)
            self.assertNotEqual(status, "ok")
            self.assertEqual(reason, "leftover_process")


if __name__ == "__main__":
    unittest.main()
