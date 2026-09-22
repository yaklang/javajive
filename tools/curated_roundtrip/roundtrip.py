#!/usr/bin/env python3
"""Audit reviewed fixture round trips; subprocess timeouts are not a sandbox."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import time

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[1]
MAX_LOG = 2 << 20


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def execute(argv, cwd, logs, timeout=20):
    logs.parent.mkdir(parents=True, exist_ok=True)
    stdout, stderr = logs.with_suffix('.stdout'), logs.with_suffix('.stderr')
    start, reason, rc = time.monotonic(), None, 127
    env = {k: v for k, v in os.environ.items() if k in (
        'PATH', 'JAVA_HOME', 'HOME', 'TMPDIR', 'TEMP', 'TMP', 'SystemRoot', 'WINDIR')}
    env.update({'LC_ALL': 'C.UTF-8', 'LANG': 'C.UTF-8'})
    with stdout.open('wb') as out, stderr.open('wb') as err:
        try:
            process = subprocess.Popen([str(x) for x in argv], cwd=cwd, stdout=out,
                                       stderr=err, stdin=subprocess.DEVNULL, env=env,
                                       start_new_session=(os.name == 'posix'))
            while process.poll() is None:
                if time.monotonic() - start > timeout:
                    reason = 'timeout'
                if os.fstat(out.fileno()).st_size + os.fstat(err.fileno()).st_size > MAX_LOG:
                    reason = 'output_limit'
                if reason:
                    try:
                        if os.name == 'posix':
                            os.killpg(process.pid, signal.SIGKILL)
                        else:
                            process.kill()
                    except ProcessLookupError:
                        pass
                    break
                time.sleep(.02)
            rc = process.wait()
        except OSError as exc:
            err.write(str(exc).encode('utf-8', 'replace'))
        if os.fstat(out.fileno()).st_size + os.fstat(err.fileno()).st_size > MAX_LOG:
            reason = 'output_limit'
    def read(path):
        with path.open('rb') as stream:
            return stream.read(MAX_LOG).decode('utf-8', 'replace')
    return {'argv': [str(x) for x in argv], 'exit_code': 125 if reason else rc,
            'process_exit_code': rc, 'limit': reason,
            'seconds': time.monotonic() - start, 'stdout': read(stdout), 'stderr': read(stderr)}


def write_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=True, indent=2) + '\n', encoding='utf-8')


def parser_self_check(helper, out):
    """Ensure parse and attribution remain independent, using actual JDK tools."""
    directory = out / 'parser-self-check'
    directory.mkdir()
    syntax = directory / 'Syntax.java'
    syntax.write_text('class Syntax { void broken( { }', encoding='utf-8')
    types = directory / 'Types.java'
    types.write_text('class Types { int x = "wrong type"; }', encoding='utf-8')
    results = {
        'invalid_syntax': execute(['java', '-cp', helper, 'ParseOnly', syntax], directory, directory / 'syntax'),
        'valid_syntax_invalid_type': execute(['java', '-cp', helper, 'ParseOnly', types], directory, directory / 'parse-types'),
        'typecheck': execute(['javac', '-proc:none', '-classpath', directory, '-d', directory, types], directory, directory / 'typecheck'),
    }
    results['passed'] = (results['invalid_syntax']['exit_code'] == 1
                         and results['valid_syntax_invalid_type']['exit_code'] == 0
                         and results['typecheck']['exit_code'] == 1)
    return results


def audit_row(spec, debug, mode, original, compile_original, adapter, helper, directory):
    rebuilt, sources = directory / 'rebuilt', directory / 'sources'
    rebuilt.mkdir(parents=True)
    sources.mkdir()
    classes = sorted(original.rglob('*.class'))
    names = [p.relative_to(original).as_posix()[:-6].replace('/', '.') for p in classes]
    row = {'case': spec['case'], 'debug': debug, 'mode': mode,
           'original_compile': compile_original, 'decompile': [], 'failures': [],
           'original_class_sha256': {p.relative_to(original).as_posix(): sha256(p) for p in classes}}
    try:
        if compile_original['exit_code'] or not classes:
            raise ValueError('original compilation/fixture invalid')
        row['original_verify'] = execute(['java', '-Xverify:all', '-cp', str(helper) + os.pathsep + str(original), 'VerifyOnly', *names], directory, directory / 'original-verify')
        row['original_run'] = execute(['java', '-Xmx192m', '-Xverify:all', '-cp', original, spec['main']], directory, directory / 'original-run')
        if row['original_verify']['exit_code'] or row['original_run']['exit_code']:
            raise ValueError('original oracle failed')
        for index, cls in enumerate(classes):
            call = execute([adapter, '-mode', mode, '-resolver-root', original, cls], directory, directory / f'decompile-{index}')
            data = json.loads(call['stdout'])
            api = data.get('result', {})
            row['decompile'].append({'input': cls.relative_to(original).as_posix(), 'process': call, 'api': data})
            if call['exit_code'] or data.get('error'):
                row['failures'].append('decompile error:' + cls.name)
            if api.get('status') != 'complete' or api.get('stub_methods'):
                row['failures'].append('not complete:' + cls.name)
            source = api.get('source', '')
            target = sources / cls.relative_to(original).with_suffix('.java')
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(source, encoding='utf-8')
            if not source:
                row['failures'].append('empty source:' + cls.name)
        rebuilt_sources = sorted(sources.rglob('*.java'))
        row['parse'] = execute(['java', '-cp', helper, 'ParseOnly', *rebuilt_sources], directory, directory / 'parse')
        # The application classpath contains rebuilt classes only. Original
        # target classes are never available to rebuilt javac or runtime.
        row['rebuilt_compile'] = execute(['javac', '-proc:none', '-encoding', 'UTF-8', '--release', str(spec['release']), '-classpath', rebuilt, '-d', rebuilt, *rebuilt_sources], directory, directory / 'rebuilt-compile')
        if row['parse']['exit_code']:
            row['failures'].append('source parse failed')
        if row['rebuilt_compile']['exit_code']:
            raise ValueError('source typecheck/compile failed')
        missing = [name for name in names if not (rebuilt / (name.replace('.', '/') + '.class')).is_file()]
        if missing:
            raise ValueError('input class identities missing:' + ','.join(missing))
        row['rebuilt_class_sha256'] = {p.relative_to(rebuilt).as_posix(): sha256(p) for p in sorted(rebuilt.rglob('*.class'))}
        row['rebuilt_verify'] = execute(['java', '-Xverify:all', '-cp', str(helper) + os.pathsep + str(rebuilt), 'VerifyOnly', *names], directory, directory / 'rebuilt-verify')
        row['rebuilt_run'] = execute(['java', '-Xmx192m', '-Xverify:all', '-cp', rebuilt, spec['main']], directory, directory / 'rebuilt-run')
        if row['rebuilt_verify']['exit_code']:
            row['failures'].append('JVM verify failed')
        if row['rebuilt_run']['exit_code']:
            row['failures'].append('rebuilt runtime failed')
        for channel in ('stdout', 'stderr'):
            if row['original_run'][channel] != row['rebuilt_run'][channel]:
                row['failures'].append('behavior ' + channel + ' differs')
    except (ValueError, OSError, KeyError, TypeError) as exc:
        row['failures'].append(str(exc))
    row['passed'] = not row['failures']
    write_json(directory / 'observation.json', row)
    return row


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--adapter', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    parser.add_argument('--case')
    parser.add_argument('--expected-rows', type=int)
    parser.add_argument('--observe', action='store_true')
    parser.add_argument('--allow-trusted-fixture-execution', action='store_true')
    args = parser.parse_args()
    if not args.allow_trusted_fixture_execution:
        parser.error('Explicitly allow reviewed fixture execution; unknown inputs require the isolated sandbox worker.')
    specs = json.loads((ROOT / 'fixtures.json').read_text(encoding='utf-8'))
    specs = [spec for spec in specs if not args.case or spec['case'] == args.case]
    if not specs:
        parser.error('No cases selected')
    if len({spec['case'] for spec in specs}) != len(specs):
        parser.error('Duplicate fixture cases')
    if args.expected_rows is not None and len(specs) * 4 != args.expected_rows:
        parser.error('Selected matrix does not match expected row count')
    adapter, out = args.adapter.resolve(), args.out.resolve()
    if not adapter.is_file():
        parser.error('Adapter binary is missing')
    if out.exists():
        parser.error('Use a new output directory; existing evidence is never overwritten')
    out.mkdir(parents=True)
    helper = out / 'helper'
    helper.mkdir()
    result = {
        'scope': 'curated_runtime_cases', 'observe_only': args.observe, 'cases': [],
        'expected_rows': len(specs) * 4, 'adapter_sha256': sha256(adapter),
        'revision': execute(['git', 'rev-parse', 'HEAD'], REPO, out / 'revision'),
        'working_tree': execute(['git', 'status', '--porcelain'], REPO, out / 'working-tree'),
        'java': execute(['java', '-version'], out, out / 'java-version'),
        'javac': execute(['javac', '-version'], out, out / 'javac-version'),
        'go': execute(['go', 'version'], REPO, out / 'go-version'),
        'adapter_build': execute(['go', 'version', '-m', adapter], REPO, out / 'adapter-build'),
        'fixture_sha256': {p.relative_to(ROOT).as_posix(): sha256(p) for p in sorted((ROOT / 'fixtures').rglob('*.java'))},
        'helper_sha256': {p.name: sha256(p) for p in sorted((ROOT / 'helpers').glob('*.java'))},
        'manifest_sha256': sha256(ROOT / 'fixtures.json'),
    }
    write_json(out / 'report.json', result)
    built_revisions = [word.split('=', 1)[1] for word in result['adapter_build']['stdout'].split()
                       if word.startswith('vcs.revision=')]
    if (result['revision']['exit_code'] or result['adapter_build']['exit_code']
            or built_revisions != [result['revision']['stdout'].strip()]):
        raise SystemExit('Adapter build revision does not match checked-out HEAD; rebuild it with VCS metadata')
    result['prepare'] = execute(['javac', '-proc:none', '-encoding', 'UTF-8', '-d', helper, ROOT / 'helpers/VerifyOnly.java', ROOT / 'helpers/ParseOnly.java'], out, out / 'prepare')
    write_json(out / 'report.json', result)
    if result['prepare']['exit_code']:
        raise SystemExit('JDK helper compilation failed; report.json records failure')
    result['parser_self_check'] = parser_self_check(helper, out)
    write_json(out / 'report.json', result)
    if not result['parser_self_check']['passed']:
        raise SystemExit('Parser/typecheck self-check failed')
    for spec in specs:
        source_files = sorted((ROOT / 'fixtures' / spec['case']).glob('*.java'))
        if not source_files:
            raise SystemExit('Missing fixture sources: ' + spec['case'])
        for debug in ('debug', 'nodebug'):
            directory = out / spec['case'] / debug
            original = directory / 'original'
            original.mkdir(parents=True)
            compile_original = execute(['javac', '-proc:none', '-encoding', 'UTF-8', '--release', str(spec['release']), '-g' if debug == 'debug' else '-g:none', '-d', original, *source_files], directory, directory / 'original-compile')
            for mode in ('precision', 'compatibility'):
                row = audit_row(spec, debug, mode, original, compile_original, adapter, helper, directory / mode)
                result['cases'].append(row)
                write_json(out / 'report.json', result)
                print(spec['case'], debug, mode, 'PASS' if row['passed'] else 'FAIL', ','.join(row['failures']), flush=True)
    failures = sum(not row['passed'] for row in result['cases'])
    result['summary'] = {'total': len(result['cases']), 'failed': failures,
                         'passed': len(result['cases']) - failures,
                         'runtime_equivalence_for_all_inputs_proven': False}
    write_json(out / 'report.json', result)
    if len(result['cases']) != result['expected_rows']:
        raise SystemExit('Incomplete fixture matrix')
    raise SystemExit(0 if args.observe else int(failures > 0))


if __name__ == '__main__':
    main()
