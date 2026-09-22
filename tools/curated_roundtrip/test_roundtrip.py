"""Hermetic runner failure classification, using only tiny reviewed processes."""
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from tools.curated_roundtrip import roundtrip


class RunnerContractTests(unittest.TestCase):
    def test_process_failures_are_not_success(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            missing = roundtrip.execute([root / 'missing-executable'], root, root / 'missing')
            self.assertEqual(missing['exit_code'], 127)
            timed = roundtrip.execute([sys.executable, '-c', 'import time; time.sleep(1)'], root, root / 'timeout', timeout=.02)
            self.assertEqual(timed['limit'], 'timeout')
            self.assertNotEqual(timed['exit_code'], 0)
            output = roundtrip.execute([sys.executable, '-c', 'print("x" * (3 << 20))'], root, root / 'output')
            self.assertEqual(output['limit'], 'output_limit')
            self.assertNotEqual(output['exit_code'], 0)

    def test_missing_selection_is_an_error(self):
        run = subprocess.run([sys.executable, roundtrip.__file__, '--adapter', __file__,
                              '--out', 'unused', '--case', 'missing-case',
                              '--allow-trusted-fixture-execution'], capture_output=True, text=True)
        self.assertNotEqual(run.returncode, 0)
        self.assertIn('No cases selected', run.stderr)

    def test_full_matrix_contains_reviewed_sources(self):
        specs = json.loads((roundtrip.ROOT / 'fixtures.json').read_text())
        self.assertEqual(len(specs), 9)
        self.assertEqual(len({spec['case'] for spec in specs}), 9)
        for spec in specs:
            self.assertTrue(list((roundtrip.ROOT / 'fixtures' / spec['case']).glob('*.java')))
            self.assertTrue((roundtrip.ROOT / 'fixtures' / spec['case'] / (spec['main'] + '.java')).is_file())
