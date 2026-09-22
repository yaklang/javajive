#!/usr/bin/env python3
"""Build the current adapter and bind its binary hash to the source revision."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

REPO = Path(__file__).resolve().parents[2]


def git(*args):
    return subprocess.check_output(['git', *args], cwd=REPO)


def source_state():
    return {'revision': git('rev-parse', 'HEAD').decode().strip(),
            'working_tree': git('status', '--porcelain').decode(),
            'diff_sha256': hashlib.sha256(git('diff', '--binary', 'HEAD')).hexdigest()}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    out = args.out.resolve()
    before = source_state()
    command = ['go', 'build', '-trimpath', '-buildvcs=true', '-o', str(out), './tools/draft-followup-probe']
    subprocess.run(command, cwd=REPO, check=True)
    if source_state() != before:
        raise SystemExit('Source tree changed during adapter build; rebuild before auditing')
    manifest = {**before, 'binary_sha256': hashlib.sha256(out.read_bytes()).hexdigest(),
                'command': command, 'go': subprocess.check_output(['go', 'version']).decode().strip()}
    Path(str(out) + '.build.json').write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')


if __name__ == '__main__':
    main()
