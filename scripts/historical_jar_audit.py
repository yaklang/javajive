#!/usr/bin/env python3
"""Reproduce the pinned historical corpus and fail on differential regressions.

Observation completion is not acceptance. Even a clean comparison only checks
the recorded compile/stub/verifier surfaces; it does not prove runtime equality.
"""
import argparse
import concurrent.futures
import hashlib
import json
from pathlib import Path
import re
import shutil
import sys
import urllib.request
import zipfile

LOCK = Path(__file__).resolve().parents[1] / "test/cross/testdata/historical-jars.lock.json"
CAPTURE_COPY_SUFFIX = re.compile(r"\b(var\d+)_f\d+\b")


def read(path):
    return json.loads(path.read_text())


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n")


def prepare(args):
    lock = read(LOCK)
    root = args.cache.resolve()

    def fetch(item):
        rel, expected = item
        dst = root / rel
        dst.parent.mkdir(parents=True, exist_ok=True)
        if not dst.exists():
            tmp = dst.with_suffix(".download")
            local = Path.home() / ".m2/repository" / rel
            if local.is_file() and hashlib.sha256(local.read_bytes()).hexdigest() == expected:
                shutil.copyfile(local, tmp)
            else:
                with urllib.request.urlopen("https://repo.maven.apache.org/maven2/" + rel, timeout=90) as response:
                    with tmp.open("wb") as output:
                        shutil.copyfileobj(response, output)
            if hashlib.sha256(tmp.read_bytes()).hexdigest() != expected:
                tmp.unlink()
                raise ValueError("download hash mismatch: " + rel)
            tmp.replace(dst)
        if hashlib.sha256(dst.read_bytes()).hexdigest() != expected:
            raise ValueError("cached hash mismatch: " + rel)
        with zipfile.ZipFile(dst) as archive:
            if not any(n.endswith(".class") for n in archive.namelist()):
                raise ValueError("no class entries: " + rel)

    with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
        list(pool.map(fetch, lock["artifacts"].items()))
    manifest = {"artifactRoot": str(root), "artifacts": lock["artifacts"], "jars": {}}
    for name, spec in lock["jars"].items():
        manifest["jars"][name] = {
            "path": str(root / spec["path"]),
            "sha256": lock["artifacts"][spec["path"]],
            "deps": [str(root / p) for p in spec["deps"]],
        }
    save(args.manifest, manifest)
    print(f"Verified {len(lock['jars'])} targets and {len(lock['artifacts'])} artifacts; manifest: {args.manifest}")
    return 0


def compiler_errors(directory):
    # Source lines shift between revisions. Compare the unit and javac diagnostic,
    # retaining multiplicity so identical new failures cannot hide in a set. The
    # `_fN` suffix is a generated final-capture copy, not a source identifier;
    # its ordinal changes when an unrelated capture is added earlier in a method.
    from collections import Counter
    def diagnostic_key(match):
        diagnostic = CAPTURE_COPY_SUFFIX.sub(r"\1_f", match.group(2))
        return match.group(1), diagnostic
    return Counter(diagnostic_key(m) for m in re.finditer(
        r"/sources/(.*?\.java):\d+: error: ([^\n]+)", (directory / "javac.log").read_text()))


def compare(args):
    lock = read(LOCK)
    rows = []
    for name, spec in lock["jars"].items():
        old_dir, new_dir = args.baseline / name, args.candidate / name
        old, new = read(old_dir / "observation.json"), read(new_dir / "observation.json")
        expected = lock["artifacts"][spec["path"]]
        if not old["Completed"] or not new["Completed"]:
            raise ValueError("incomplete observation: " + name)
        if old["InputSHA256"] != expected or new["InputSHA256"] != expected:
            raise ValueError("incomparable input: " + name)
        if old["Compiler"] != new["Compiler"]:
            raise ValueError("different javac versions: " + name)
        added = compiler_errors(new_dir) - compiler_errors(old_dir)
        stubs = {unit: count - old["StubUnits"].get(unit, 0)
                 for unit, count in new["StubUnits"].items() if count > old["StubUnits"].get(unit, 0)}
        missing = sorted(set(old["SourceHashes"]) - set(new["SourceHashes"]))
        reasons = []
        if old["CompileSucceeded"] and not new["CompileSucceeded"]:
            reasons.append("lost clean compilation")
        if added:
            reasons.append("new compiler diagnostics")
        if stubs:
            reasons.append("new stub methods")
        if missing or new["DecompileFailures"] > old["DecompileFailures"]:
            reasons.append("lost decompiled source")
        # Comparing partial javac output is misleading. Check verifier degradation
        # only when both entire compilations succeeded, with nonempty output.
        if old["CompileSucceeded"] and new["CompileSucceeded"]:
            if new["RebuiltVerifyFail"] > old["RebuiltVerifyFail"]:
                reasons.append("more JVM verification failures")
            if not new["RebuiltClasses"]:
                reasons.append("empty rebuilt artifact")
        fields = ("CompileSucceeded", "CompilerErrors", "StubMethods", "DecompileFailures", "InputClasses", "SourceUnits", "RebuiltVerifyOK", "RebuiltVerifyFail")
        row = {"jar": name, "baseline": {k: old[k] for k in fields},
               "candidate": {k: new[k] for k in fields}, "reasons": reasons,
               "new_errors": [{"unit": u, "diagnostic": d, "count": c} for (u, d), c in sorted(added.items())],
               "new_stubs": stubs, "missing_sources": missing}
        rows.append(row)
        print(f"{name:15} errors {old['CompilerErrors']:3}->{new['CompilerErrors']:3} "
              f"stubs {old['StubMethods']:2}->{new['StubMethods']:2} " + (", ".join(reasons) or "no recorded regression"))
    regressed = sum(bool(row["reasons"]) for row in rows)
    result = {"targets": len(rows), "regressed": regressed, "rows": rows,
              "baseline": str(args.baseline.resolve()), "candidate": str(args.candidate.resolve()),
              "runtime_equivalence_proven": False}
    save(args.output, result)
    print(f"{len(rows)} targets compared; {regressed} with recorded regressions. Runtime equivalence is not established.")
    return 1 if regressed else 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    p = commands.add_parser("prepare")
    p.add_argument("--cache", type=Path, required=True)
    p.add_argument("--manifest", type=Path, required=True)
    p.set_defaults(run=prepare)
    p = commands.add_parser("compare")
    p.add_argument("--baseline", type=Path, required=True)
    p.add_argument("--candidate", type=Path, required=True)
    p.add_argument("--output", type=Path, required=True)
    p.set_defaults(run=compare)
    args = parser.parse_args()
    try:
        return args.run(args)
    except (OSError, ValueError, KeyError, zipfile.BadZipFile) as error:
        print(f"Audit could not complete: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
