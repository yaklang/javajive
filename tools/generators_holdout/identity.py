"""Compiler identity: javac21 --release 8 is not real javac 8 and not ECJ."""

from __future__ import annotations

import hashlib
import json
import os
import re
import shutil
import subprocess
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any


PINNED_ECJ_VERSION = "3.37.0"
PINNED_ECJ_SHA256 = "cde026ff966b48b5e5f148b6f041ceff3cf4f85cf75155f4ec0f40e4ee14b545"


class InfraError(Exception):
    """Missing toolchain or failed compile infrastructure."""


@dataclass(frozen=True)
class CompilerIdentity:
    kind: str  # javac | ecj
    version: str
    release: int | None
    executable: str
    digest: str
    home: str
    label: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    def coverage_key(self) -> str:
        rel = "native" if self.release is None else f"release{self.release}"
        return f"{self.kind}|{self.version}|{rel}|{self.digest[:12]}"


def _sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def _run(argv: list[str], env: dict[str, str] | None = None, timeout: float = 30) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        argv,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        env=env,
        timeout=timeout,
        check=False,
    )


def _javac_version(javac: Path) -> str:
    proc = _run([str(javac), "-version"])
    text = (proc.stderr or "") + (proc.stdout or "")
    m = re.search(r"javac\s+([0-9._]+)", text)
    if not m:
        raise InfraError(f"cannot parse javac version from {javac}: {text!r}")
    return m.group(1)


def _homes_from_env(spec: str) -> list[Path]:
    keys: list[str]
    if spec.startswith("1.8") or spec == "8":
        keys = ["JAVA8_HOME", "JAVA_HOME_8", "JDK8_HOME"]
    else:
        keys = ["JAVA21_HOME", "JAVA_HOME_21", "JAVA_HOME"]
    out = []
    for key in keys:
        raw = os.environ.get(key)
        if raw:
            out.append(Path(raw))
    return out


def _scan_jvm_homes() -> list[Path]:
    roots = [
        Path.home() / "Library/Java/JavaVirtualMachines",
        Path("/Library/Java/JavaVirtualMachines"),
        Path("/usr/lib/jvm"),
        Path("/usr/lib64/jvm"),
        Path("/opt/java"),
    ]
    homes: list[Path] = []
    for root in roots:
        if not root.is_dir():
            continue
        for child in sorted(root.iterdir()):
            for rel in ("Contents/Home", ""):
                home = child / rel if rel else child
                if (home / "bin" / "javac").is_file():
                    homes.append(home)
    return homes


def _match_spec(ver: str, spec: str) -> bool:
    if spec.startswith("1.8") or spec == "8":
        return ver.startswith("1.8")
    return ver.startswith(spec)


def _java_home_for_version(spec: str) -> Path | None:
    """Portable discovery. No machine-specific hardcoded user paths."""
    candidates = list(_homes_from_env(spec))
    helper = Path("/usr/libexec/java_home")
    if helper.is_file():
        proc = _run([str(helper), "-v", spec])
        if proc.returncode == 0 and proc.stdout.strip():
            candidates.append(Path(proc.stdout.strip()))
    candidates.extend(_scan_jvm_homes())
    which = shutil.which("javac")
    if which:
        candidates.append(Path(which).resolve().parent.parent)
    env_home = os.environ.get("JAVA_HOME")
    if env_home:
        candidates.append(Path(env_home))
    seen: set[str] = set()
    for home in candidates:
        key = str(home)
        if key in seen:
            continue
        seen.add(key)
        javac = home / "bin" / "javac"
        if not javac.is_file():
            continue
        try:
            ver = _javac_version(javac)
        except InfraError:
            continue
        if _match_spec(ver, spec):
            return home
    return None


def _pin_path() -> Path:
    return Path(__file__).resolve().parent / "ecj.pin.json"


def load_ecj_pin() -> dict[str, str]:
    return json.loads(_pin_path().read_text(encoding="utf-8"))


def find_ecj_jar() -> Path | None:
    pin = load_ecj_pin()
    expected = pin["sha256"]
    env = os.environ.get("ECJ_JAR")
    candidates = []
    if env:
        candidates.append(Path(env))
    here = Path(__file__).resolve().parent
    candidates.append(here / "testdata" / pin["filename"])
    candidates.append(here / pin["filename"])
    m2 = Path.home() / ".m2/repository/org/eclipse/jdt/ecj" / pin["version"] / pin["filename"]
    candidates.append(m2)
    for path in candidates:
        if path.is_file() and _sha256_file(path) == expected:
            return path
    return None


def discover_compilers() -> dict[str, CompilerIdentity | None]:
    """Return named identities. Missing tools are None, never aliased."""
    out: dict[str, CompilerIdentity | None] = {
        "javac21_release8": None,
        "javac8_native": None,
        "ecj_pinned": None,
    }
    home21 = _java_home_for_version("21")
    if home21 is None:
        which = shutil.which("javac")
        if which:
            home21 = Path(which).resolve().parent.parent
    if home21 is not None:
        javac = home21 / "bin" / "javac"
        if javac.is_file():
            ver = _javac_version(javac)
            if not ver.startswith("1.8"):
                out["javac21_release8"] = CompilerIdentity(
                    kind="javac",
                    version=ver,
                    release=8,
                    executable=str(javac),
                    digest=_sha256_file(javac),
                    home=str(home21),
                    label=f"javac {ver} --release 8",
                )
    home8 = _java_home_for_version("1.8")
    if home8 is not None:
        javac = home8 / "bin" / "javac"
        ver = _javac_version(javac)
        if ver.startswith("1.8"):
            out["javac8_native"] = CompilerIdentity(
                kind="javac",
                version=ver,
                release=None,
                executable=str(javac),
                digest=_sha256_file(javac),
                home=str(home8),
                label=f"javac {ver} native source/target 8",
            )
    jar = find_ecj_jar()
    if jar is not None:
        java = shutil.which("java") or "java"
        out["ecj_pinned"] = CompilerIdentity(
            kind="ecj",
            version=PINNED_ECJ_VERSION,
            release=8,
            executable=str(jar),
            digest=PINNED_ECJ_SHA256,
            home=str(jar.parent),
            label=f"ecj {PINNED_ECJ_VERSION} -source 8 -target 8 sha256={PINNED_ECJ_SHA256[:12]}",
        )
        _ = java
    return out


def compile_sources(
    identity: CompilerIdentity,
    sources: list[Path],
    dest: Path,
    *,
    debug: str = "nodebug",
    timeout: float = 120,
) -> dict[str, Any]:
    dest.mkdir(parents=True, exist_ok=True)
    debug_flag = "-g" if debug == "debug" else "-g:none"
    env = os.environ.copy()
    if identity.kind == "javac":
        env["JAVA_HOME"] = identity.home
        argv = [identity.executable, "-proc:none", "-encoding", "UTF-8", debug_flag, "-d", str(dest)]
        if identity.release is not None:
            argv.extend(["--release", str(identity.release)])
        else:
            argv.extend(["-source", "8", "-target", "8"])
        argv.extend(str(p) for p in sources)
    elif identity.kind == "ecj":
        java = shutil.which("java")
        if not java:
            raise InfraError("java runtime missing; cannot invoke ECJ")
        argv = [
            java,
            "-jar",
            identity.executable,
            "-source",
            "8",
            "-target",
            "8",
            "-encoding",
            "UTF-8",
            "-proc:none",
            debug_flag,
            "-d",
            str(dest),
        ]
        argv.extend(str(p) for p in sources)
    else:
        raise InfraError(f"unsupported compiler kind {identity.kind}")
    try:
        proc = _run(argv, env=env, timeout=timeout)
    except (OSError, subprocess.TimeoutExpired) as exc:
        return {
            "argv": argv,
            "rc": 127,
            "stdout": "",
            "stderr": f"infra_error:{exc}",
            "identity": identity.to_dict(),
            "debug": debug,
            "dest": str(dest),
        }
    return {
        "argv": argv,
        "rc": proc.returncode,
        "stdout": proc.stdout,
        "stderr": proc.stderr,
        "identity": identity.to_dict(),
        "debug": debug,
        "dest": str(dest),
    }


def java_command(identity: CompilerIdentity | None = None) -> str:
    if identity is not None:
        candidate = Path(identity.home) / "bin" / "java"
        if candidate.is_file():
            return str(candidate)
    return shutil.which("java") or "java"


def classify_java_process(ran: dict[str, Any]) -> str:
    """Explicit stage: verify_fail / linkage_error / run_fail / run_ok / infra_error."""
    stderr = ran.get("stderr") or ""
    stdout = ran.get("stdout") or ""
    blob = stderr + "\n" + stdout
    if ran.get("timeout"):
        return "infra_error"
    if "VerifyError" in blob:
        return "verify_fail"
    if any(
        tok in blob
        for tok in (
            "ClassNotFoundException",
            "NoClassDefFoundError",
            "UnsatisfiedLinkError",
            "NoSuchMethodError",
            "UnsupportedClassVersionError",
        )
    ):
        return "linkage_error"
    if ran.get("rc") == 0:
        return "run_ok"
    if ran.get("rc") is None:
        return "infra_error"
    return "run_fail"


def verify_and_run(
    class_dir: Path,
    class_name: str,
    *,
    java_bin: str | None = None,
    extra_cp: list[Path] | None = None,
    timeout: float = 20,
) -> dict[str, Any]:
    java = java_bin or shutil.which("java") or "java"
    cp = os.pathsep.join([str(class_dir), *([str(p) for p in extra_cp] if extra_cp else [])])
    argv = [
        java,
        "-Xmx192m",
        "-XX:ActiveProcessorCount=2",
        "-Xverify:all",
        "-Dfile.encoding=UTF-8",
        "-Duser.language=en",
        "-Duser.country=US",
        "-cp",
        cp,
        class_name,
    ]
    try:
        proc = _run(argv, timeout=timeout)
        timed_out = False
    except subprocess.TimeoutExpired as exc:
        timed_out = True
        proc = exc
    if timed_out:
        payload = {
            "argv": argv,
            "rc": None,
            "stdout": "",
            "stderr": "timeout",
            "timeout": True,
            "verified_and_ran": False,
        }
        payload["stage"] = classify_java_process(payload)
        return payload
    payload = {
        "argv": argv,
        "rc": proc.returncode,
        "stdout": proc.stdout,
        "stderr": proc.stderr,
        "timeout": False,
        "verified_and_ran": proc.returncode == 0,
        "classpath": cp,
    }
    payload["stage"] = classify_java_process(payload)
    return payload
