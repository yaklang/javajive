"""python3 -m tools.milestone_ledger  |  python3 tools/milestone_ledger/__main__.py"""

from __future__ import annotations

import sys
from pathlib import Path


def _main() -> int:
    if __package__ in (None, ""):
        sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
        from milestone_ledger.cli import main
    else:
        from .cli import main
    return main()


if __name__ == "__main__":
    raise SystemExit(_main())
