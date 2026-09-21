"""Linux unshare+mount namespace backend. Used only when bind policy can be applied."""

from __future__ import annotations

import shlex
import subprocess
import threading
import time
from pathlib import Path
from textwrap import dedent

from ..constants import CONTAINER_ARTIFACTS, CONTAINER_HOME, CONTAINER_INPUTS, CONTAINER_TMP, CONTAINER_WORK
from ..job import UntrustedJob
from ..mounts import assert_mounts_allowed, inventory
from ..policy import Limits
from ..procutil import close_pipes, kill_pg
from .result import BackendResult


def _pump(stream, buf: bytearray, cap: int, capped: list[bool]) -> None:
    try:
        while True:
            chunk = stream.read(4096)
            if not chunk:
                break
            if len(buf) < cap:
                buf.extend(chunk[: cap - len(buf)])
                if len(buf) >= cap:
                    capped[0] = True
            else:
                capped[0] = True
    except OSError:
        return


def ulimit_values(limits: Limits) -> dict[str, int | None]:
    """Translate job.limits into ulimit units (KB, 512-byte blocks, counts, seconds).

    memory_bytes=0 / pids=0 means "not requested": do not emit a 1-unit fake cap.
    Unshare ulimit is still not a verified cgroup hard-memory/pids backend.
    """
    out: dict[str, int | None] = {
        "memory_bytes": int(limits.memory_bytes or 0),
        "as_kb": None,
        "data_kb": None,
        "nproc": None,
        "fsize_blocks": max(int(limits.fsize_bytes) // 512, 1),
        "nofile": max(int(limits.nofile), 1),
        "cpu_seconds": max(int(limits.cpu_seconds), 1),
    }
    if int(limits.memory_bytes or 0) > 0:
        mem_bytes = int(limits.memory_bytes)
        out["as_kb"] = max(mem_bytes // 1024, 1)
        out["data_kb"] = max(mem_bytes // 1024, 1)
    if int(limits.pids or 0) > 0:
        out["nproc"] = max(int(limits.pids), 1)
    return out


def ulimit_block(limits: Limits) -> str:
    """Inner-script rlimits for cpu/fsize/nofile, plus AS/nproc only when requested."""
    u = ulimit_values(limits)
    lines = [
        "ulimit -c 0",
        f"ulimit -t {u['cpu_seconds']}",
        f"ulimit -f {u['fsize_blocks']}",
        f"ulimit -n {u['nofile']}",
    ]
    if u["nproc"] is not None:
        lines.append(f"ulimit -u {u['nproc']}")
    if u["as_kb"] is not None:
        lines.append(f"ulimit -v {u['as_kb']}")
        lines.append(f"ulimit -d {u['data_kb']}")
    return "\n".join(lines)


def _export_env(job: UntrustedJob, job_id: str) -> str:
    lines = [
        f"export HOME={shlex.quote(CONTAINER_HOME)}",
        "export USER=nobody",
        "export PATH=/usr/bin:/bin:/usr/sbin:/sbin",
        "export LANG=C.UTF-8",
        "export LC_ALL=C.UTF-8",
        "export PYTHONNOUSERSITE=1",
        "export PYTHONDONTWRITEBYTECODE=1",
        f"export T30_JOB={shlex.quote(job_id)}",
        f"export TMPDIR={shlex.quote(CONTAINER_TMP)}",
        f"export T30_ARTIFACTS={shlex.quote(CONTAINER_ARTIFACTS)}",
    ]
    for key, val in job.env.items():
        if key in {"HOME", "DOCKER_HOST", "TMPDIR", "SSH_AUTH_SOCK"}:
            continue
        lines.append(f"export {shlex.quote(key)}={shlex.quote(val)}")
    lines.append(f"export HOME={shlex.quote(CONTAINER_HOME)}")
    lines.append(f"export TMPDIR={shlex.quote(CONTAINER_TMP)}")
    return "\n".join(lines)


def build_unshare_script(
    *,
    job: UntrustedJob,
    inputs: Path,
    artifacts: Path,
    job_id: str,
) -> str:
    inner = " ".join(shlex.quote(x) for x in job.argv)
    in_path = shlex.quote(str(inputs))
    art_path = shlex.quote(str(artifacts))
    root = f"/tmp/t30root-{job_id}"
    limits_sh = ulimit_block(job.limits)
    env_sh = _export_env(job, job_id)
    chroot_cmd = "cd " + CONTAINER_WORK + " && exec " + inner
    return dedent(
        f"""
        set -eu
        ROOT={shlex.quote(root)}
        mkdir -p "$ROOT" "$ROOT/usr" "$ROOT/bin" "$ROOT/lib" "$ROOT/lib64" "$ROOT/proc" "$ROOT/dev" \
          "$ROOT{CONTAINER_TMP}" "$ROOT{CONTAINER_WORK}" "$ROOT{CONTAINER_INPUTS}" "$ROOT{CONTAINER_ARTIFACTS}" \
          "$ROOT{CONTAINER_HOME}" "$ROOT/etc"
        mount --bind "$ROOT" "$ROOT"
        mount --make-rprivate "$ROOT" 2>/dev/null || mount --make-private "$ROOT" 2>/dev/null || true
        mount --bind /usr "$ROOT/usr"
        mount -o remount,ro,bind "$ROOT/usr"
        mount --bind /bin "$ROOT/bin"
        mount -o remount,ro,bind "$ROOT/bin"
        if [ -d /lib ]; then mount --bind /lib "$ROOT/lib"; mount -o remount,ro,bind "$ROOT/lib"; fi
        if [ -d /lib64 ]; then mount --bind /lib64 "$ROOT/lib64"; mount -o remount,ro,bind "$ROOT/lib64"; fi
        mount --bind {in_path} "$ROOT{CONTAINER_INPUTS}"
        mount -o remount,ro,bind "$ROOT{CONTAINER_INPUTS}"
        mount --bind {art_path} "$ROOT{CONTAINER_ARTIFACTS}"
        mount -t proc proc "$ROOT/proc"
        mount -t tmpfs -o size=32m,nosuid,nodev tmpfs "$ROOT{CONTAINER_TMP}"
        mount -t tmpfs -o size=32m,nosuid,nodev tmpfs "$ROOT{CONTAINER_WORK}"
        mkdir -p "$ROOT{CONTAINER_HOME}"
        touch "$ROOT/dev/null"
        mount --bind /dev/null "$ROOT/dev/null"
        if [ -e /dev/urandom ]; then touch "$ROOT/dev/urandom"; mount --bind /dev/urandom "$ROOT/dev/urandom"; fi
        if [ -e /dev/zero ]; then touch "$ROOT/dev/zero"; mount --bind /dev/zero "$ROOT/dev/zero"; fi
        mount -o remount,ro,bind "$ROOT"
        {env_sh}
        {limits_sh}
        exec chroot "$ROOT" /bin/sh -c {shlex.quote(chroot_cmd)}
        """
    )


def plan_unshare(
    *,
    binary: str,
    job: UntrustedJob,
    inputs: Path,
    artifacts: Path,
    job_id: str,
) -> dict:
    script = build_unshare_script(job=job, inputs=inputs, artifacts=artifacts, job_id=job_id)
    argv = [
        binary,
        "--user",
        "--map-root-user",
        "--net",
        "--mount",
        "--pid",
        "--fork",
        "--kill-child",
        "/bin/sh",
        "-c",
        script,
    ]
    binds = [
        (str(inputs), CONTAINER_INPUTS, "ro"),
        (str(artifacts), CONTAINER_ARTIFACTS, "rw"),
        ("/usr", "/usr", "ro"),
        ("/bin", "/bin", "ro"),
    ]
    assert_mounts_allowed([(str(inputs), CONTAINER_INPUTS, "ro"), (str(artifacts), CONTAINER_ARTIFACTS, "rw")])
    inv = inventory(
        backend="unshare",
        binds=binds,
        tmpfs=[CONTAINER_TMP, CONTAINER_WORK, CONTAINER_HOME],
        extra={
            "network": "none",
            "namespaces": ["user", "net", "mount", "pid"],
            "host_home_bind": False,
            "docker_sock_bind": False,
        },
    )
    limits = job.limits
    u = ulimit_values(limits)
    resource_limits = {
        "memory_bytes": limits.memory_bytes,
        "pids": limits.pids,
        "cpu_seconds": limits.cpu_seconds,
        "fsize_bytes": limits.fsize_bytes,
        "nofile": limits.nofile,
        "output_bytes": limits.output_bytes,
        "timeout_seconds": limits.timeout_seconds,
        "applied_by": "unshare+ulimit",
        "ulimit": u,
        # Not a verified cgroup. Executor fail-closes when hard memory/pids are requested.
        "memory_enforced": False,
        "pids_enforced": False,
        "nproc_scope": "user-wide-if-applied",
    }
    return {
        "argv": argv,
        "script": script,
        "binds": binds,
        "mount_inventory": inv,
        "resource_limits": resource_limits,
    }


def run_unshare(
    *,
    binary: str,
    job: UntrustedJob,
    inputs: Path,
    artifacts: Path,
    job_id: str,
) -> BackendResult:
    limits = job.limits
    plan = plan_unshare(binary=binary, job=job, inputs=inputs, artifacts=artifacts, job_id=job_id)
    argv = plan["argv"]
    inv = plan["mount_inventory"]
    resource_limits = plan["resource_limits"]
    stdout_buf = bytearray()
    stderr_buf = bytearray()
    capped = [False]
    timed_out = False
    killed = False
    proc = subprocess.Popen(
        argv,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        stdin=subprocess.DEVNULL,
        start_new_session=True,
    )
    t_out = threading.Thread(target=_pump, args=(proc.stdout, stdout_buf, limits.output_bytes, capped), daemon=True)
    t_err = threading.Thread(target=_pump, args=(proc.stderr, stderr_buf, limits.output_bytes, capped), daemon=True)
    t_out.start()
    t_err.start()
    deadline = time.time() + float(limits.timeout_seconds)
    while proc.poll() is None and time.time() < deadline:
        if capped[0]:
            break
        time.sleep(0.05)
    if proc.poll() is None:
        timed_out = not capped[0]
        killed = True
        kill_pg(proc.pid)
        try:
            proc.kill()
        except OSError:
            pass
    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        kill_pg(proc.pid)
    t_out.join(timeout=2)
    t_err.join(timeout=2)
    close_pipes(proc)
    return BackendResult(
        exit_code=proc.returncode,
        stdout=bytes(stdout_buf),
        stderr=bytes(stderr_buf),
        timed_out=timed_out,
        output_capped=capped[0],
        killed=killed,
        argv=argv,
        mount_inventory=inv,
        resource_limits=resource_limits,
        extra={"script": plan["script"], "work": None},
    )
