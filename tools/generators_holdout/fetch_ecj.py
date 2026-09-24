#!/usr/bin/env python3
"""Download the pinned ECJ jar and verify sha256. Fail closed on mismatch."""

from __future__ import annotations

import hashlib
import json
import sys
import urllib.request
from pathlib import Path

HERE = Path(__file__).resolve().parent
PIN = json.loads((HERE / "ecj.pin.json").read_text(encoding="utf-8"))


def main() -> int:
    dest = Path(sys.argv[1]) if len(sys.argv) > 1 else HERE / "testdata" / PIN["filename"]
    dest.parent.mkdir(parents=True, exist_ok=True)
    if dest.is_file() and hashlib.sha256(dest.read_bytes()).hexdigest() == PIN["sha256"]:
        print(dest)
        return 0
    tmp = dest.with_suffix(".download")
    with urllib.request.urlopen(PIN["url"], timeout=90) as response, tmp.open("wb") as out:
        out.write(response.read())
    digest = hashlib.sha256(tmp.read_bytes()).hexdigest()
    if digest != PIN["sha256"]:
        tmp.unlink(missing_ok=True)
        print(f"infra_error: ECJ hash mismatch got {digest} want {PIN['sha256']}", file=sys.stderr)
        return 2
    tmp.replace(dest)
    print(dest)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
