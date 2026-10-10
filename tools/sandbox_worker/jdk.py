"""Locate a real JDK (javac + java) without mutating host environ.

macOS /usr/bin/java is a stub launcher, not a JDK. Callers pass the resolved
home into a worker-scoped env/policy. Never assign os.environ["JAVA_HOME"].
"""

from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

_REJECT_HOMES = {"/", "/usr", "/usr/local", "/bin", "/sbin", "/opt", "/opt/homebrew", "/Library", "/System"}
_ENV_KEYS = (
    "JAVAJIVE_JAVA_HOME",
    "JAVA21_HOME",
    "JAVA_HOME_21",
    "JAVA_HOME",
    "JDK_HOME",
    "JAVA8_HOME",
    "JAVA_HOME_8",
    "JDK8_HOME",
)


def _looks_like_jdk(home: Path) -> bool:
    try:
        resolved = home.resolve()
    except OSError:
        resolved = home
    if str(resolved) in _REJECT_HOMES:
        return False
    java = resolved / "bin" / "java"
    javac = resolved / "bin" / "javac"
    if not java.is_file() or not javac.is_file():
        return False
    if not os.access(java, os.X_OK) or not os.access(javac, os.X_OK):
        return False
    if str(java) in {"/usr/bin/java", "/bin/java"}:
        return False
    return (
        (resolved / "release").is_file()
        or (resolved / "lib" / "modules").is_file()
        or (resolved / "lib" / "rt.jar").is_file()
        or (resolved / "jre" / "lib" / "rt.jar").is_file()
    )


def _home_variants(path: Path) -> list[Path]:
    out = [path]
    for rel in (
        "Contents/Home",
        "libexec/openjdk.jdk/Contents/Home",
        "Home",
    ):
        out.append(path / rel)
    return out


def _accepted(path: Path | None) -> Path | None:
    if path is None:
        return None
    for cand in _home_variants(Path(path)):
        if _looks_like_jdk(cand):
            try:
                return cand.resolve()
            except OSError:
                return cand
    return None


def _from_env() -> list[Path]:
    found: list[Path] = []
    for key in _ENV_KEYS:
        raw = os.environ.get(key)
        if raw:
            found.append(Path(raw))
    return found


def _from_java_home_helper() -> list[Path]:
    helper = Path("/usr/libexec/java_home")
    if not helper.is_file():
        return []
    found: list[Path] = []
    specs: list[list[str]] = [["-v", "21"], ["-v", "17"], ["-v", "11"], ["-v", "1.8"], []]
    for args in specs:
        try:
            proc = subprocess.run(
                [str(helper), *args],
                stdout=subprocess.PIPE,
                stderr=subprocess.DEVNULL,
                text=True,
                timeout=10,
                check=False,
            )
        except (OSError, subprocess.TimeoutExpired):
            continue
        text = (proc.stdout or "").strip()
        if proc.returncode == 0 and text:
            found.append(Path(text.splitlines()[0].strip()))
    return found


def _scan_roots() -> list[Path]:
    roots = [
        Path.home() / "Library/Java/JavaVirtualMachines",
        Path("/Library/Java/JavaVirtualMachines"),
        Path("/usr/lib/jvm"),
        Path("/usr/lib64/jvm"),
        Path("/opt/java"),
        Path("/opt/homebrew/opt/openjdk"),
        Path("/opt/homebrew/opt/openjdk@21"),
        Path("/opt/homebrew/opt/openjdk@17"),
        Path("/usr/local/opt/openjdk"),
        Path("/usr/local/opt/openjdk@21"),
    ]
    for cellar in (Path("/opt/homebrew/Cellar"), Path("/usr/local/Cellar")):
        if not cellar.is_dir():
            continue
        try:
            for child in sorted(cellar.glob("openjdk*")):
                roots.append(child)
                try:
                    for ver in sorted(child.iterdir()):
                        roots.append(ver)
                except OSError:
                    continue
        except OSError:
            continue
    found: list[Path] = []
    for root in roots:
        if not root.exists():
            continue
        found.append(root)
        if root.is_dir() and (root / "Contents").is_dir():
            found.append(root)
            continue
        if root.is_dir():
            try:
                for child in sorted(root.iterdir()):
                    found.append(child)
            except OSError:
                continue
    return found


def resolve_host_jdk() -> Path | None:
    """Return a real JDK home or None. Does not mutate os.environ."""
    candidates: list[Path] = []
    candidates.extend(_from_env())
    candidates.extend(_from_java_home_helper())
    candidates.extend(_scan_roots())
    which_javac = shutil.which("javac")
    if which_javac:
        javac = Path(which_javac)
        try:
            javac = javac.resolve()
        except OSError:
            pass
        if str(javac) not in {"/usr/bin/javac", "/bin/javac"}:
            candidates.append(javac.parent.parent)
    seen: set[str] = set()
    for raw in candidates:
        key = str(raw)
        if key in seen:
            continue
        seen.add(key)
        home = _accepted(raw)
        if home is not None:
            return home
    return None


def jdk_bins(home: Path | None = None) -> tuple[Path, Path] | None:
    home = home or resolve_host_jdk()
    if home is None:
        return None
    java = home / "bin" / "java"
    javac = home / "bin" / "javac"
    if not java.is_file() or not javac.is_file():
        return None
    return java, javac
