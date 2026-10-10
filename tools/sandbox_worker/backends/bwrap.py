"""bubblewrap backend: unshare net/pid, bind only inputs+artifacts+toolchain."""

from __future__ import annotations

import subprocess
import threading
import time
from pathlib import Path

from ..constants import (
    CONTAINER_ARTIFACTS,
    CONTAINER_HOME,
    CONTAINER_INPUTS,
    CONTAINER_JDK,
    CONTAINER_TMP,
    CONTAINER_WORK,
    DEFAULT_PATH,
    SANDBOX_USER_GID,
    SANDBOX_USER_UID,
)
from ..job import UntrustedJob
from ..mounts import assert_mounts_allowed, inventory
from ..procutil import close_pipes, kill_pg
from .docker import linux_java_home
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


def run_bwrap(
    *,
    binary: str,
    job: UntrustedJob,
    inputs: Path,
    artifacts: Path,
    job_id: str,
) -> BackendResult:
    limits = job.limits
    argv = [
        binary,
        "--die-with-parent",
        "--new-session",
        "--unshare-net",
        "--unshare-pid",
        "--unshare-ipc",
        "--unshare-uts",
        "--hostname",
        "javajive-t30",
        "--cap-drop",
        "ALL",
        "--uid",
        str(SANDBOX_USER_UID),
        "--gid",
        str(SANDBOX_USER_GID),
        "--ro-bind",
        "/usr",
        "/usr",
        "--ro-bind",
        "/bin",
        "/bin",
        "--ro-bind-try",
        "/lib",
        "/lib",
        "--ro-bind-try",
        "/lib64",
        "/lib64",
        "--ro-bind-try",
        "/sbin",
        "/sbin",
        "--ro-bind-try",
        "/etc/alternatives",
        "/etc/alternatives",
        "--ro-bind-try",
        "/etc/ssl",
        "/etc/ssl",
        "--ro-bind-try",
        "/etc/java",
        "/etc/java",
        "--dev",
        "/dev",
        "--proc",
        "/proc",
        "--tmpfs",
        CONTAINER_TMP,
        "--tmpfs",
        CONTAINER_WORK,
        "--tmpfs",
        CONTAINER_HOME,
        "--ro-bind",
        str(inputs),
        CONTAINER_INPUTS,
        "--bind",
        str(artifacts),
        CONTAINER_ARTIFACTS,
        "--chdir",
        job.workdir,
        "--setenv",
        "HOME",
        CONTAINER_HOME,
        "--setenv",
        "PATH",
        DEFAULT_PATH,
        "--setenv",
        "PYTHONNOUSERSITE",
        "1",
        "--setenv",
        "T30_JOB",
        job_id,
        "--setenv",
        "LANG",
        "C.UTF-8",
    ]
    binds = [
        (str(inputs), CONTAINER_INPUTS, "ro"),
        (str(artifacts), CONTAINER_ARTIFACTS, "rw"),
        ("/usr", "/usr", "ro"),
        ("/bin", "/bin", "ro"),
    ]
    jdk = linux_java_home() if job.need_java else None
    if jdk is not None:
        idx = argv.index("--chdir")
        argv[idx:idx] = ["--ro-bind", str(jdk), CONTAINER_JDK, "--setenv", "JAVA_HOME", CONTAINER_JDK]
        binds.append((str(jdk), CONTAINER_JDK, "ro"))
    for key, val in job.env.items():
        argv.extend(["--setenv", key, val])
    argv.extend(job.argv)
    assert_mounts_allowed([(h, d, m) for h, d, m in binds if not h.startswith("/usr") and h not in {"/bin"}])

    inv = inventory(
        backend="bwrap",
        binds=binds,
        tmpfs=[CONTAINER_TMP, CONTAINER_WORK, CONTAINER_HOME],
        extra={"network": "none", "unshare": ["net", "pid", "ipc", "uts"]},
    )
    resource_limits = {
        "memory_bytes": limits.memory_bytes,
        "pids": limits.pids,
        "output_bytes": limits.output_bytes,
        "timeout_seconds": limits.timeout_seconds,
        "applied_by": "bwrap+timeout",
    }
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
    )
