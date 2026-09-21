"""T30 fallback backends: real rlimits/bind/network on unshare and sandbox-exec."""

from __future__ import annotations

import json
import os
import platform
import re
import shutil
import socket
import socketserver
import sys
import tempfile
import threading
import unittest
from pathlib import Path
from unittest import mock

REPO_ROOT = Path(__file__).resolve().parents[2]
if str(REPO_ROOT) not in sys.path:
    sys.path.insert(0, str(REPO_ROOT))
sys.path.insert(0, str(Path(__file__).resolve().parent))

from tools.sandbox_worker.artifacts import ArtifactEscape, confined_path, write_bytes  # noqa: E402
from tools.sandbox_worker.backends.seatbelt import _allow_subpaths_for_binary, build_profile  # noqa: E402
from tools.sandbox_worker.backends.unshare import plan_unshare, ulimit_block, ulimit_values  # noqa: E402
from tools.sandbox_worker.constants import CONTAINER_ARTIFACTS, CONTAINER_INPUTS  # noqa: E402
from tools.sandbox_worker.detect import detect  # noqa: E402
from tools.sandbox_worker.executor import SandboxWorker  # noqa: E402
from tools.sandbox_worker.job import UntrustedJob  # noqa: E402
from tools.sandbox_worker.mounts import host_home  # noqa: E402
from tools.sandbox_worker.network import routable_ipv4  # noqa: E402
from tools.sandbox_worker.capabilities import choose_backend, missing_hard_caps  # noqa: E402
from tools.sandbox_worker.constants import (  # noqa: E402
    REASON_CAPABILITY_UNSUPPORTED,
    STATUS_RESOURCE,
    STATUS_UNSUPPORTED,
)
from tools.sandbox_worker.policy import Limits  # noqa: E402

# Seatbelt/unshare must not be asked for cgroup memory/pids; those tests use docker.
SOFT = Limits(memory_bytes=0, pids=0, timeout_seconds=8, output_bytes=32 * 1024, cpu_seconds=11, nofile=128, fsize_bytes=65536)

BASELINE = REPO_ROOT / "tools" / "sandbox_worker" / "fixtures" / "Baseline.java"
FAKE_TOKEN = "T30-FAKE-TOKEN-NOT-A-SECRET"


def _redact_text(text: str) -> str:
    home = str(Path.home())
    return text.replace(home, "${HOME}")


def _dump_evidence(payload: dict) -> Path:
    redacted = json.loads(_redact_text(json.dumps(payload, default=str, sort_keys=True)))
    handle, name = tempfile.mkstemp(prefix="t30-fallback-evidence-", suffix=".json")
    os.close(handle)
    path = Path(name)
    path.write_text(json.dumps(redacted, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return path


def _require_executed(test: unittest.TestCase, obs, what: str) -> None:
    blob = (
        f"status={obs.status} reason={obs.reason} exit={obs.exit_code} "
        f"did_execute={obs.did_execute} stderr={(obs.stderr or '')[-500:]}"
    )
    if obs.status == "infra_error":
        test.fail(f"{what}: infra_error is not success ({blob})")
    if obs.reason == "isolation_unavailable":
        test.fail(f"{what}: isolation_unavailable is not success ({blob})")
    if not obs.did_execute:
        test.fail(f"{what}: did_execute=False ({blob})")


def _quoted_subpaths(profile: str, prefix: str) -> list[str]:
    found: list[str] = []
    for line in profile.splitlines():
        if line.startswith(prefix):
            found.extend(re.findall(r'subpath "([^"]+)"', line))
    return found


def _quoted_literals(profile: str, prefix: str) -> list[str]:
    found: list[str] = []
    for line in profile.splitlines():
        if line.startswith(prefix):
            found.extend(re.findall(r'literal "([^"]+)"', line))
    return found


def _unshare_available() -> bool:
    return any(p.name == "unshare" and p.available for p in detect().probes)


def _seatbelt_available() -> bool:
    return any(p.name == "sandbox-exec" and p.available for p in detect().probes)


def _assert_inventory_clean(test: unittest.TestCase, inv: dict) -> None:
    sources = inv.get("host_bind_sources") or []
    dests = inv.get("container_destinations") or []
    blob = " ".join(sources + dests).lower()
    test.assertFalse(inv.get("host_home_mounted"), "home must not be mounted")
    test.assertFalse(inv.get("docker_sock_mounted"), "docker.sock must not be mounted")
    test.assertNotIn("docker.sock", blob)
    home = str(Path.home().resolve())
    for src in sources:
        resolved = str(Path(src).resolve())
        test.assertFalse(resolved == home or resolved.startswith(home + os.sep), f"mounted home path {src}")
    test.assertTrue(inv.get("denied_host_paths"), "denied path inventory required")
    denied = " ".join(inv.get("denied_host_paths") or [])
    test.assertIn(str(Path.home()), denied)
    test.assertTrue(any("docker.sock" in p for p in inv.get("denied_host_paths") or []))


class _Handler(socketserver.BaseRequestHandler):
    def handle(self) -> None:
        try:
            self.request.sendall(b"T30-FALLBACK-ALIVE\n")
        except OSError:
            pass


class TestUnshareFallbackPlan(unittest.TestCase):
    def test_unshare_inner_script_applies_ulimits_net_and_bind_policy(self) -> None:
        limits = Limits(
            memory_bytes=64 * 1024 * 1024,
            pids=12,
            cpu_seconds=9,
            fsize_bytes=4096,
            nofile=32,
            timeout_seconds=5,
            output_bytes=1024,
        )
        job = UntrustedJob(argv=["/bin/echo", "hi"], limits=limits)
        inputs = Path(tempfile.mkdtemp(prefix="t30-unshare-in-"))
        artifacts = Path(tempfile.mkdtemp(prefix="t30-unshare-art-"))
        plan = plan_unshare(
            binary="unshare",
            job=job,
            inputs=inputs,
            artifacts=artifacts,
            job_id="feedface",
        )
        script = plan["script"]
        argv = plan["argv"]
        u = ulimit_values(limits)
        block = ulimit_block(limits)
        self.assertIn("ulimit -t 9", block)
        self.assertIn("ulimit -f 8", block)
        self.assertIn("ulimit -n 32", block)
        self.assertIn("ulimit -u 12", block)
        self.assertIn("ulimit -v 65536", block)
        self.assertIn("ulimit -d 65536", block)
        for line in block.splitlines():
            self.assertIn(line, script)
        self.assertIn("--net", argv)
        self.assertEqual(plan["resource_limits"]["applied_by"], "unshare+ulimit")
        self.assertNotEqual(plan["resource_limits"]["applied_by"], "unshare+timeout")
        self.assertEqual(plan["resource_limits"]["ulimit"]["as_kb"], u["as_kb"])
        self.assertEqual(plan["resource_limits"]["cpu_seconds"], 9)
        self.assertEqual(plan["resource_limits"]["fsize_bytes"], 4096)
        self.assertEqual(plan["resource_limits"]["nofile"], 32)
        binds = {(src, dest, mode) for src, dest, mode in plan["binds"]}
        self.assertIn((str(inputs), CONTAINER_INPUTS, "ro"), binds)
        self.assertIn((str(artifacts), CONTAINER_ARTIFACTS, "rw"), binds)
        blob = " ".join(src + " " + dest for src, dest, _ in plan["binds"]).lower()
        self.assertNotIn("docker.sock", blob)
        home = str(Path.home().resolve()).lower()
        for src, dest, _mode in plan["binds"]:
            resolved = str(Path(src).resolve()).lower()
            self.assertFalse(resolved == home or resolved.startswith(home + os.sep))
            self.assertNotIn("docker.sock", dest.lower())
        extra = plan["mount_inventory"]["extra"]
        self.assertEqual(extra.get("network"), "none")
        self.assertIn("net", extra.get("namespaces") or [])
        self.assertIn("mount --bind \"$ROOT\" \"$ROOT\"", script)
        self.assertIn("mount -o remount,ro,bind \"$ROOT\"", script)
        self.assertIn('mount -o remount,ro,bind "$ROOT/inputs"', script.replace(CONTAINER_INPUTS, "/inputs"))
        evidence = _dump_evidence(
            {
                "applied_by": plan["resource_limits"]["applied_by"],
                "ulimit": plan["resource_limits"]["ulimit"],
                "binds": plan["binds"],
                "network": extra.get("network"),
            }
        )
        self.assertNotIn(str(Path.home()), evidence.read_text(encoding="utf-8"))

    def test_forced_unshare_hard_memory_is_unsupported_not_a_pass(self) -> None:
        old = os.environ.get("JAVAJIVE_SANDBOX_BACKEND")
        os.environ["JAVAJIVE_SANDBOX_BACKEND"] = "unshare"
        try:
            worker = SandboxWorker()
            obs = worker.run(
                UntrustedJob(
                    argv=["/bin/echo", "should-not-run"],
                    limits=Limits(memory_bytes=32 * 1024 * 1024, pids=16, timeout_seconds=5),
                )
            )
            self.assertFalse(obs.did_execute)
            self.assertIn(obs.reason, {REASON_CAPABILITY_UNSUPPORTED, "isolation_unavailable"})
            self.assertTrue(obs.extra.get("not_supported"))
            self.assertNotEqual(obs.status, "ok")
        finally:
            if old is None:
                os.environ.pop("JAVAJIVE_SANDBOX_BACKEND", None)
            else:
                os.environ["JAVAJIVE_SANDBOX_BACKEND"] = old

    def test_unshare_darwin_reports_not_supported_not_a_capable_gate(self) -> None:
        det = detect()
        unshare = next(p for p in det.probes if p.name == "unshare")
        if platform.system() != "Linux":
            self.assertFalse(unshare.available)
            self.assertIn("Linux-only", unshare.detail)
            # This is not a capable-backend gate. Docker is the hard-limit backend.
            self.assertTrue(any(p.name == "docker" and p.available for p in det.probes))
            return
        self.assertTrue(unshare.available)

    @unittest.skipUnless(_unshare_available(), "unshare live run is Linux-only (not_supported on macOS)")
    def test_unshare_live_enforcement_when_available(self) -> None:
        old = os.environ.get("JAVAJIVE_SANDBOX_BACKEND")
        os.environ["JAVAJIVE_SANDBOX_BACKEND"] = "unshare"
        try:
            worker = SandboxWorker()
            self.assertEqual(worker.backend, "unshare")
            obs = worker._run_script(
                "echo.sh",
                "#!/bin/sh\necho unshare-ok\nulimit -t\nulimit -n\n",
                Limits(
                    timeout_seconds=8,
                    memory_bytes=0,
                    pids=0,
                    output_bytes=16 * 1024,
                    cpu_seconds=7,
                    nofile=64,
                ),
            )
            _require_executed(self, obs, "unshare live echo")
            self.assertIn("unshare-ok", obs.stdout)
            self.assertEqual(obs.resource_limits.get("applied_by"), "unshare+ulimit")
            self.assertFalse(obs.resource_limits.get("memory_enforced"))
            self.assertFalse(obs.resource_limits.get("pids_enforced"))
            _assert_inventory_clean(self, obs.mount_inventory)
        finally:
            if old is None:
                os.environ.pop("JAVAJIVE_SANDBOX_BACKEND", None)
            else:
                os.environ["JAVAJIVE_SANDBOX_BACKEND"] = old


class TestSeatbeltProfile(unittest.TestCase):
    def test_resolve_host_jdk_is_real_not_usr_bin_stub(self) -> None:
        from tools.sandbox_worker.jdk import resolve_host_jdk

        dropped = {
            "JAVA_HOME",
            "JAVA21_HOME",
            "JAVA_HOME_21",
            "JDK_HOME",
            "JAVA_TOOL_OPTIONS",
            "JAVAJIVE_JAVA_HOME",
        }
        clean = {k: v for k, v in os.environ.items() if k not in dropped}
        with mock.patch.dict(os.environ, clean, clear=True):
            jdk = resolve_host_jdk()
        self.assertIsNotNone(jdk, "real JDK must be discoverable without JAVA_HOME")
        self.assertNotEqual(str(jdk / "bin" / "java"), "/usr/bin/java")
        self.assertTrue((jdk / "bin" / "javac").is_file())
        self.assertTrue((jdk / "release").is_file() or (jdk / "lib" / "modules").is_file())

    def test_profile_denies_home_users_docker_sock_and_tightens_opt(self) -> None:
        inputs = Path(tempfile.mkdtemp(prefix="t30-sb-in-"))
        artifacts = Path(tempfile.mkdtemp(prefix="t30-sb-art-"))
        work = Path(tempfile.mkdtemp(prefix="t30-sb-work-"))
        extra = []
        for exe in ("python3", "java", "javac"):
            path = shutil.which(exe)
            if path:
                extra.append(Path(path))
        profile = build_profile(inputs=inputs, artifacts=artifacts, work=work, extra_read=extra)
        self.assertIn("(deny default)", profile)
        self.assertIn("(deny network*)", profile)
        self.assertIn("(deny network-outbound)", profile)
        self.assertIn("(deny network-inbound)", profile)
        self.assertIn('(deny file-read* (subpath "/Users"))', profile)
        self.assertIn(f'(deny file-read* (subpath "{host_home()}"))', profile)
        self.assertTrue("docker.sock" in profile)
        read_subs = _quoted_subpaths(profile, "(allow file-read*")
        self.assertNotIn("/opt", read_subs)
        self.assertNotIn("/Users", read_subs)
        write_subs = _quoted_subpaths(profile, "(allow file-write*")
        write_lits = _quoted_literals(profile, "(allow file-write*")
        allowed_write = set(write_subs) | set(write_lits)
        art_forms = {str(artifacts), str(artifacts.resolve())}
        work_forms = {str(work), str(work.resolve())}
        for path in allowed_write:
            ok = (
                path in art_forms
                or path in work_forms
                or path in {"/dev/null", "/dev/dtracehelper"}
                or any(path.startswith(root + os.sep) or path == root for root in art_forms | work_forms)
            )
            self.assertTrue(ok, f"file-write allow too broad: {path}")
        for exe in extra:
            for prefix in _allow_subpaths_for_binary(str(exe)):
                self.assertNotEqual(prefix, "/opt")
                self.assertNotEqual(prefix, "/Users")
        evidence = _dump_evidence({"profile_has_network_deny": True, "read_subpaths_sample": read_subs[:12]})
        self.assertNotIn(str(Path.home()), evidence.read_text(encoding="utf-8"))


class TestSeatbeltLiveFallback(unittest.TestCase):
    def setUp(self) -> None:
        self._old_backend = os.environ.get("JAVAJIVE_SANDBOX_BACKEND")
        os.environ["JAVAJIVE_SANDBOX_BACKEND"] = "sandbox-exec"
        if platform.system() != "Darwin":
            self.skipTest("sandbox-exec live tests require macOS")
        if not _seatbelt_available():
            self.fail("sandbox-exec is required on this macOS host; refusing to pass closed")
        self.worker = SandboxWorker()
        if self.worker.backend != "sandbox-exec":
            self.fail(
                f"JAVAJIVE_SANDBOX_BACKEND=sandbox-exec did not select sandbox-exec "
                f"(got {self.worker.backend!r}); infra skip is not success"
            )

    def tearDown(self) -> None:
        if self._old_backend is None:
            os.environ.pop("JAVAJIVE_SANDBOX_BACKEND", None)
        else:
            os.environ["JAVAJIVE_SANDBOX_BACKEND"] = self._old_backend

    def test_forced_backend_network_deny(self) -> None:
        server = socketserver.ThreadingTCPServer(("0.0.0.0", 0), _Handler)
        server.allow_reuse_address = True
        port = server.server_address[1]
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=2) as sock:
                banner = sock.recv(32)
            self.assertIn(b"T30-FALLBACK-ALIVE", banner)
            host_ip = routable_ipv4()
            obs = self.worker.probe_network(host_ip, port, limits=SOFT)
            _require_executed(self, obs, "seatbelt network probe")
            text = obs.stdout + "\n" + obs.stderr
            self.assertNotIn("T30-FALLBACK-ALIVE", text)
            self.assertNotEqual(obs.network_class, "connected")
            self.assertNotEqual(obs.network_class, "dns_failure")
            self.assertEqual(obs.network_class, "policy_deny")
            self.assertEqual((obs.policy or {}).get("network"), "none")
            _dump_evidence({"network_class": obs.network_class, "status": obs.status, "backend": obs.backend})
        finally:
            server.shutdown()
            server.server_close()

    def test_host_canary_and_docker_sock_invisible(self) -> None:
        home_canary = Path.home() / ".t30-fake-token-not-a-secret"
        other_dir = Path(tempfile.mkdtemp(prefix="t30-fallback-host-"))
        other_canary = other_dir / "github_token"
        home_canary.write_text(FAKE_TOKEN, encoding="utf-8")
        other_canary.write_text(FAKE_TOKEN, encoding="utf-8")
        os.chmod(home_canary, 0o600)
        try:
            obs = self.worker.probe_canary(home_canary, limits=SOFT)
            _require_executed(self, obs, "seatbelt home canary")
            text = obs.stdout + "\n" + obs.stderr
            self.assertNotIn(FAKE_TOKEN, text)
            self.assertNotIn("CANARY_VISIBLE=1", text)
            self.assertIn("CANARY_VISIBLE=0", text)
            self.assertNotIn("FOUND:/var/run/docker.sock", text)
            self.assertNotIn("FOUND:/run/docker.sock", text)
            _assert_inventory_clean(self, obs.mount_inventory)
            obs2 = self.worker.probe_canary(other_canary, limits=SOFT)
            _require_executed(self, obs2, "seatbelt tempfile canary")
            text2 = obs2.stdout + "\n" + obs2.stderr
            self.assertNotIn(FAKE_TOKEN, text2)
            self.assertNotIn("CANARY_VISIBLE=1", text2)
            evidence = _dump_evidence({"observation": obs.as_dict(), "observation_other": obs2.as_dict()})
            dumped = evidence.read_text(encoding="utf-8")
            self.assertNotIn(str(Path.home()), dumped)
        finally:
            try:
                home_canary.unlink()
            except OSError:
                pass
            try:
                other_canary.unlink()
                other_dir.rmdir()
            except OSError:
                pass

    def test_rlimits_applied_inside_sandbox(self) -> None:
        limits = Limits(
            timeout_seconds=8,
            memory_bytes=0,
            pids=0,
            output_bytes=32 * 1024,
            fsize_bytes=65536,
            nofile=128,
            cpu_seconds=11,
        )
        py = (
            "import resource\n"
            "for n in ['RLIMIT_NOFILE','RLIMIT_FSIZE','RLIMIT_CPU','RLIMIT_NPROC','RLIMIT_AS','RLIMIT_DATA']:\n"
            "    c = getattr(resource, n, None)\n"
            "    print(n, resource.getrlimit(c) if c is not None else None)\n"
        )
        script = "#!/bin/sh\nDIR=$(dirname \"$0\")\npython3 \"$DIR/rlim.py\"\n"
        obs = self.worker._run_script("rlim.sh", script, limits, extra_files={"rlim.py": py.encode()})
        _require_executed(self, obs, "seatbelt rlimit probe")
        self.assertEqual(obs.resource_limits.get("applied_by"), "rlimit+seatbelt")
        self.assertEqual(obs.resource_limits.get("nofile"), 128)
        self.assertEqual(obs.resource_limits.get("fsize_bytes"), 65536)
        self.assertEqual(obs.resource_limits.get("cpu_seconds"), 11)
        self.assertIn("RLIMIT_NOFILE (128, 128)", obs.stdout)
        self.assertIn("RLIMIT_FSIZE (65536, 65536)", obs.stdout)
        self.assertIn("RLIMIT_CPU (11, 11)", obs.stdout)
        self.assertFalse(obs.resource_limits.get("memory_enforced"))
        self.assertFalse(obs.resource_limits.get("pids_enforced"))
        _dump_evidence({"resource_limits": obs.resource_limits, "stdout": obs.stdout})

    def test_file_write_only_artifacts_and_work(self) -> None:
        outside_dir = Path(tempfile.mkdtemp(prefix="t30-fallback-outside-"))
        outside = outside_dir / "sentinel.txt"
        outside.write_text("UNCHANGED", encoding="utf-8")
        script = f"""#!/bin/sh
DIR=$(dirname "$0")
ART=$(cd "$DIR/../artifacts" && pwd)
echo pwned > "{outside}" || echo OUTSIDE_WRITE_DENIED
echo ok-inside > "$ART/inside.txt" || echo INSIDE_WRITE_DENIED
"""
        obs = self.worker._run_script(
            "write.sh",
            script,
            Limits(timeout_seconds=8, memory_bytes=0, pids=0, output_bytes=16 * 1024),
        )
        _require_executed(self, obs, "seatbelt write confinement")
        self.assertIn("OUTSIDE_WRITE_DENIED", obs.stdout)
        self.assertNotIn("INSIDE_WRITE_DENIED", obs.stdout)
        self.assertEqual(outside.read_text(encoding="utf-8"), "UNCHANGED")
        art = Path(obs.extra["artifact_root"])
        self.assertEqual((art / "inside.txt").read_text(encoding="utf-8").strip(), "ok-inside")
        try:
            outside.unlink()
            outside_dir.rmdir()
        except OSError:
            pass

    def test_artifact_escape_fail_closed(self) -> None:
        root = Path(tempfile.mkdtemp(prefix="t30-fallback-art-"))
        parent = root.parent
        sentinel = parent / "t30-fallback-outside-sentinel.txt"
        sentinel.write_text("UNCHANGED", encoding="utf-8")
        try:
            with self.assertRaises(ArtifactEscape):
                write_bytes(root, "../escape.txt", b"pwned", cap=4096)
            with self.assertRaises(ArtifactEscape):
                write_bytes(root, "/tmp/escape.txt", b"pwned", cap=4096)
            link = root / "outlink"
            link.symlink_to(sentinel)
            with self.assertRaises(ArtifactEscape):
                write_bytes(root, "outlink", b"pwned", cap=4096)
            with self.assertRaises(ArtifactEscape):
                confined_path(root, "outlink")
            self.assertEqual(sentinel.read_text(encoding="utf-8"), "UNCHANGED")

            limits = Limits(
                timeout_seconds=8,
                memory_bytes=0,
                pids=0,
                output_bytes=32 * 1024,
                fsize_bytes=16 * 1024,
                artifact_bytes=16 * 1024,
            )
            obs = self.worker.probe_artifact_escape(limits)
            _require_executed(self, obs, "seatbelt artifact escape")
            stage = Path(obs.extra["stage"])
            self.assertFalse((stage / "escape.txt").exists(), "dot-dot write escaped artifact root")
            self.assertEqual(sentinel.read_text(encoding="utf-8"), "UNCHANGED")
            self.assertLessEqual(obs.artifact_bytes, limits.artifact_bytes)
            _dump_evidence(
                {
                    "escape_exists": (stage / "escape.txt").exists(),
                    "artifact_bytes": obs.artifact_bytes,
                    "status": obs.status,
                }
            )
        finally:
            try:
                sentinel.unlink()
            except OSError:
                pass

    def test_baseline_java_compile_and_run(self) -> None:
        self.assertTrue(BASELINE.is_file())
        src = BASELINE.read_text(encoding="utf-8")
        self.assertIn("class Baseline", src)
        from tools.sandbox_worker.jdk import resolve_host_jdk

        dropped = {
            "JAVA_HOME",
            "JAVA21_HOME",
            "JAVA_HOME_21",
            "JAVA8_HOME",
            "JAVA_HOME_8",
            "JDK_HOME",
            "JDK8_HOME",
            "JAVA_TOOL_OPTIONS",
            "JAVAJIVE_JAVA_HOME",
        }
        clean = {k: v for k, v in os.environ.items() if k not in dropped}
        with mock.patch.dict(os.environ, clean, clear=True):
            jdk = resolve_host_jdk()
            self.assertIsNotNone(
                jdk,
                "infra_error: real JDK is installed but resolve_host_jdk returned None "
                "(must not skip seatbelt or accept /usr/bin/java stub)",
            )
            self.assertNotEqual(str(jdk / "bin" / "java"), "/usr/bin/java")
            self.assertTrue((jdk / "bin" / "javac").is_file())
            obs = self.worker.compile_and_run_java(BASELINE, limits=SOFT)
            _require_executed(self, obs, "seatbelt Baseline.java")
            self.assertNotEqual(obs.status, "blocked", obs.stdout + "\n" + obs.stderr)
            self.assertEqual(obs.status, "ok", obs.stdout + "\n" + obs.stderr)
            self.assertEqual(obs.exit_code, 0, obs.stdout + "\n" + obs.stderr)
            self.assertIn("\n7", "\n" + obs.stdout.replace("\r\n", "\n"))
            home = obs.extra.get("host_jdk_home")
            self.assertTrue(home, msg=obs.extra)
            self.assertEqual(Path(home).resolve(), Path(jdk).resolve())
            argv_blob = " ".join(obs.argv or [])
            self.assertNotIn("/usr/bin/java", argv_blob)
            _dump_evidence(
                {
                    "status": obs.status,
                    "stdout": obs.stdout[-200:],
                    "backend": obs.backend,
                    "host_jdk_home": home,
                    "compiler": obs.compiler,
                    "argv": obs.argv,
                }
            )


class TestHardLimitSelector(unittest.TestCase):
    def test_default_selector_chooses_docker_for_hard_memory(self) -> None:
        det = detect()
        self.assertTrue(any(p.name == "docker" and p.available for p in det.probes), "docker required here")
        worker = SandboxWorker()
        self.assertEqual(worker.backend, "docker")
        hard = Limits(memory_bytes=32 * 1024 * 1024, pids=32)
        self.assertEqual(missing_hard_caps("sandbox-exec", hard), ["memory", "pids"])
        self.assertEqual(missing_hard_caps("docker", hard), [])
        selected, note = choose_backend(
            [p.name for p in det.probes if p.available],
            hard,
            forced=None,
        )
        self.assertEqual(selected, "docker", note)

    def test_forced_seatbelt_hard_memory_is_unsupported_not_a_pass(self) -> None:
        if platform.system() != "Darwin":
            self.skipTest("seatbelt force is macOS-only")
        old = os.environ.get("JAVAJIVE_SANDBOX_BACKEND")
        os.environ["JAVAJIVE_SANDBOX_BACKEND"] = "sandbox-exec"
        try:
            worker = SandboxWorker()
            self.assertEqual(worker.backend, "sandbox-exec")
            obs = worker.run(
                UntrustedJob(
                    argv=["/bin/echo", "should-not-run"],
                    limits=Limits(memory_bytes=32 * 1024 * 1024, pids=16, timeout_seconds=5),
                )
            )
            self.assertFalse(obs.did_execute)
            self.assertEqual(obs.status, STATUS_UNSUPPORTED)
            self.assertEqual(obs.reason, REASON_CAPABILITY_UNSUPPORTED)
            self.assertIn("memory", obs.extra.get("missing_capabilities") or [])
            self.assertTrue(obs.extra.get("not_supported"))
            self.assertFalse(obs.extra.get("memory_enforced"))
        finally:
            if old is None:
                os.environ.pop("JAVAJIVE_SANDBOX_BACKEND", None)
            else:
                os.environ["JAVAJIVE_SANDBOX_BACKEND"] = old

    def test_docker_memory_breach_kills_without_host_children(self) -> None:
        worker = SandboxWorker()
        self.assertEqual(worker.backend, "docker")
        worker.ensure_toolchain_image()
        limits = Limits(
            memory_bytes=16 * 1024 * 1024,
            pids=32,
            timeout_seconds=20,
            output_bytes=8192,
            artifact_bytes=8192,
        )
        obs = worker.run(
            UntrustedJob(
                argv=[
                    "python3",
                    "-c",
                    "x=bytearray(128*1024*1024); print('survived-memory', len(x))",
                ],
                limits=limits,
                need_python=True,
                marker="T30MEMBREACH",
            )
        )
        blob = (obs.stdout or "") + "\n" + (obs.stderr or "")
        self.assertTrue(obs.did_execute, obs.as_dict())
        self.assertNotEqual(obs.status, "ok", blob)
        self.assertNotIn("survived-memory", blob)
        self.assertFalse(obs.timed_out, "timeout is not a memory cap")
        self.assertFalse(obs.output_capped, "output cap is not a memory cap")
        self.assertTrue(obs.resource_limits.get("memory_enforced"), obs.resource_limits)
        self.assertEqual(obs.leftover_host_pids, [])
        self.assertEqual(obs.leftover_containers, [])
        self.assertFalse(obs.extra.get("leftover_query_failed"), obs.extra)
        self.assertTrue(obs.extra.get("leftover_verified"), obs.extra)
        if obs.leftover_host_pids or obs.leftover_containers:
            self.assertNotEqual(obs.status, "ok")
            self.assertEqual(obs.reason, "leftover_process")
        self.assertIn(obs.backend, {"docker", "podman"})
        self.assertTrue(
            obs.status in {STATUS_RESOURCE, STATUS_UNSUPPORTED, "blocked"}
            or (obs.exit_code not in (0, None)),
            msg=obs.as_dict(),
        )
        _dump_evidence(
            {
                "backend": obs.backend,
                "status": obs.status,
                "exit_code": obs.exit_code,
                "timed_out": obs.timed_out,
                "output_capped": obs.output_capped,
                "leftover_host_pids": obs.leftover_host_pids,
                "leftover_containers": obs.leftover_containers,
                "leftover_verified": obs.extra.get("leftover_verified"),
                "resource_limits": obs.resource_limits,
            }
        )

    def test_docker_pids_limit_kills_without_host_children(self) -> None:
        worker = SandboxWorker()
        self.assertEqual(worker.backend, "docker")
        limits = Limits(
            memory_bytes=64 * 1024 * 1024,
            pids=8,
            timeout_seconds=12,
            output_bytes=8192,
            artifact_bytes=8192,
        )
        obs = worker.run(
            UntrustedJob(
                argv=[
                    "sh",
                    "-c",
                    "i=0; while [ \"$i\" -lt 80 ]; do sleep 8 & i=$((i+1)); echo forked:$i; done; echo survived-forks; wait",
                ],
                limits=limits,
                need_python=False,
                marker="T30PIDBREACH",
            )
        )
        blob = (obs.stdout or "") + "\n" + (obs.stderr or "")
        self.assertTrue(obs.did_execute, obs.as_dict())
        self.assertNotEqual(obs.status, "ok", blob)
        self.assertNotIn("survived-forks", blob)
        self.assertFalse(obs.timed_out, "timeout is not a process cap")
        self.assertFalse(obs.output_capped, "output cap is not a process cap")
        self.assertTrue(obs.resource_limits.get("pids_enforced"), obs.resource_limits)
        self.assertEqual(obs.leftover_host_pids, [])
        self.assertEqual(obs.leftover_containers, [])
        self.assertFalse(obs.extra.get("leftover_query_failed"), obs.extra)
        self.assertTrue(obs.extra.get("leftover_verified"), obs.extra)
        if obs.leftover_host_pids or obs.leftover_containers:
            self.assertNotEqual(obs.status, "ok")
            self.assertEqual(obs.reason, "leftover_process")
        _dump_evidence(
            {
                "backend": obs.backend,
                "status": obs.status,
                "exit_code": obs.exit_code,
                "timed_out": obs.timed_out,
                "leftover_host_pids": obs.leftover_host_pids,
                "leftover_containers": obs.leftover_containers,
                "leftover_verified": obs.extra.get("leftover_verified"),
                "stdout_tail": (obs.stdout or "")[-400:],
            }
        )

    def test_docker_timeout_unmarked_children_engine_cleanup(self) -> None:
        worker = SandboxWorker()
        self.assertEqual(worker.backend, "docker")
        marker = "T30NOMARK-" + os.urandom(4).hex()
        obs = worker.run(
            UntrustedJob(
                argv=["sh", "-c", "unset T30_MARKER; exec sleep 20"],
                limits=Limits(
                    memory_bytes=32 * 1024 * 1024,
                    pids=16,
                    timeout_seconds=2,
                    output_bytes=8192,
                    artifact_bytes=8192,
                ),
                marker=marker,
            )
        )
        blob = (obs.stdout or "") + "\n" + (obs.stderr or "")
        self.assertTrue(obs.did_execute, obs.as_dict())
        self.assertTrue(obs.timed_out, blob)
        self.assertNotEqual(obs.status, "ok", blob)
        self.assertEqual(obs.leftover_host_pids, [])
        self.assertEqual(obs.leftover_containers, [])
        self.assertFalse(obs.extra.get("leftover_query_failed"), obs.extra)
        self.assertTrue(obs.extra.get("leftover_verified"), obs.extra)
        if obs.leftover_host_pids or obs.leftover_containers:
            self.assertNotEqual(obs.status, "ok")
            self.assertEqual(obs.reason, "leftover_process")
        _dump_evidence(
            {
                "backend": obs.backend,
                "status": obs.status,
                "timed_out": obs.timed_out,
                "leftover_host_pids": obs.leftover_host_pids,
                "leftover_containers": obs.leftover_containers,
                "leftover_verified": obs.extra.get("leftover_verified"),
                "marker": marker,
            }
        )


if __name__ == "__main__":
    unittest.main()
