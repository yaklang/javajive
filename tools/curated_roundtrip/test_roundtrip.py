"""Hermetic runner failure classification, using only tiny reviewed processes."""
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from tools.curated_roundtrip import build_adapter
from tools.curated_roundtrip import roundtrip


class RunnerContractTests(unittest.TestCase):
    def test_build_manifest_fingerprints_untracked_source_content(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            subprocess.run(['git', 'init', '-q'], cwd=repo, check=True)
            subprocess.run(['git', 'config', 'user.email', 'test@example.invalid'], cwd=repo, check=True)
            subprocess.run(['git', 'config', 'user.name', 'Test'], cwd=repo, check=True)
            (repo / 'tracked.go').write_text('package tracked\n', encoding='utf-8')
            subprocess.run(['git', 'add', 'tracked.go'], cwd=repo, check=True)
            subprocess.run(['git', 'commit', '-qm', 'fixture'], cwd=repo, check=True)
            source = repo / 'new.go'
            source.write_text('package new\nvar value = 1\n', encoding='utf-8')
            previous_repo = build_adapter.REPO
            build_adapter.REPO = repo
            try:
                first = build_adapter.source_state()
                source.write_text('package new\nvar value = 2\n', encoding='utf-8')
                second = build_adapter.source_state()
                evidence = repo / 'artifacts' / 'roundtrip'
                evidence.mkdir(parents=True)
                before_evidence = build_adapter.source_state(exclude=evidence)
                (evidence / 'report.json').write_text('{}\n', encoding='utf-8')
                after_evidence = build_adapter.source_state(exclude=evidence)
            finally:
                build_adapter.REPO = previous_repo
            self.assertNotEqual(first['untracked_sha256'], second['untracked_sha256'])
            self.assertEqual(before_evidence['untracked_sha256'], after_evidence['untracked_sha256'])

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
        self.assertEqual(len(specs), 10)
        self.assertEqual(len({spec['case'] for spec in specs}), 10)
        for spec in specs:
            self.assertTrue(list((roundtrip.ROOT / 'fixtures' / spec['case']).glob('*.java')))
            self.assertTrue((roundtrip.ROOT / 'fixtures' / spec['case'] / (spec['main'] + '.java')).is_file())
