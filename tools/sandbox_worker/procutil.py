"""Process-group kill and leftover scans. Timeout must kill the whole tree."""

from __future__ import annotations

import os
import signal
import subprocess
import time
from pathlib import Path


def kill_pg(pid: int, *, grace: float = 0.4) -> None:
    if pid <= 0:
        return
    try:
        os.killpg(pid, signal.SIGKILL)
    except (ProcessLookupError, PermissionError, OSError):
        try:
            os.kill(pid, signal.SIGKILL)
        except (ProcessLookupError, PermissionError, OSError):
            pass
    deadline = time.time() + grace
    while time.time() < deadline:
        try:
            os.kill(pid, 0)
        except (ProcessLookupError, OSError):
            return
        time.sleep(0.05)


def snapshot_pids(needle: str) -> list[int]:
    """Host PIDs whose command line contains needle (self-check leftover detector)."""
    if not needle:
        return []
    argv = ["ps", "-ax", "-o", "pid=,command="]
    try:
        proc = subprocess.run(argv, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, check=False)
    except OSError:
        return []
    found: list[int] = []
    me = os.getpid()
    parent = os.getppid()
    for line in proc.stdout.splitlines():
        line = line.strip()
        if not line or needle not in line:
            continue
        try:
            pid_s, cmd = line.split(None, 1)
            pid = int(pid_s)
        except ValueError:
            continue
        if pid in (me, parent):
            continue
        found.append(pid)
    return found


def docker_leftovers(engine: str, job_id: str | None = None) -> list[str]:
    argv = [engine, "ps", "-aq", "--filter", "label=javajive.t30=1"]
    if job_id:
        argv.extend(["--filter", f"label=javajive.t30.job={job_id}"])
    try:
        proc = subprocess.run(argv, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, check=False, timeout=20)
    except (OSError, subprocess.TimeoutExpired):
        return []
    return [line.strip() for line in proc.stdout.splitlines() if line.strip()]


def force_rm_containers(engine: str, names: list[str]) -> None:
    for name in names:
        if not name:
            continue
        subprocess.run([engine, "rm", "-f", name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)


def close_pipes(proc: subprocess.Popen) -> None:
    for stream in (proc.stdout, proc.stderr, proc.stdin):
        if stream is not None:
            try:
                stream.close()
            except OSError:
                pass


def which_java() -> tuple[str | None, str | None]:
    java = _which("java")
    javac = _which("javac")
    home = os.environ.get("JAVA_HOME")
    if home and Path(home, "bin", "javac").exists():
        javac = str(Path(home, "bin", "javac"))
        java = str(Path(home, "bin", "java"))
    return java, javac


def _which(name: str) -> str | None:
    from shutil import which

    return which(name)
