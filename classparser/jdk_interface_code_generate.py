#!/usr/bin/env python3
"""Retain original interface declarations from the catalog's pinned primary archives.

Reads ZIP/classfile data only. Existing constructor entries retain exact bytes.
The archive map is a JSON object from catalog release to local rt.jar/base.jmod.
"""
import argparse
import hashlib
import json
from pathlib import Path
import zipfile


def extend(catalog_path, bundle_path, archives):
    catalog = json.loads(catalog_path.read_text())
    with zipfile.ZipFile(bundle_path) as bundle:
        entries = {name: bundle.read(name) for name in bundle.namelist()}
    manifest = json.loads(entries.pop('manifest.json'))
    profiles = {p['release']: p for p in manifest['profiles']}
    assert manifest['schema'] == 1
    assert set(profiles) == {p['release'] for p in catalog['profiles']}
    for p in catalog['profiles']:
        release = p['release']
        original = profiles[release]
        assert original['archive_sha256'] == p['archive_sha256']
        archive = Path(archives[str(release)])
        assert hashlib.sha256(archive.read_bytes()).hexdigest() == p['archive_sha256']
        with zipfile.ZipFile(archive) as source:
            queue = sorted(n for n, c in p['classes'].items()
                           if c['IsInterface'] and p['provenance'][n]['archive'] == archive.name)
            seen = set()
            while queue:
                name = queue.pop(0)
                if name in seen:
                    continue
                seen.add(name)
                provenance = p['provenance'][name]
                assert provenance['archive'] == archive.name
                raw = source.read(provenance['entry'])
                digest = hashlib.sha256(raw).hexdigest()
                assert digest == provenance['sha256']
                key = f'{release}/{name}.class'
                if key in entries:
                    assert entries[key] == raw
                entries[key] = raw
                original['classes'][name] = digest
                queue.extend(p['classes'][name]['Parents'])
    entries['manifest.json'] = json.dumps(manifest, sort_keys=True, separators=(',', ':')).encode()
    temporary = bundle_path.with_suffix('.tmp')
    with zipfile.ZipFile(temporary, 'w', compression=zipfile.ZIP_DEFLATED) as bundle:
        for name, raw in sorted(entries.items()):
            info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o100644 << 16
            bundle.writestr(info, raw)
    temporary.replace(bundle_path)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--archives', type=Path, required=True)
    parser.add_argument('--catalog', type=Path, default=Path(__file__).with_name('jdk_metadata_catalog.json'))
    parser.add_argument('--bundle', type=Path, default=Path(__file__).with_name('jdk_constructor_code.zip'))
    args = parser.parse_args()
    extend(args.catalog, args.bundle, json.loads(args.archives.read_text()))
