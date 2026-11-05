#!/usr/bin/env python3
"""Run one dex2jar probe through the repository isolation worker."""

from __future__ import annotations

import argparse
import json
import shutil
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
if str(REPO) not in sys.path:
    sys.path.insert(0, str(REPO))

from tools.sandbox_worker import Limits, SandboxWorker  # noqa: E402


def read_bounded(path: Path, limit: int) -> bytes:
    if path.is_symlink() or not path.is_file():
        raise ValueError(f"not a regular staged input: {path}")
    with path.open("rb") as stream:
        data = stream.read(limit + 1)
    if len(data) > limit:
        raise ValueError(f"staged input exceeds {limit} bytes: {path}")
    return data


def copy_artifacts(root: Path, output: Path, limit: int) -> list[str]:
    output.mkdir(mode=0o700)
    copied: list[str] = []
    for source in sorted(root.iterdir()):
        if source.is_symlink() or not source.is_file():
            continue
        if source.name != "output.jar" and source.suffix.lower() != ".zip":
            continue
        data = read_bounded(source, limit)
        target = output / source.name
        target.write_bytes(data)
        copied.append(source.name)
    return copied


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--request", type=Path, required=True)
    parser.add_argument("--response", type=Path, required=True)
    parser.add_argument("--artifacts", type=Path, required=True)
    args = parser.parse_args()

    request = json.loads(read_bounded(args.request, 1 << 20))
    files: dict[str, bytes] = {}
    for item in request["inputs"]:
        files[item["name"]] = read_bounded(Path(item["path"]), int(item["limit"]))

    classpath = ":".join(f"$INPUTS/{name}" for name in request["classpath"])
    input_name = request["input_name"]
    script = f"""#!/bin/sh
set -u
INPUTS=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ARTIFACTS=$(CDPATH= cd -- "$INPUTS/../artifacts" && pwd)
exec java -Xms32m -Xmx512m -XX:MaxMetaspaceSize=256m \\
  -Djava.awt.headless=true -Dfile.encoding=UTF-8 \\
  -classpath "{classpath}" com.googlecode.dex2jar.tools.Dex2jarCmd \\
  --force -o "$ARTIFACTS/output.jar" "$INPUTS/{input_name}"
"""
    limits = Limits(
        memory_bytes=1024 * 1024 * 1024,
        pids=64,
        cpu_seconds=max(1, int(request["timeout_seconds"])),
        cpus="1",
        output_bytes=int(request["output_cap"]),
        artifact_bytes=512 * 1024 * 1024,
        fsize_bytes=256 * 1024 * 1024,
        timeout_seconds=float(request["timeout_seconds"]),
        nofile=256,
    )
    worker = SandboxWorker()
    observation = worker._run_script(  # Repository-owned bridge to the worker's path-aware launcher.
        "run.sh", script, limits, need_java=True, extra_files=files
    )
    extra = observation.extra or {}
    artifact_root = Path(extra["artifact_root"]) if extra.get("artifact_root") else None
    copied = copy_artifacts(artifact_root, args.artifacts, 256 * 1024 * 1024) if artifact_root else []
    cleanup_verified = bool(
        extra.get("leftover_verified") is True
        and not extra.get("leftover_query_failed")
        and not extra.get("leftover_cleanup_failed")
        and not observation.leftover_host_pids
        and not observation.leftover_containers
    )
    response = {
        "status": observation.status,
        "reason": observation.reason,
        "backend": observation.backend,
        "policy_digest": observation.policy_digest,
        "did_execute": observation.did_execute,
        "cleanup_verified": cleanup_verified,
        "exit_code": observation.exit_code,
        "timed_out": observation.timed_out,
        "output_limited": observation.output_capped or observation.artifact_capped,
        "stdout": observation.stdout or "",
        "stderr": observation.stderr or "",
        "artifacts": copied,
    }
    args.response.write_text(json.dumps(response, sort_keys=True) + "\n", encoding="utf-8")
    if artifact_root:
        shutil.rmtree(artifact_root.parent, ignore_errors=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
