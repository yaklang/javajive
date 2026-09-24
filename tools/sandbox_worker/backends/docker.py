"""Docker/Podman backend: network=none, non-root, no sock, RO root, cgroup limits."""

from __future__ import annotations

import os
import shutil
import subprocess
import threading
import time
from pathlib import Path

from ..constants import (
    ALPINE_IMAGE,
    CONTAINER_ARTIFACTS,
    CONTAINER_HOME,
    CONTAINER_INPUTS,
    CONTAINER_JDK,
    CONTAINER_TMP,
    CONTAINER_WORK,
    DEFAULT_PATH,
    LABEL_ID,
    LABEL_JOB,
    SANDBOX_USER,
    TOOLCHAIN_IMAGE,
)
from ..job import UntrustedJob
from ..mounts import assert_mounts_allowed, inventory
from ..policy import Limits
from ..procutil import cleanup_job_containers, close_pipes
from .result import BackendResult


def image_exists(engine: str, image: str) -> bool:
    proc = subprocess.run(
        [engine, "image", "inspect", image],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        check=False,
    )
    return proc.returncode == 0


def image_id(engine: str, image: str) -> str:
    proc = subprocess.run(
        [engine, "image", "inspect", "--format", "{{.Id}}", image],
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
        check=False,
    )
    return (proc.stdout or "").strip()


def select_image(engine: str, job: UntrustedJob) -> str:
    forced = os.environ.get("JAVAJIVE_SANDBOX_IMAGE")
    if forced and image_exists(engine, forced):
        return forced
    if image_exists(engine, TOOLCHAIN_IMAGE):
        return TOOLCHAIN_IMAGE
    if (job.need_java or job.need_python) and image_exists(engine, TOOLCHAIN_IMAGE):
        return TOOLCHAIN_IMAGE
    if image_exists(engine, ALPINE_IMAGE):
        return ALPINE_IMAGE
    if image_exists(engine, "alpine:3.20"):
        return ALPINE_IMAGE
    raise RuntimeError("no local sandbox image (refusing to pull during untrusted run)")


def linux_java_home() -> Path | None:
    if os.uname().sysname != "Linux":
        return None
    home = os.environ.get("JAVA_HOME")
    if home and Path(home, "bin", "javac").exists():
        return Path(home).resolve()
    javac = shutil.which("javac")
    if not javac:
        return None
    # .../bin/javac -> JAVA_HOME
    p = Path(javac).resolve().parent.parent
    if (p / "bin" / "java").exists():
        return p
    return None


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


def run_docker(
    *,
    engine: str,
    engine_path: str,
    job: UntrustedJob,
    inputs: Path,
    artifacts: Path,
    job_id: str,
    image: str | None = None,
) -> BackendResult:
    limits: Limits = job.limits
    image = image or select_image(engine_path, job)
    binds: list[tuple[str, str, str]] = [
        (str(inputs), CONTAINER_INPUTS, "ro"),
        (str(artifacts), CONTAINER_ARTIFACTS, "rw"),
    ]
    extra_env: dict[str, str] = {
        "HOME": CONTAINER_HOME,
        "USER": "nobody",
        "LOGNAME": "nobody",
        "PATH": DEFAULT_PATH,
        "LANG": "C.UTF-8",
        "LC_ALL": "C.UTF-8",
        "PYTHONNOUSERSITE": "1",
        "JAVA_TOOL_OPTIONS": "-Djava.awt.headless=true",
        "T30_JOB": job_id,
    }
    jdk = linux_java_home() if job.need_java else None
    if jdk is not None and "toolchain" not in image:
        binds.append((str(jdk), CONTAINER_JDK, "ro"))
        extra_env["JAVA_HOME"] = CONTAINER_JDK
        extra_env["PATH"] = CONTAINER_JDK + "/bin:" + DEFAULT_PATH
    elif "toolchain" in image or image.startswith(TOOLCHAIN_IMAGE.split(":")[0]):
        extra_env["JAVA_HOME"] = "/usr/lib/jvm/java-21-openjdk"
    assert_mounts_allowed(binds)

    name = f"jvt30-{job_id[:12]}"
    argv = [
        engine_path,
        "run",
        "--name",
        name,
        "--network=none",
        f"--user={SANDBOX_USER}",
        "--read-only",
        "--cap-drop=ALL",
        "--security-opt=no-new-privileges:true",
        "--pull=never",
        "--init",
        "--stop-signal=SIGKILL",
        f"--cpus={limits.cpus}",
        f"--ulimit=fsize={limits.fsize_bytes}:{limits.fsize_bytes}",
        "--ulimit=core=0:0",
        f"--ulimit=nofile={limits.nofile}:{limits.nofile}",
        f"--label={LABEL_JOB}=1",
        f"--label={LABEL_ID}={job_id}",
        f"--tmpfs={CONTAINER_TMP}:rw,nosuid,nodev,size=32m,mode=1777",
        f"--tmpfs={CONTAINER_WORK}:rw,nosuid,nodev,size=32m,mode=1777",
        f"--tmpfs={CONTAINER_HOME}:rw,nosuid,nodev,size=4m,mode=1777",
        f"--workdir={job.workdir}",
    ]
    memory_enforced = int(limits.memory_bytes or 0) > 0
    pids_enforced = int(limits.pids or 0) > 0
    if memory_enforced:
        argv.extend(
            [
                f"--memory={limits.memory_bytes}",
                f"--memory-swap={limits.memory_bytes}",
            ]
        )
    if pids_enforced:
        argv.extend(
            [
                f"--pids-limit={limits.pids}",
                f"--ulimit=nproc={limits.pids}:{limits.pids}",
            ]
        )
    for host, dest, mode in binds:
        spec = f"{host}:{dest}:ro" if "ro" in mode else f"{host}:{dest}"
        argv.extend(["-v", spec])
    for key, val in extra_env.items():
        argv.extend(["-e", f"{key}={val}"])
    for key, val in job.env.items():
        argv.extend(["-e", f"{key}={val}"])
    argv.append(image)
    argv.extend(job.argv)

    tmpfs = [CONTAINER_TMP, CONTAINER_WORK, CONTAINER_HOME]
    inv = inventory(
        backend=engine,
        binds=binds,
        tmpfs=tmpfs,
        extra={
            "image": image,
            "image_id": image_id(engine_path, image),
            "network": "none",
            "user": SANDBOX_USER,
            "cap_drop": ["ALL"],
            "read_only_root": True,
            "no_new_privileges": True,
            "container_name": name,
        },
    )
    resource_limits = {
        "memory_bytes": limits.memory_bytes,
        "pids": limits.pids,
        "cpus": limits.cpus,
        "fsize_bytes": limits.fsize_bytes,
        "output_bytes": limits.output_bytes,
        "timeout_seconds": limits.timeout_seconds,
        "applied_by": engine,
        "enforcement": "cgroup",
        "memory_enforced": memory_enforced,
        "pids_enforced": pids_enforced,
    }

    stdout_buf = bytearray()
    stderr_buf = bytearray()
    capped = [False]
    timed_out = False
    killed = False
    proc: subprocess.Popen[bytes] | None = None
    code: int | None = None
    os_error: OSError | None = None
    leftover_info: dict = {
        "leftover_containers": [],
        "query_error": "cleanup not run",
        "rm_failed": True,
        "verified": False,
    }
    try:
        proc = subprocess.Popen(
            argv,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            stdin=subprocess.DEVNULL,
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
            if capped[0]:
                killed = True
            else:
                timed_out = True
                killed = True
            subprocess.run([engine_path, "kill", name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
            try:
                proc.kill()
            except OSError:
                pass
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()
        t_out.join(timeout=2)
        t_err.join(timeout=2)
        close_pipes(proc)
        code = proc.returncode
    except OSError as exc:
        os_error = exc
    finally:
        leftover_info = cleanup_job_containers(engine_path, job_id, [name])

    extra = {
        "container_name": name,
        "image": image,
        "leftover_containers": list(leftover_info.get("leftover_containers") or []),
        "leftover_query_failed": leftover_info.get("query_error") is not None,
        "leftover_query_error": leftover_info.get("query_error"),
        "leftover_rm_failed": bool(leftover_info.get("rm_failed")),
        "leftover_verified": bool(leftover_info.get("verified")),
    }
    if os_error is not None:
        return BackendResult(
            exit_code=None,
            stdout=bytes(stdout_buf),
            stderr=bytes(stderr_buf) + str(os_error).encode(),
            timed_out=False,
            output_capped=capped[0],
            killed=False,
            argv=argv,
            mount_inventory=inv,
            resource_limits=resource_limits,
            extra=extra,
            error=str(os_error),
        )
    return BackendResult(
        exit_code=code,
        stdout=bytes(stdout_buf),
        stderr=bytes(stderr_buf),
        timed_out=timed_out,
        output_capped=capped[0],
        killed=killed,
        argv=argv,
        mount_inventory=inv,
        resource_limits=resource_limits,
        extra=extra,
    )
