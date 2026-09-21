"""Sandbox worker: real isolation backends only; fail closed otherwise."""

from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
import uuid
from pathlib import Path

from .artifacts import enforce_cap, write_bytes
from .backends.bwrap import run_bwrap
from .backends.docker import image_exists, run_docker, select_image
from .backends.seatbelt import run_seatbelt
from .backends.unshare import run_unshare
from .capabilities import choose_backend, missing_hard_caps
from .constants import (
    CONTAINER_ARTIFACTS,
    CONTAINER_INPUTS,
    CONTAINER_WORK,
    POLICY_VERSION,
    REASON_ISOLATION_UNAVAILABLE,
    REASON_RESOURCE_LIMIT,
    REASON_TIMEOUT,
    STATUS_BLOCKED,
    STATUS_INFRA_ERROR,
    STATUS_INVALID_INPUT,
    STATUS_OK,
    STATUS_RESOURCE,
    STATUS_TIMEOUT,
    TOOLCHAIN_IMAGE,
)
from .detect import detect, detection_for_forced
from .job import UntrustedJob
from .network import PROBE_PYTHON, PROBE_SH, classify_text
from .observation import Observation
from .policy import Limits, make_policy
from .procutil import LeftoverQueryError, docker_leftovers, snapshot_pids


def apply_leftover_status(
    status: str,
    reason: str,
    leftover_pids: list[int],
    leftover_containers: list[str],
    leftover_query_failed: bool,
    leftover_rm_failed: bool = False,
) -> tuple[str, str]:
    """Cleanup failure is a failed observation; empty leftover lists are success only if the query ran."""
    if leftover_query_failed:
        return STATUS_INFRA_ERROR, "leftover_query_failed"
    if leftover_rm_failed or leftover_pids or leftover_containers:
        if status == STATUS_OK:
            status = STATUS_INFRA_ERROR
        return status, "leftover_process"
    return status, reason


class SandboxWorker:
    def __init__(self, *, prefer: str | None = None) -> None:
        forced = prefer or os.environ.get("JAVAJIVE_SANDBOX_BACKEND")
        self.prefer = forced
        self.detection = detect()
        self._probe = {p.name: p for p in self.detection.probes}
        self.os_name = self.detection.os_name
        available = [p.name for p in self.detection.probes if p.available]
        if forced:
            # Keep the forced name for tests; run() fail-closes if it cannot enforce.
            forced_det = detection_for_forced(forced)
            self.backend = forced_det.selected
            self.selection_note = f"forced {forced}"
        else:
            self.backend, self.selection_note = choose_backend(available, Limits())

    def engine_path(self) -> str | None:
        if self.backend in {"docker", "podman"}:
            p = self._probe.get(self.backend)
            return p.binary if p else None
        return None

    def sandbox_paths(self, inputs: Path, artifacts: Path, work: Path | None = None) -> dict[str, str]:
        if self.backend in {"docker", "podman", "bwrap", "unshare"}:
            return {
                "inputs": CONTAINER_INPUTS,
                "artifacts": CONTAINER_ARTIFACTS,
                "work": CONTAINER_WORK,
            }
        return {
            "inputs": str(inputs),
            "artifacts": str(artifacts),
            "work": str(work or inputs.parent / "work"),
        }

    def ensure_toolchain_image(self) -> str | None:
        if self.backend not in {"docker", "podman"}:
            return None
        engine = self.engine_path()
        if not engine:
            return None
        if image_exists(engine, TOOLCHAIN_IMAGE):
            return TOOLCHAIN_IMAGE
        dockerfile = Path(__file__).resolve().parent / "image" / "Dockerfile"
        if not dockerfile.is_file():
            return None
        proc = subprocess.run(
            [engine, "build", "-t", TOOLCHAIN_IMAGE, "-f", str(dockerfile), str(dockerfile.parent)],
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            check=False,
            timeout=360,
        )
        if proc.returncode != 0:
            raise RuntimeError("toolchain image build failed:\n" + (proc.stdout or "")[-2000:])
        return TOOLCHAIN_IMAGE

    def _policy(self, limits: Limits, extra: dict | None = None) -> "tuple":
        extra = dict(extra or {})
        extra["policy_lock"] = "execution-sandbox-policy"
        extra["policy_version"] = POLICY_VERSION
        extra["os"] = self.os_name
        policy = make_policy(backend=self.backend or "none", limits=limits, extra=extra)
        return policy, policy.digest()

    def run(self, job: UntrustedJob) -> Observation:
        err = job.validate()
        if err:
            return Observation(
                status=STATUS_INVALID_INPUT,
                reason=err,
                backend=self.backend,
                policy_digest=None,
                policy=None,
                argv=list(job.argv),
                exit_code=None,
                stdout="",
                stderr=err,
                did_execute=False,
            )
        if self.backend is None:
            return Observation.isolation_unavailable(
                "no isolation backend available (docker/podman/bwrap/unshare/sandbox-exec); "
                "refusing to run untrusted work with subprocess+timeout",
                os_name=self.os_name,
            )

        missing = missing_hard_caps(self.backend, job.limits)
        if missing:
            available = [p.name for p in self.detection.probes if p.available]
            alt, note = choose_backend(available, job.limits, forced=None)
            if self.prefer:
                return Observation.capability_unsupported(
                    f"backend {self.backend} cannot enforce hard {missing}; "
                    f"timeout/output caps are not a memory/process sandbox. {note}",
                    backend=self.backend,
                    missing=missing,
                    os_name=self.os_name,
                    argv=list(job.argv),
                )
            if not alt:
                return Observation.capability_unsupported(
                    f"no capable backend for hard {missing}. {note}",
                    backend=self.backend,
                    missing=missing,
                    os_name=self.os_name,
                    argv=list(job.argv),
                )
            self.backend = alt

        job_id = uuid.uuid4().hex
        marker = job.marker or f"T30JOB-{job_id[:12]}"
        job.marker = marker
        job.env = dict(job.env)
        job.env.setdefault("T30_MARKER", marker)

        stage = Path(tempfile.mkdtemp(prefix=f"t30-stage-{job_id[:8]}-"))
        inputs = stage / "inputs"
        artifacts = stage / "artifacts"
        work = stage / "work"
        inputs.mkdir()
        artifacts.mkdir()
        work.mkdir()
        os.chmod(artifacts, 0o1777)
        os.chmod(work, 0o1777)
        for rel, data in job.input_files.items():
            dest = inputs / rel
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_bytes(data if isinstance(data, bytes) else str(data).encode())
            dest.chmod(0o644)
        # Make inputs tree world-readable for container uid 65534.
        for dirpath, dirnames, filenames in os.walk(inputs):
            os.chmod(dirpath, 0o755)
            for name in filenames:
                os.chmod(os.path.join(dirpath, name), 0o755 if name.endswith(".sh") else 0o644)

        extra_policy = {}
        if self.backend in {"docker", "podman"}:
            try:
                extra_policy["image"] = select_image(self.engine_path() or self.backend, job)
            except RuntimeError as exc:
                return Observation(
                    status=STATUS_INFRA_ERROR,
                    reason=REASON_ISOLATION_UNAVAILABLE,
                    backend=self.backend,
                    policy_digest=None,
                    policy=None,
                    argv=list(job.argv),
                    exit_code=None,
                    stdout="",
                    stderr=str(exc),
                    did_execute=False,
                )

        policy, digest = self._policy(job.limits, extra_policy)
        try:
            before_pids = snapshot_pids(marker)
        except LeftoverQueryError as exc:
            return Observation(
                status=STATUS_INFRA_ERROR,
                reason="leftover_query_failed",
                backend=self.backend,
                policy_digest=digest,
                policy=policy.as_dict(),
                argv=list(job.argv),
                exit_code=None,
                stdout="",
                stderr=str(exc),
                did_execute=False,
                extra={"leftover_query_failed": True, "leftover_verified": False},
            )
        result = None
        try:
            if self.backend in {"docker", "podman"}:
                result = run_docker(
                    engine=self.backend,
                    engine_path=self.engine_path() or self.backend,
                    job=job,
                    inputs=inputs,
                    artifacts=artifacts,
                    job_id=job_id,
                    image=extra_policy.get("image"),
                )
            elif self.backend == "bwrap":
                result = run_bwrap(
                    binary=self._probe["bwrap"].binary or "bwrap",
                    job=job,
                    inputs=inputs,
                    artifacts=artifacts,
                    job_id=job_id,
                )
            elif self.backend == "unshare":
                result = run_unshare(
                    binary=self._probe["unshare"].binary or "unshare",
                    job=job,
                    inputs=inputs,
                    artifacts=artifacts,
                    job_id=job_id,
                )
            elif self.backend == "sandbox-exec":
                result = run_seatbelt(
                    binary=self._probe["sandbox-exec"].binary or "sandbox-exec",
                    job=job,
                    inputs=inputs,
                    artifacts=artifacts,
                    job_id=job_id,
                )
            else:
                return Observation.isolation_unavailable(
                    f"backend {self.backend} not implemented", os_name=self.os_name
                )
        except Exception as exc:  # noqa: BLE001 — convert to infra observation
            return Observation(
                status=STATUS_INFRA_ERROR,
                reason=type(exc).__name__,
                backend=self.backend,
                policy_digest=digest,
                policy=policy.as_dict(),
                argv=list(job.argv),
                exit_code=None,
                stdout="",
                stderr=str(exc),
                did_execute=False,
            )

        art_size, art_capped = enforce_cap(artifacts, job.limits.artifact_bytes)
        leftover_pids: list[int] = []
        leftover_ct: list[str] = []
        leftover_query_failed = False
        leftover_query_error = None
        leftover_rm_failed = False
        try:
            leftover_pids = [p for p in snapshot_pids(marker) if p not in before_pids]
        except LeftoverQueryError as exc:
            leftover_query_failed = True
            leftover_query_error = str(exc)
        if self.backend in {"docker", "podman"}:
            engine = self.engine_path() or self.backend
            try:
                leftover_ct = docker_leftovers(engine, job_id)
            except LeftoverQueryError as exc:
                leftover_query_failed = True
                leftover_query_error = str(exc)
                leftover_ct = []
        backend_extra = result.extra or {}
        if backend_extra.get("leftover_query_failed"):
            leftover_query_failed = True
            leftover_query_error = leftover_query_error or backend_extra.get("leftover_query_error")
        if backend_extra.get("leftover_rm_failed"):
            leftover_rm_failed = True
        for cid in backend_extra.get("leftover_containers") or []:
            if cid and cid not in leftover_ct:
                leftover_ct.append(cid)

        stdout = result.stdout.decode("utf-8", "replace")
        stderr = result.stderr.decode("utf-8", "replace")
        network_class = classify_text(stdout + "\n" + stderr) if "NETWORK_CLASS=" in (stdout + stderr) else None

        if result.timed_out:
            status, reason = STATUS_TIMEOUT, REASON_TIMEOUT
        elif result.output_capped or art_capped:
            status, reason = STATUS_RESOURCE, REASON_RESOURCE_LIMIT
        elif result.error:
            status, reason = STATUS_INFRA_ERROR, result.error
        elif result.exit_code == 0:
            status, reason = STATUS_OK, "ok"
        else:
            status, reason = STATUS_BLOCKED, f"exit:{result.exit_code}"
        status, reason = apply_leftover_status(
            status,
            reason,
            leftover_pids,
            leftover_ct,
            leftover_query_failed,
            leftover_rm_failed,
        )

        return Observation(
            status=status,
            reason=reason,
            backend=self.backend,
            policy_digest=digest,
            policy=policy.as_dict(),
            argv=result.argv,
            exit_code=result.exit_code,
            stdout=stdout,
            stderr=stderr,
            timed_out=result.timed_out,
            output_capped=result.output_capped,
            artifact_bytes=art_size,
            artifact_capped=art_capped,
            mount_inventory=result.mount_inventory,
            resource_limits=result.resource_limits,
            network_class=network_class,
            did_execute=True,
            leftover_host_pids=leftover_pids,
            leftover_containers=leftover_ct,
            extra={
                "job_id": job_id,
                "marker": marker,
                "stage": str(stage),
                "artifact_root": str(artifacts),
                "backend_extra": result.extra,
                "killed": result.killed,
                "leftover_query_failed": leftover_query_failed,
                "leftover_query_error": leftover_query_error,
                "leftover_rm_failed": leftover_rm_failed,
                "leftover_verified": (
                    not leftover_query_failed
                    and not leftover_rm_failed
                    and not leftover_ct
                    and not leftover_pids
                ),
                "leftover_cleanup_failed": bool(
                    leftover_pids or leftover_ct or leftover_query_failed or leftover_rm_failed
                ),
                "host_jdk_home": (result.extra or {}).get("host_jdk_home")
                or extra_policy.get("host_jdk_home"),
            },
        )

    def probe_canary(self, canary: Path, limits: Limits | None = None) -> Observation:
        limits = limits or Limits(timeout_seconds=8, memory_bytes=64 * 1024 * 1024, pids=32, output_bytes=64 * 1024)
        script = f"""#!/bin/sh
set -f
CANARY={_sh_quote(str(canary))}
echo "probe-canary"
if [ -e "$CANARY" ] || [ -r "$CANARY" ]; then
  echo CANARY_VISIBLE=1
  cat "$CANARY" 2>/dev/null || true
  exit 10
fi
echo CANARY_VISIBLE=0
# Token/socket inventory from inside the worker
for p in /var/run/docker.sock /run/docker.sock /var/run/podman/podman.sock "$HOME/.docker/run/docker.sock" "$HOME/.ssh/id_rsa" "$HOME/.git-credentials" "$HOME/.netrc"; do
  if [ -e "$p" ]; then
    echo "FOUND:$p"
  fi
done
echo HOME_IN_SANDBOX="$HOME"
ls -ld /Users 2>/dev/null || echo "NO_USERS_DIR"
exit 0
"""
        job = UntrustedJob(
            argv=["/bin/sh", f"{self._input_path_placeholder()}/probe_canary.sh"],
            input_files={"probe_canary.sh": script.encode()},
            limits=limits,
            name="canary",
        )
        # For seatbelt, argv must use the real staged path; rewrite after staging is hard,
        # so use a launcher that lives in /inputs after copy — docker uses /inputs.
        if self.backend == "sandbox-exec":
            # seatbelt runs argv as given; we rewrite in run() only via input files.
            # Use /bin/sh with the script passed as the copied file by wrapping after stage.
            return self._run_script("probe_canary.sh", script, limits, need_python=False)
        return self._run_script("probe_canary.sh", script, limits, need_python=False)

    def _input_path_placeholder(self) -> str:
        if self.backend in {"docker", "podman", "bwrap", "unshare"}:
            return CONTAINER_INPUTS
        return "{inputs}"  # rewritten in _run_script for seatbelt via relative after copy

    def _run_script(
        self,
        name: str,
        script: str,
        limits: Limits,
        *,
        need_python: bool = False,
        need_java: bool = False,
        extra_files: dict[str, bytes] | None = None,
        env: dict[str, str] | None = None,
    ) -> Observation:
        files = {name: script.encode() if isinstance(script, str) else script}
        if extra_files:
            files.update(extra_files)
        if self.backend == "sandbox-exec":
            # argv uses /bin/sh and we pass the script via relative name; executor copies to inputs.
            # Seatbelt cwd is work; use absolute by injecting after we know paths — handled below
            # by running sh -c with path substitution in a two-phase job.
            return self._run_seatbelt_script(name, files, limits, need_java=need_java, env=env)
        argv = ["/bin/sh", f"{CONTAINER_INPUTS}/{name}"]
        return self.run(
            UntrustedJob(
                argv=argv,
                input_files=files,
                limits=limits,
                need_python=need_python,
                need_java=need_java,
                env=env or {},
            )
        )

    def _run_seatbelt_script(
        self,
        name: str,
        files: dict[str, bytes],
        limits: Limits,
        *,
        need_java: bool = False,
        env: dict[str, str] | None = None,
    ) -> Observation:
        # Stage first by using a trampoline: run copies files, but argv must exist.
        # We put the script in input_files and set argv to /bin/sh + host path after a custom stage.
        # Simpler: write a tiny python/sh that sandbox-exec can run from /bin/sh -c with $1.
        sh = shutil.which("sh") or "/bin/sh"
        # Create job whose argv is rewritten inside a custom path-aware run.
        missing = missing_hard_caps(self.backend, limits)
        if missing:
            return Observation.capability_unsupported(
                f"backend {self.backend} cannot enforce hard {missing}; "
                "timeout/output caps are not a memory/process sandbox",
                backend=self.backend,
                missing=missing,
                os_name=self.os_name,
                argv=[sh, name],
            )
        job = UntrustedJob(
            argv=[sh, "SCRIPT"],  # placeholder
            input_files=files,
            limits=limits,
            need_java=need_java,
            env=env or {},
        )
        # Monkeypatch by staging here then pointing argv at the real script.
        err = job.validate()
        if err:
            return Observation(
                status=STATUS_INVALID_INPUT,
                reason=err,
                backend=self.backend,
                policy_digest=None,
                policy=None,
                argv=list(job.argv),
                exit_code=None,
                stdout="",
                stderr=err,
                did_execute=False,
            )
        job_id = uuid.uuid4().hex
        stage = Path(tempfile.mkdtemp(prefix=f"t30-stage-{job_id[:8]}-"))
        inputs = stage / "inputs"
        artifacts = stage / "artifacts"
        inputs.mkdir()
        artifacts.mkdir()
        os.chmod(artifacts, 0o1777)
        for rel, data in files.items():
            dest = inputs / rel
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_bytes(data if isinstance(data, bytes) else str(data).encode())
            dest.chmod(0o755 if rel.endswith(".sh") else 0o644)
        script_path = inputs / name
        job.argv = [sh, str(script_path)]
        job.marker = f"T30JOB-{job_id[:12]}"
        job.env = dict(job.env)
        job.env.setdefault("T30_MARKER", job.marker)
        policy, digest = self._policy(limits, {"image": None})
        result = run_seatbelt(
            binary=self._probe["sandbox-exec"].binary or "sandbox-exec",
            job=job,
            inputs=inputs,
            artifacts=artifacts,
            job_id=job_id,
        )
        art_size, art_capped = enforce_cap(artifacts, limits.artifact_bytes)
        leftover_query_failed = False
        leftover_pids: list[int] = []
        leftover_ct: list[str] = []
        try:
            leftover_pids = snapshot_pids(job.marker)
        except LeftoverQueryError:
            leftover_query_failed = True
            leftover_pids = []
        stdout = result.stdout.decode("utf-8", "replace")
        stderr = result.stderr.decode("utf-8", "replace")
        if result.timed_out:
            status, reason = STATUS_TIMEOUT, REASON_TIMEOUT
        elif result.output_capped or art_capped:
            status, reason = STATUS_RESOURCE, REASON_RESOURCE_LIMIT
        elif result.error:
            status, reason = STATUS_INFRA_ERROR, result.error
        elif result.exit_code == 0:
            status, reason = STATUS_OK, "ok"
        else:
            status, reason = STATUS_BLOCKED, f"exit:{result.exit_code}"
        status, reason = apply_leftover_status(
            status, reason, leftover_pids, leftover_ct, leftover_query_failed
        )
        return Observation(
            status=status,
            reason=reason,
            backend=self.backend,
            policy_digest=digest,
            policy=policy.as_dict(),
            argv=result.argv,
            exit_code=result.exit_code,
            stdout=stdout,
            stderr=stderr,
            timed_out=result.timed_out,
            output_capped=result.output_capped,
            artifact_bytes=art_size,
            artifact_capped=art_capped,
            mount_inventory=result.mount_inventory,
            resource_limits=result.resource_limits,
            network_class=classify_text(stdout + "\n" + stderr) if "NETWORK_CLASS=" in (stdout + stderr) else None,
            did_execute=True,
            leftover_host_pids=leftover_pids,
            leftover_containers=leftover_ct,
            extra={
                "job_id": job_id,
                "artifact_root": str(artifacts),
                "stage": str(stage),
                "killed": result.killed,
                "leftover_query_failed": leftover_query_failed,
                "leftover_verified": (not leftover_query_failed) and not leftover_pids and not leftover_ct,
                "leftover_cleanup_failed": bool(leftover_pids or leftover_ct or leftover_query_failed),
                "backend_extra": result.extra,
                "host_jdk_home": (result.extra or {}).get("host_jdk_home"),
            },
        )

    def probe_network(self, ip: str, port: int, limits: Limits | None = None) -> Observation:
        limits = limits or Limits(timeout_seconds=8, memory_bytes=64 * 1024 * 1024, pids=16, output_bytes=32 * 1024)
        if self.backend is None:
            return Observation.isolation_unavailable(
                "network probe refused: isolation_unavailable", os_name=self.os_name
            )
        # Numeric IP only — DNS failure must not masquerade as policy.
        try:
            parts = ip.split(".")
            if len(parts) != 4 or not all(p.isdigit() and 0 <= int(p) <= 255 for p in parts):
                return Observation(
                    status=STATUS_INVALID_INPUT,
                    reason="network probe requires numeric IPv4",
                    backend=self.backend,
                    policy_digest=None,
                    policy=None,
                    argv=[],
                    exit_code=None,
                    stdout="",
                    stderr="non-numeric target rejected",
                    did_execute=False,
                )
        except ValueError:
            return Observation(
                status=STATUS_INVALID_INPUT,
                reason="bad ip",
                backend=self.backend,
                policy_digest=None,
                policy=None,
                argv=[],
                exit_code=None,
                stdout="",
                stderr="bad ip",
                did_execute=False,
            )
        py = "#!/bin/sh\n" + "python3 - <<'PY'\n" + PROBE_PYTHON + "PY\n"
        # Always ship both; sh wrapper tries python then wget.
        wrapper = f"""#!/bin/sh
IP={_sh_quote(ip)}
PORT={int(port)}
if command -v python3 >/dev/null 2>&1; then
  python3 {_sh_quote(CONTAINER_INPUTS if self.backend != 'sandbox-exec' else 'INPUTS')}/net_probe.py "$IP" "$PORT"
  exit $?
fi
/bin/sh {_sh_quote(CONTAINER_INPUTS if self.backend != 'sandbox-exec' else 'INPUTS')}/net_probe.sh "$IP" "$PORT"
exit $?
"""
        if self.backend == "sandbox-exec":
            wrapper = f"""#!/bin/sh
DIR=$(dirname "$0")
IP={_sh_quote(ip)}
PORT={int(port)}
if command -v python3 >/dev/null 2>&1; then
  python3 "$DIR/net_probe.py" "$IP" "$PORT"
  exit $?
fi
/bin/sh "$DIR/net_probe.sh" "$IP" "$PORT"
exit $?
"""
        files = {
            "net_probe.py": PROBE_PYTHON.encode(),
            "net_probe.sh": ("#!/bin/sh\n" + PROBE_SH).encode(),
        }
        return self._run_script(
            "net_wrap.sh",
            wrapper,
            limits,
            need_python=True,
            extra_files=files,
        )

    def probe_resources(self, limits: Limits | None = None) -> Observation:
        limits = limits or Limits(
            timeout_seconds=6,
            memory_bytes=32 * 1024 * 1024,
            pids=8,
            output_bytes=32 * 1024,
            fsize_bytes=64 * 1024,
            artifact_bytes=64 * 1024,
            cpus="0.3",
        )
        if self.backend is None:
            return Observation.isolation_unavailable("resource probe refused", os_name=self.os_name)
        # Fork + output spam + memory doubling. Limited or predictably killed.
        script = f"""#!/bin/sh
echo "resource-probe start"
# output spam
i=0
while [ "$i" -lt 20000 ]; do
  echo "SPAM-LINE-$i-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
  i=$((i+1))
done
# fork
j=0
while [ "$j" -lt 40 ]; do
  sleep 30 &
  j=$((j+1))
  echo "forked:$j"
done
# memory
a=xxxxxxxxxx
k=0
while [ "$k" -lt 28 ]; do
  a="$a$a"
  k=$((k+1))
  echo "memdoubles:$k"
done
echo "resource-probe survived"
wait
"""
        return self._run_script("resource.sh", script, limits)

    def probe_artifact_escape(self, limits: Limits | None = None) -> Observation:
        limits = limits or Limits(
            timeout_seconds=8,
            memory_bytes=64 * 1024 * 1024,
            pids=16,
            output_bytes=32 * 1024,
            fsize_bytes=32 * 1024,
            artifact_bytes=32 * 1024,
        )
        if self.backend is None:
            return Observation.isolation_unavailable("artifact probe refused", os_name=self.os_name)
        art = CONTAINER_ARTIFACTS if self.backend != "sandbox-exec" else "ARTIFACTS"
        script = f"""#!/bin/sh
ART="{art}"
if [ -z "$T30_ARTIFACTS" ]; then T30_ARTIFACTS="$ART"; fi
ARTROOT="$T30_ARTIFACTS"
echo "escape-probe"
echo pwned > "$ARTROOT/../escape.txt" 2>/tmp/e1 || echo "dotdot_write_failed"
ln -s /etc/passwd "$ARTROOT/symlink_out" 2>/tmp/e2 || echo "symlink_create_failed"
echo pwned > "$ARTROOT/symlink_out" 2>/tmp/e3 || echo "symlink_write_failed"
dd if=/dev/zero of="$ARTROOT/huge.bin" bs=1024 count=200 2>/tmp/e4 || echo "huge_write_failed"
echo ok-inside > "$ARTROOT/inside.txt" 2>/tmp/e5 || echo "inside_write_failed"
cat /tmp/e1 /tmp/e2 /tmp/e3 /tmp/e4 /tmp/e5 2>/dev/null || true
ls -la "$ARTROOT" || true
"""
        if self.backend == "sandbox-exec":
            script = """#!/bin/sh
DIR=$(dirname "$0")
ARTROOT=$(cd "$DIR/../artifacts" 2>/dev/null; pwd)
echo "escape-probe"
echo pwned > "$ARTROOT/../escape.txt" 2>/tmp/e1 || echo "dotdot_write_failed"
ln -s /etc/passwd "$ARTROOT/symlink_out" 2>/tmp/e2 || echo "symlink_create_failed"
echo pwned > "$ARTROOT/symlink_out" 2>/tmp/e3 || echo "symlink_write_failed"
dd if=/dev/zero of="$ARTROOT/huge.bin" bs=1024 count=200 2>/tmp/e4 || echo "huge_write_failed"
echo ok-inside > "$ARTROOT/inside.txt" 2>/tmp/e5 || echo "inside_write_failed"
cat /tmp/e1 /tmp/e2 /tmp/e3 /tmp/e4 /tmp/e5 2>/dev/null || true
ls -la "$ARTROOT" || true
"""
        obs = self._run_script("escape.sh", script, limits, env={"T30_ARTIFACTS": CONTAINER_ARTIFACTS})
        return obs

    def compile_and_run_java(self, source: Path, class_name: str = "Baseline", limits: Limits | None = None) -> Observation:
        limits = limits or Limits(
            timeout_seconds=40,
            memory_bytes=384 * 1024 * 1024,
            pids=64,
            output_bytes=128 * 1024,
            artifact_bytes=2 * 1024 * 1024,
            fsize_bytes=2 * 1024 * 1024,
            cpus="1.0",
        )
        if self.backend is None:
            return Observation.isolation_unavailable(
                "java compile/run refused: isolation_unavailable", os_name=self.os_name
            )
        if not class_name.isidentifier():
            return Observation(
                status=STATUS_INVALID_INPUT,
                reason="illegal class name",
                backend=self.backend,
                policy_digest=None,
                policy=None,
                argv=[],
                exit_code=None,
                stdout="",
                stderr="illegal class name",
                did_execute=False,
            )
        src = Path(source).read_bytes()
        if self.backend in {"docker", "podman"}:
            try:
                self.ensure_toolchain_image()
            except RuntimeError as exc:
                # Linux hosts can bind-mount JAVA_HOME into alpine; Darwin cannot.
                if self.os_name != "Linux":
                    return Observation(
                        status=STATUS_INFRA_ERROR,
                        reason=REASON_ISOLATION_UNAVAILABLE,
                        backend=self.backend,
                        policy_digest=None,
                        policy=None,
                        argv=[],
                        exit_code=None,
                        stdout="",
                        stderr=str(exc),
                        did_execute=False,
                    )
        seatbelt_env: dict[str, str] | None = None
        if self.backend == "sandbox-exec":
            from .jdk import resolve_host_jdk

            jdk = resolve_host_jdk()
            if jdk is None:
                return Observation(
                    status=STATUS_INFRA_ERROR,
                    reason="jdk_not_found",
                    backend=self.backend,
                    policy_digest=None,
                    policy=None,
                    argv=[],
                    exit_code=None,
                    stdout="",
                    stderr="unable to locate a real JDK (javac+java); refusing /usr/bin/java stub",
                    did_execute=False,
                    extra={"host_jdk_home": None},
                )
            seatbelt_env = {"JAVA_HOME": str(jdk)}
            script = f"""#!/bin/sh
set -e
DIR=$(dirname "$0")
ART=$(cd "$DIR/../artifacts" && pwd)
if [ -z "$JAVA_HOME" ] || [ ! -x "$JAVA_HOME/bin/javac" ] || [ ! -x "$JAVA_HOME/bin/java" ]; then
  echo "real JDK missing under JAVA_HOME=$JAVA_HOME" >&2
  exit 127
fi
export PATH="$JAVA_HOME/bin:$PATH"
mkdir -p "$ART/classes"
"$JAVA_HOME/bin/javac" -proc:none -encoding UTF-8 --release 8 -d "$ART/classes" "$DIR/{class_name}.java"
"$JAVA_HOME/bin/javac" -version > "$ART/javac.version" 2>&1 || true
"$JAVA_HOME/bin/java" -Xverify:all -classpath "$ART/classes" {class_name}
"""
        else:
            script = f"""#!/bin/sh
set -e
mkdir -p {CONTAINER_ARTIFACTS}/classes
if [ -x /toolchain/jdk/bin/javac ]; then
  export JAVA_HOME=/toolchain/jdk
  export PATH="$JAVA_HOME/bin:$PATH"
fi
javac -proc:none -encoding UTF-8 --release 8 -d {CONTAINER_ARTIFACTS}/classes {CONTAINER_INPUTS}/{class_name}.java
javac -version > {CONTAINER_ARTIFACTS}/javac.version 2>&1 || true
java -Xverify:all -classpath {CONTAINER_ARTIFACTS}/classes {class_name}
"""
        obs = self._run_script(
            "run_java.sh",
            script,
            limits,
            need_java=True,
            extra_files={f"{class_name}.java": src},
            env=seatbelt_env,
        )
        extra = dict(obs.extra or {})
        if seatbelt_env and seatbelt_env.get("JAVA_HOME"):
            extra["host_jdk_home"] = seatbelt_env["JAVA_HOME"]
        backend_extra = extra.get("backend_extra") or {}
        if backend_extra.get("host_jdk_home"):
            extra["host_jdk_home"] = backend_extra["host_jdk_home"]
        obs.extra = extra
        obs.compiler = (obs.extra.get("artifact_root") and _read_quiet(Path(obs.extra["artifact_root"]) / "javac.version")) or None
        return obs


def _read_quiet(path: Path) -> str | None:
    try:
        return path.read_text(encoding="utf-8", errors="replace").strip()
    except OSError:
        return None


def _sh_quote(s: str) -> str:
    return "'" + s.replace("'", "'\"'\"'") + "'"


def write_confined_artifact(root: Path, rel: str, data: bytes, cap: int) -> Path:
    return write_bytes(root, rel, data, cap=cap)
