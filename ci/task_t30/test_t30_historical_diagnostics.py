"""Regression tests for stable historical-JAR compiler diagnostic keys."""

from __future__ import annotations

import importlib.util
from pathlib import Path
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "historical_jar_audit", ROOT / "scripts/historical_jar_audit.py"
)
assert SPEC is not None and SPEC.loader is not None
HISTORICAL_AUDIT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(HISTORICAL_AUDIT)


class HistoricalDiagnosticKeys(unittest.TestCase):
    def test_capture_copy_ordinal_does_not_create_a_false_regression(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            log = Path(tmp) / "javac.log"
            log.write_text(
                "/sources/p/C.java:10: error: no suitable method for f(var2_f4)\n"
                "/sources/p/C.java:20: error: no suitable method for f(var2_f7)\n"
            )
            errors = HISTORICAL_AUDIT.compiler_errors(Path(tmp))
        self.assertEqual(errors[("p/C.java", "no suitable method for f(var2_f)")], 2)

    def test_real_source_local_name_remains_part_of_diagnostic(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            log = Path(tmp) / "javac.log"
            log.write_text(
                "/sources/p/C.java:10: error: cannot find symbol var2\n"
                "/sources/p/C.java:20: error: cannot find symbol var3\n"
            )
            errors = HISTORICAL_AUDIT.compiler_errors(Path(tmp))
        self.assertEqual(errors[("p/C.java", "cannot find symbol var2")], 1)
        self.assertEqual(errors[("p/C.java", "cannot find symbol var3")], 1)


if __name__ == "__main__":
    unittest.main()
