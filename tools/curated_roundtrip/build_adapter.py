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


def source_state(exclude=None):
    excluded = Path(exclude).absolute() if exclude is not None else None
    untracked = git('ls-files', '--others', '--exclude-standard').decode().splitlines()
    untracked_hash = hashlib.sha256()
    for name in sorted(untracked):
        path = (REPO / name).absolute()
        if excluded is not None:
            try:
                path.relative_to(excluded)
                continue
            except ValueError:
                pass
        if not path.is_file():
            continue
        untracked_hash.update(name.encode('utf-8', 'surrogateescape'))
        untracked_hash.update(b'\0')
        untracked_hash.update(hashlib.sha256(path.read_bytes()).digest())
    return {'revision': git('rev-parse', 'HEAD').decode().strip(),
            'working_tree': git('status', '--porcelain').decode(),
            'diff_sha256': hashlib.sha256(git('diff', '--binary', 'HEAD')).hexdigest(),
            'untracked_sha256': untracked_hash.hexdigest()}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--out', type=Path, required=True)
    parser.add_argument('--ldflags', default='')
    args = parser.parse_args()
    out = args.out.resolve()
    before = source_state()
    command = ['go', 'build', '-trimpath', '-buildvcs=true']
    if args.ldflags:
        command.extend(['-ldflags', args.ldflags])
    command.extend(['-o', str(out), './tools/draft-followup-probe'])
    subprocess.run(command, cwd=REPO, check=True)
    if source_state() != before:
        raise SystemExit('Source tree changed during adapter build; rebuild before auditing')
    manifest = {**before, 'binary_sha256': hashlib.sha256(out.read_bytes()).hexdigest(),
                'command': command, 'go': subprocess.check_output(['go', 'version']).decode().strip()}
    Path(str(out) + '.build.json').write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')


if __name__ == '__main__':
    main()
