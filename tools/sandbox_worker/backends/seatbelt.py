"""macOS sandbox-exec (seatbelt) backend: deny-network, deny home/token/docker.sock."""

from __future__ import annotations

import os
import resource
import shutil
import subprocess
import sys
import tempfile
import threading
import time
from pathlib import Path

from ..constants import SANDBOX_USER
from ..job import UntrustedJob
from ..mounts import denied_host_paths, docker_sock_candidates, inventory
from ..procutil import close_pipes, kill_pg
from .result import BackendResult

# Never punch a hole through these prefixes (toolchain must live elsewhere).
_BLOCKED_READ_ROOTS = {"/opt", "/Users", "/home", "/", "/private", "/var", "/tmp", "/private/tmp"}
_HOMEBREW_PREFIXES = {"/opt/homebrew", "/usr/local", "/opt/homebrew/opt", "/usr/local/opt"}


def _quote(path: str) -> str:
    return '"' + path.replace("\\", "\\\\").replace('"', '\\"') + '"'


def _path_forms(path: Path | str) -> set[str]:
    """Darwin exposes /var and /private/var; allow both spellings of a path."""
    raw = Path(path)
    forms = {str(raw)}
    try:
        forms.add(str(raw.resolve()))
    except OSError:
        pass
    extra: set[str] = set()
    for s in list(forms):
        if s.startswith("/private/var/") or s.startswith("/private/tmp/") or s.startswith("/private/etc/"):
            extra.add(s[len("/private") :])
        elif s.startswith("/var/") or s.startswith("/tmp/") or s.startswith("/etc/"):
            extra.add("/private" + s)
    forms.update(extra)
    return {p for p in forms if p}


def _ancestor_dirs(path: Path | str) -> set[str]:
    """Directory search chain for Darwin path lookup (getcwd / open)."""
    out: set[str] = set()
    stop = {"/", ""}
    for form in _path_forms(path):
        current = Path(form)
        for anc in current.parents:
            s = str(anc)
            if s in stop:
                break
            out.add(s)
    return out


def _allow_subpaths_for_binary(binary: str) -> list[str]:
    """Read prefixes for a toolchain binary without opening all of /opt or /Users."""
    paths: list[str] = []
    if not binary:
        return paths
    raw = Path(binary)
    try:
        resolved = raw.resolve()
    except OSError:
        resolved = raw
    paths.append(str(resolved.parent))
    if raw.exists():
        paths.append(str(raw.parent))
    for p in resolved.parents:
        s = str(p)
        if p.name == "Cellar":
            paths.append(s)
            if p.parent.exists():
                paths.append(str(p.parent))
        elif s in _HOMEBREW_PREFIXES:
            paths.append(s)
        elif p.name == "homebrew" and str(p.parent) in {"/opt", "/usr/local"}:
            paths.append(s)
    out: list[str] = []
    seen: set[str] = set()
    for p in paths:
        if p in _BLOCKED_READ_ROOTS or p in seen:
            continue
        seen.add(p)
        out.append(p)
    return out


def _count_user_processes() -> int:
    try:
        proc = subprocess.run(
            ["ps", "-u", str(os.getuid()), "-o", "pid="],
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            text=True,
            check=False,
            timeout=5,
        )
    except (OSError, subprocess.TimeoutExpired):
        return 0
    return len([line for line in proc.stdout.splitlines() if line.strip()])


def build_profile(
    *,
    inputs: Path,
    artifacts: Path,
    work: Path,
    extra_read: list[Path],
) -> str:
    read_dirs = {
        "/usr",
        "/bin",
        "/sbin",
        "/System",
        "/Library",
        "/private/etc",
        "/etc",
        "/dev",
        "/private/var/db",
        "/private/var/select",
        "/var/select",
        "/dev/fd",
    }
    meta_dirs: set[str] = set()
    for p in (inputs, work, artifacts):
        read_dirs.update(_path_forms(p))
        meta_dirs.update(_ancestor_dirs(p))
    # /private/var/run is not globally readable: docker.sock often lives there.
    for p in extra_read:
        read_dirs.update(_path_forms(p))
        read_dirs.update(_allow_subpaths_for_binary(str(p)))
        meta_dirs.update(_ancestor_dirs(p))
    for exe in ("python3", "java", "javac", "sh", "bash"):
        w = shutil.which(exe)
        if w:
            read_dirs.update(_allow_subpaths_for_binary(w))
            meta_dirs.update(_ancestor_dirs(w))
    java_home = os.environ.get("JAVA_HOME")
    if java_home:
        read_dirs.update(_allow_subpaths_for_binary(os.path.join(java_home, "bin", "java")))
        try:
            read_dirs.add(str(Path(java_home).resolve()))
        except OSError:
            read_dirs.add(java_home)
    read_dirs = {p for p in read_dirs if p and p not in _BLOCKED_READ_ROOTS}
    # Metadata-only ancestors (path walk). Do not promote /opt or /Users to file-read*.
    meta_dirs = {p for p in meta_dirs if p and p not in {"/Users", "/home"} and not p.startswith("/Users/")}

    write_dirs = set()
    write_dirs.update(_path_forms(artifacts))
    write_dirs.update(_path_forms(work))
    deny_read: list[str] = []
    seen_deny: set[str] = set()
    for d in [*denied_host_paths(), "/Users", *docker_sock_candidates()]:
        if d and d not in seen_deny:
            seen_deny.add(d)
            deny_read.append(d)

    lines = [
        "(version 1)",
        "(deny default)",
        "(deny network*)",
        "(deny network-outbound)",
        "(deny network-inbound)",
        "(deny network-bind)",
        "(allow process-fork)",
        "(allow process-exec)",
        "(allow process-exec*)",
        "(allow signal)",
        "(allow sysctl-read)",
        "(allow mach-lookup)",
        "(allow file-ioctl)",
        "(allow ipc-posix-shm*)",
        "(allow ipc-posix-sem*)",
        "(allow system-fcntl)",
        "(allow file-map-executable)",
        # literal "/" is required for dyld path lookup; it is not a subpath allow-all.
        '(allow file-read* (literal "/"))',
    ]
    for p in sorted(read_dirs):
        lines.append(f"(allow file-read* (subpath {_quote(p)}))")
    for p in sorted(meta_dirs):
        # literal, not subpath: search the dir without revealing sibling temp files.
        lines.append(f"(allow file-read-metadata (literal {_quote(p)}))")
    for p in sorted(write_dirs):
        lines.append(f"(allow file-write* (subpath {_quote(p)}))")
    lines.append('(allow file-write* (literal "/dev/dtracehelper") (literal "/dev/null"))')
    # Last-matching SBPL rule wins: keep host home / Users / docker.sock denied.
    sock_literals: list[str] = []
    for sock in docker_sock_candidates():
        sock_literals.append(sock)
        if sock.startswith("/var/") or sock.startswith("/run/"):
            sock_literals.append("/private" + sock)
    for d in list(deny_read) + sock_literals:
        if not d:
            continue
        lines.append(f"(deny file-read* (subpath {_quote(d)}))")
        lines.append(f"(deny file-read-metadata (subpath {_quote(d)}))")
        lines.append(f"(deny file-write* (subpath {_quote(d)}))")
        lines.append(f"(deny file-read* (literal {_quote(d)}))")
        lines.append(f"(deny file-read-metadata (literal {_quote(d)}))")
        lines.append(f"(deny file-write* (literal {_quote(d)}))")
    return "\n".join(lines) + "\n"


def _preexec(limits, *, nproc_floor: int = 0) -> None:
    _ = nproc_floor
    try:
        os.setsid()
    except OSError:
        pass
    try:
        resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
    except (ValueError, resource.error, OSError):
        pass
    try:
        resource.setrlimit(resource.RLIMIT_NOFILE, (limits.nofile, limits.nofile))
    except (ValueError, resource.error, OSError):
        pass
    try:
        resource.setrlimit(resource.RLIMIT_FSIZE, (limits.fsize_bytes, limits.fsize_bytes))
    except (ValueError, resource.error, OSError):
        pass
    try:
        if int(limits.pids or 0) <= 0:
            raise OSError("nproc not requested")
        nproc = max(int(limits.pids), 16)
        # Darwin RLIMIT_NPROC is user-wide. Never treat this as a job cgroup.
        if sys.platform == "darwin":
            raise OSError("darwin nproc is user-wide; not a job process cap")
        resource.setrlimit(resource.RLIMIT_NPROC, (nproc, nproc))
    except (ValueError, resource.error, OSError, AttributeError):
        pass
    try:
        resource.setrlimit(resource.RLIMIT_CPU, (max(int(limits.cpu_seconds), 1), max(int(limits.cpu_seconds), 1)))
    except (ValueError, resource.error, OSError):
        pass
    if int(limits.memory_bytes or 0) > 0:
        mem = int(limits.memory_bytes)
        try:
            if hasattr(resource, "RLIMIT_AS"):
                resource.setrlimit(resource.RLIMIT_AS, (mem, mem))
        except (ValueError, resource.error, OSError):
            pass
        try:
            if hasattr(resource, "RLIMIT_DATA"):
                resource.setrlimit(resource.RLIMIT_DATA, (mem, mem))
        except (ValueError, resource.error, OSError):
            pass


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


def run_seatbelt(
    *,
    binary: str,
    job: UntrustedJob,
    inputs: Path,
    artifacts: Path,
    job_id: str,
) -> BackendResult:
    limits = job.limits
    work = Path(tempfile.mkdtemp(prefix=f"t30-seatbelt-{job_id[:8]}-"))
    os.chmod(work, 0o1777)
    extra_read = [Path(job.argv[0])] if job.argv else []
    for exe in ("python3", "java", "javac"):
        w = shutil.which(exe)
        if w:
            extra_read.append(Path(w))
    java_home = os.environ.get("JAVA_HOME")
    if java_home:
        extra_read.append(Path(java_home))
    profile = build_profile(inputs=inputs, artifacts=artifacts, work=work, extra_read=extra_read)
    profile_path = work / "profile.sb"
    profile_path.write_text(profile, encoding="utf-8")

    env = {
        "HOME": str(work / "home"),
        "USER": "nobody",
        "PATH": os.environ.get("PATH", "/usr/bin:/bin:/opt/homebrew/bin"),
        "LANG": "C.UTF-8",
        "LC_ALL": "C.UTF-8",
        "PYTHONNOUSERSITE": "1",
        "PYTHONDONTWRITEBYTECODE": "1",
        "T30_JOB": job_id,
        "TMPDIR": str(work),
        "JAVA_TOOL_OPTIONS": "-Djava.awt.headless=true",
    }
    if java_home:
        env["JAVA_HOME"] = java_home
    env.update(job.env)
    env["HOME"] = str(work / "home")
    env["TMPDIR"] = str(work)
    (work / "home").mkdir(exist_ok=True)

    argv = [binary, "-f", str(profile_path), *job.argv]
    binds = [
        (str(inputs), str(inputs), "ro"),
        (str(artifacts), str(artifacts), "rw"),
        (str(work), str(work), "rw"),
    ]
    inv = inventory(
        backend="sandbox-exec",
        binds=binds,
        tmpfs=[],
        extra={
            "network": "none",
            "user": SANDBOX_USER,
            "profile": str(profile_path),
            "denied_host_paths": denied_host_paths(),
            "seatbelt_deny_network": True,
            "host_home_bind": False,
            "docker_sock_bind": False,
        },
    )
    resource_limits = {
        "memory_bytes": limits.memory_bytes,
        "pids": limits.pids,
        "cpu_seconds": limits.cpu_seconds,
        "fsize_bytes": limits.fsize_bytes,
        "nofile": limits.nofile,
        "output_bytes": limits.output_bytes,
        "timeout_seconds": limits.timeout_seconds,
        "applied_by": "rlimit+seatbelt",
        "memory_enforced": False,
        "pids_enforced": False,
        "note": (
            "seatbelt enforces FS/network deny plus cpu/nofile/fsize rlimits; "
            "Darwin cannot enforce RLIMIT_AS/DATA; NPROC is user-wide. "
            "Hard memory/pids requests must fail closed before this backend runs."
        ),
    }
    stdout_buf = bytearray()
    stderr_buf = bytearray()
    capped = [False]
    timed_out = False
    killed = False
    proc = None
    nproc_floor = _count_user_processes()
    try:
        proc = subprocess.Popen(
            argv,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            stdin=subprocess.DEVNULL,
            cwd=str(work),
            env=env,
            start_new_session=True,
            preexec_fn=lambda: _preexec(limits, nproc_floor=nproc_floor),
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
        code = proc.returncode
    except OSError as exc:
        return BackendResult(
            exit_code=None,
            stdout=bytes(stdout_buf),
            stderr=str(exc).encode(),
            timed_out=False,
            output_capped=False,
            killed=False,
            argv=argv,
            mount_inventory=inv,
            resource_limits=resource_limits,
            extra={"profile": profile, "work": str(work)},
            error=str(exc),
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
        extra={"profile": profile, "work": str(work)},
    )
