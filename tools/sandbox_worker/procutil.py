"""Process-group kill and leftover scans. Timeout must kill the whole tree."""

from __future__ import annotations

import os
import signal
import subprocess
import time
from pathlib import Path

from .constants import LABEL_ID, LABEL_JOB


class LeftoverQueryError(RuntimeError):
    """Engine leftover query failed; an empty list is not success."""


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
    """Host PIDs whose command line contains needle. Query failure is not an empty success."""
    if not needle:
        raise LeftoverQueryError("empty leftover marker")
    argv = ["ps", "-ax", "-o", "pid=,command="]
    try:
        proc = subprocess.run(
            argv,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            check=False,
            timeout=20,
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise LeftoverQueryError(f"host pid query failed: {exc}") from exc
    if proc.returncode != 0:
        err = (proc.stderr or proc.stdout or "").strip()
        raise LeftoverQueryError(f"host pid query failed rc={proc.returncode}: {err}")
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
    """List leftover containers via the engine. Raises on query failure; [] is verified empty."""
    if not engine:
        raise LeftoverQueryError("no engine path for leftover query")
    argv = [engine, "ps", "-aq", "--filter", f"label={LABEL_JOB}=1"]
    if job_id:
        argv.extend(["--filter", f"label={LABEL_ID}={job_id}"])
    try:
        proc = subprocess.run(
            argv,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            check=False,
            timeout=20,
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise LeftoverQueryError(f"leftover query failed: {exc}") from exc
    if proc.returncode != 0:
        err = (proc.stderr or proc.stdout or "").strip()
        raise LeftoverQueryError(f"leftover query failed rc={proc.returncode}: {err}")
    return [line.strip() for line in (proc.stdout or "").splitlines() if line.strip()]


def force_rm_containers(engine: str, names: list[str]) -> None:
    for name in names:
        if not name:
            continue
        try:
            subprocess.run(
                [engine, "rm", "-f", name],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                check=False,
                timeout=20,
            )
        except (OSError, subprocess.TimeoutExpired):
            continue


def cleanup_job_containers(engine: str, job_id: str, names: list[str]) -> dict[str, object]:
    """rm then verify leftovers with the engine. leftover list is valid only if query_error is None."""
    force_rm_containers(engine, names)
    try:
        leftover = docker_leftovers(engine, job_id)
    except LeftoverQueryError as exc:
        return {
            "leftover_containers": [],
            "query_error": str(exc),
            "rm_failed": True,
            "verified": False,
        }
    if leftover:
        force_rm_containers(engine, leftover)
        try:
            leftover = docker_leftovers(engine, job_id)
        except LeftoverQueryError as exc:
            return {
                "leftover_containers": leftover,
                "query_error": str(exc),
                "rm_failed": True,
                "verified": False,
            }
    return {
        "leftover_containers": leftover,
        "query_error": None,
        "rm_failed": bool(leftover),
        "verified": not leftover,
    }


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
