"""Untrusted class/JAR execution must use SandboxWorker, never host java.

Default policy is untrusted. Reviewed fixtures call verify_and_run(trusted=True).
"""

from __future__ import annotations

import os
import stat
from dataclasses import replace
from pathlib import Path
from typing import Any, Iterable

from .artifacts import ArtifactEscape
from .constants import CONTAINER_INPUTS, CONTAINER_JDK, TOOLCHAIN_IMAGE
from .executor import SandboxWorker
from .job import UntrustedJob
from .policy import Limits

# Documented default: toolchain JDK in docker needs more than 64MiB to boot.
# Case-specific Limits may override. Never fall back to host java on OOM.
DEFAULT_UNTRUSTED_LIMITS = Limits(
    memory_bytes=256 * 1024 * 1024,
    pids=64,
    timeout_seconds=30,
    output_bytes=256 * 1024,
    artifact_bytes=256 * 1024,
    cpus="0.5",
)

ISOLATION_STATUS = {
    "unsupported",
    "infra_error",
    "timeout",
    "resource_limit",
    "isolation_unavailable",
    "invalid_input",
}
ISOLATION_REASON = {
    "isolation_unavailable",
    "capability_unsupported",
    "leftover_process",
    "leftover_query_failed",
    "resource_limit",
    "timeout",
    "policy_deny",
}


class StagingError(ValueError):
    def __init__(self, stage: str, message: str) -> None:
        super().__init__(message)
        self.stage = stage
        self.message = message


def _under(root: Path, path: Path) -> bool:
    try:
        path.resolve().relative_to(root.resolve())
        return True
    except (ValueError, OSError):
        return False


MAX_STAGED_BYTES = 64 * 1024 * 1024
MAX_STAGED_FILES = 10000


def _read_regular_file(path: Path, *, root: Path, max_bytes: int = MAX_STAGED_BYTES) -> bytes:
    if not all(hasattr(os, name) for name in ("O_DIRECTORY", "O_NOFOLLOW", "O_NONBLOCK")):
        raise StagingError("unsupported", "safe staging requires no-follow directory descriptors")
    if path.is_symlink():
        raise StagingError("invalid_input", f"symlink rejected: {path}")
    fd = None
    try:
        root = root.resolve(strict=True)
        resolved = path.resolve(strict=True)
        relative = resolved.relative_to(root)
        # Anchor each lookup to an opened directory and reject symlinks on every
        # component; a concurrent symlink swap cannot redirect a staged read.
        fd = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        for component in relative.parts[:-1]:
            child = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd)
            fd = child
        child = os.open(relative.name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=fd)
        os.close(fd)
        fd = child
        st = os.fstat(fd)
        if not stat.S_ISREG(st.st_mode):
            raise StagingError("invalid_input", f"not a regular file: {path}")
        if st.st_size > max_bytes:
            raise StagingError("resource_limit", f"staged input bytes exceed {MAX_STAGED_BYTES}")
        with os.fdopen(fd, "rb") as stream:
            fd = None
            data = stream.read(max_bytes + 1)
        if len(data) > max_bytes:
            raise StagingError("resource_limit", "staged input grew beyond byte limit")
        return data
    except (OSError, ValueError) as exc:
        if isinstance(exc, StagingError):
            raise
        raise StagingError("invalid_input", f"unreadable or escaping input {path}: {exc}") from exc
    finally:
        if fd is not None:
            os.close(fd)


class _StagingBudget:
    def __init__(self, max_bytes: int, max_files: int):
        self.remaining, self.files = max_bytes, max_files

    def read(self, path: Path, root: Path) -> bytes:
        if self.files <= 0:
            raise StagingError("resource_limit", "staged input file count exceeded")
        data = _read_regular_file(path, root=root, max_bytes=self.remaining)
        self.remaining -= len(data)
        self.files -= 1
        return data


def _walk_error(exc: OSError) -> None:
    raise StagingError("invalid_input", f"unreadable classpath directory: {exc}")


def _stage_class_tree(root: Path, prefix: str, files: dict[str, bytes], budget: _StagingBudget) -> None:
    """Stage .class files under prefix. Directory `-cp` does not load nested jars.

    Symlink directories and symlink files fail closed (never silently skipped).
    """
    if root.is_symlink():
        raise StagingError("invalid_input", f"directory symlink rejected: {root}")
    if not root.is_dir():
        raise StagingError("invalid_input", f"not a directory: {root}")
    root_res = root.resolve()
    for dirpath, dirnames, filenames in os.walk(root, followlinks=False, onerror=_walk_error):
        for d in list(dirnames):
            child = Path(dirpath) / d
            if child.is_symlink():
                raise StagingError("invalid_input", f"directory symlink rejected: {child}")
        for name in filenames:
            path = Path(dirpath) / name
            if path.is_symlink():
                raise StagingError("invalid_input", f"symlink rejected: {path}")
            if path.suffix != ".class":
                continue
            rel = path.relative_to(root).as_posix()
            if ".." in Path(rel).parts:
                raise StagingError("invalid_input", f"parent segment in extra_cp: {rel}")
            key = f"{prefix}/{rel}" if prefix else rel
            files[key] = budget.read(path, root_res)


def _rel_files(root: Path, extra_cp: Iterable[Path] | None, *, max_bytes: int = MAX_STAGED_BYTES, max_files: int = MAX_STAGED_FILES) -> tuple[dict[str, bytes], list[str]]:
    """Stage class/jar family. Classpath order is [class_dir, *extra_cp] (Java semantics).

    extra_cp FILE jars are classpath entries. extra_cp directories are class roots only;
    nested jars inside a directory are not put on -cp (no implicit wildcard).
    Missing / unsupported / symlink inputs raise StagingError (never silent drop).
    """
    if max_bytes < 0 or max_files < 0:
        raise StagingError("invalid_input", "negative staging limit")
    budget = _StagingBudget(max_bytes, max_files)
    files: dict[str, bytes] = {}
    cp_entries: list[str] = []
    root = Path(root)
    if root.is_symlink():
        raise StagingError("invalid_input", f"class_dir symlink rejected: {root}")
    if not root.exists():
        raise StagingError("invalid_input", f"class_dir missing: {root}")
    _stage_class_tree(root, "main", files, budget)
    cp_entries.append(f"{CONTAINER_INPUTS}/main")
    for i, extra in enumerate(extra_cp or []):
        extra = Path(extra)
        if extra.is_symlink():
            raise StagingError("invalid_input", f"extra_cp symlink rejected: {extra}")
        if not extra.exists():
            raise StagingError("invalid_input", f"extra_cp missing: {extra}")
        prefix = f"extra{i}"
        if extra.is_file():
            if extra.suffix != ".jar":
                raise StagingError("unsupported", f"extra_cp file is not a jar: {extra}")
            key = f"{prefix}/{extra.name}"
            files[key] = budget.read(extra, extra.parent.resolve())
            cp_entries.append(f"{CONTAINER_INPUTS}/{key}")
            continue
        if extra.is_dir():
            _stage_class_tree(extra, prefix, files, budget)
            cp_entries.append(f"{CONTAINER_INPUTS}/{prefix}")
            continue
        raise StagingError("unsupported", f"extra_cp is not a directory or jar: {extra}")
    if not files:
        raise StagingError("invalid_input", "untrusted input has no class/jar bytes")
    return files, cp_entries


def _isolation_stage(obs) -> str | None:
    extra = obs.extra or {}
    if extra.get("leftover_query_failed") or extra.get("leftover_cleanup_failed"):
        return "leftover_query_failed" if extra.get("leftover_query_failed") else "leftover_process"
    if obs.leftover_host_pids or obs.leftover_containers:
        return "leftover_process"
    if obs.status in ISOLATION_STATUS and obs.status != "ok":
        if obs.status in {"unsupported", "isolation_unavailable"}:
            return "isolation_unavailable" if obs.reason == "isolation_unavailable" else "capability_unsupported"
        if obs.status in {"timeout", "resource_limit"}:
            return obs.status
        if obs.status == "infra_error":
            return "leftover_query_failed" if obs.reason == "leftover_query_failed" else "infra_error"
        return obs.status
    if obs.reason in ISOLATION_REASON:
        return obs.reason
    if not obs.did_execute:
        return "policy_deny"
    if extra.get("leftover_verified") is not True:
        return "leftover_query_failed"
    return None


def _limits_for_timeout(caps: Limits, timeout: float | None) -> Limits:
    if timeout and timeout < caps.timeout_seconds:
        return replace(caps, timeout_seconds=float(timeout))
    return caps


def run_untrusted_class_dir(
    class_dir: Path,
    class_name: str,
    *,
    extra_cp: list[Path] | None = None,
    timeout: float = 20,
    limits: Limits | None = None,
    worker: SandboxWorker | None = None,
) -> dict[str, Any]:
    """Run a class family inside the isolation worker. Never subprocess host java."""
    try:
        from tools.generators_holdout.pipeline import InvalidBinaryName, validate_binary_name
        try:
            class_name = validate_binary_name(class_name)
        except InvalidBinaryName as exc:
            raise StagingError("invalid_input", str(exc)) from exc
        files, cp_entries = _rel_files(Path(class_dir), extra_cp)
    except StagingError as exc:
        return {
            "argv": [],
            "rc": None,
            "stdout": "",
            "stderr": exc.message,
            "timeout": False,
            "verified_and_ran": False,
            "stage": exc.stage,
            "host_java": False,
            "backend": None,
            "did_execute": False,
            "status": exc.stage,
            "reason": exc.message,
        }
    except ArtifactEscape as exc:
        return {
            "argv": [],
            "rc": None,
            "stdout": "",
            "stderr": str(exc),
            "timeout": False,
            "verified_and_ran": False,
            "stage": "invalid_input",
            "host_java": False,
            "backend": None,
            "did_execute": False,
            "status": "invalid_input",
            "reason": str(exc),
        }
    caps = _limits_for_timeout(limits or DEFAULT_UNTRUSTED_LIMITS, timeout)
    cp = ":".join(cp_entries)
    worker = worker or SandboxWorker()
    obs = worker.run(
        UntrustedJob(
            argv=["java", "-Xverify:all", "-cp", cp, class_name],
            input_files=files,
            limits=caps,
            need_java=True,
            marker="T30UNTRUSTED",
        )
    )
    extra = obs.extra or {}
    isolation = _isolation_stage(obs)
    verified = bool(
        obs.did_execute
        and obs.exit_code == 0
        and obs.status == "ok"
        and isolation is None
        and extra.get("leftover_verified") is True
        and not extra.get("leftover_query_failed")
        and not extra.get("leftover_cleanup_failed")
        and not obs.leftover_host_pids
        and not obs.leftover_containers
    )
    backend_extra = extra.get("backend_extra") or {}
    ran: dict[str, Any] = {
        "argv": list(obs.argv),
        "rc": obs.exit_code,
        "stdout": obs.stdout or "",
        "stderr": (obs.stderr or "") + ("" if obs.did_execute else f" [{obs.status}:{obs.reason}]"),
        "timeout": bool(obs.timed_out),
        "verified_and_ran": verified,
        "host_java": False,
        "backend": obs.backend,
        "did_execute": obs.did_execute,
        "status": obs.status,
        "reason": obs.reason,
        "leftover_host_pids": list(obs.leftover_host_pids),
        "leftover_containers": list(obs.leftover_containers),
        "leftover_query_failed": bool(extra.get("leftover_query_failed")),
        "leftover_cleanup_failed": bool(extra.get("leftover_cleanup_failed")),
        "leftover_verified": bool(extra.get("leftover_verified")),
        "classpath": cp,
        "cp_entries": list(cp_entries),
        "staged_files": sorted(files),
        "worker_image": (obs.mount_inventory or {}).get("image")
        or backend_extra.get("image")
        or extra.get("image")
        or TOOLCHAIN_IMAGE,
        "worker_java_home": extra.get("host_jdk_home")
        or backend_extra.get("host_jdk_home")
        or extra.get("JAVA_HOME")
        or CONTAINER_JDK,
        "limits": caps.as_dict(),
        "limits_memory_bytes": caps.memory_bytes,
        "limits_pids": caps.pids,
        "limits_fsize_bytes": caps.fsize_bytes,
        "limits_nofile": caps.nofile,
        "limits_cpu_seconds": caps.cpu_seconds,
        "policy": obs.policy,
        "policy_digest": obs.policy_digest,
    }
    if isolation:
        ran["stage"] = isolation
        ran["verified_and_ran"] = False
        return ran
    from tools.generators_holdout.identity import classify_java_process

    ran["stage"] = classify_java_process(ran)
    if ran["stage"] != "run_ok":
        ran["verified_and_ran"] = False
    return ran
