#!/usr/bin/env python3
"""Generate bounded invocation metadata from installed, trusted JDK classfiles.
Reads ZIP/classfile bytes only; does not load or execute any Java code.
"""
import argparse
import hashlib
import json
from pathlib import Path
import re
import struct
import zipfile


class Reader:
    def __init__(self, data):
        self.data, self.offset = data, 0

    def take(self, n):
        out = self.data[self.offset:self.offset+n]
        if len(out) != n:
            raise ValueError('truncated classfile')
        self.offset += n
        return out

    def u1(self):
        return self.take(1)[0]

    def u2(self):
        return struct.unpack('>H', self.take(2))[0]

    def u4(self):
        return struct.unpack('>I', self.take(4))[0]


def declarations(data):
    r = Reader(data)
    if r.u4() != 0xCAFEBABE:
        raise ValueError('invalid classfile magic')
    minor, major = r.u2(), r.u2()
    pool = [None] * r.u2()
    i = 1
    while i < len(pool):
        tag = r.u1()
        if tag == 1:
            # The metadata identifiers/descriptors consumed here are ASCII;
            # unrelated modified-UTF8 string constants are not interpreted.
            pool[i] = r.take(r.u2())
        elif tag in (7, 8, 16, 19, 20):
            pool[i] = r.u2()
        elif tag in (3, 4, 9, 10, 11, 12, 17, 18):
            r.take(4)
        elif tag in (5, 6):
            r.take(8)
            i += 1
        elif tag == 15:
            r.take(3)
        else:
            raise ValueError('unknown constant-pool tag ' + str(tag))
        i += 1
    def utf(index):
        return pool[index].decode('ascii')
    def cls(index):
        return utf(pool[index])
    flags, own, parent = r.u2(), r.u2(), r.u2()
    parents = [cls(parent)] if parent else []
    parents += [cls(r.u2()) for _ in range(r.u2())]
    def attributes():
        names = []
        for _ in range(r.u2()):
            names.append(utf(r.u2()))
            r.take(r.u4())
        return names
    methods = []
    for fields in (True, False):
        for _ in range(r.u2()):
            access, name, desc = r.u2(), utf(r.u2()), utf(r.u2())
            attrs = attributes()
            if not fields and name not in ('<init>', '<clinit>'):
                methods.append({'Name': name, 'Desc': desc, 'Public': bool(access & 1),
                                'Static': bool(access & 8), 'Generic': 'Signature' in attrs,
                                'Varargs': bool(access & 0x80), 'Bridge': bool(access & 0x40)})
    attributes()
    if r.offset != len(data):
        raise ValueError('trailing classfile data')
    return {'Name': cls(own), 'Parents': parents, 'Methods': methods,
            'Public': bool(flags & 1), 'IsInterface': bool(flags & 0x200),
            'MembersComplete': True, 'ParentsComplete': True}, major


def profile(release, archive, prefix, jdk_version):
    with zipfile.ZipFile(archive) as source:
        classes, provenance = {}, {}
        roots = ['java/lang/Object', 'java/lang/String', 'java/util/Map']
        if release >= 16:
            roots.append('java/lang/Record')
        pending = list(roots)
        while pending:
            name = pending.pop(0)
            if name in classes:
                continue
            entry = prefix + name + '.class'
            raw = source.read(entry)
            cls, major = declarations(raw)
            if cls['Name'] != name or major != release + 44:
                raise ValueError('profile identity/version mismatch ' + name)
            classes[name] = cls
            provenance[name] = {'entry': entry, 'sha256': hashlib.sha256(raw).hexdigest(), 'major': major}
            pending.extend(cls['Parents'])
            # Include source-denotability metadata for every reference type in
            # the root method descriptors, plus each type's complete ancestry.
            # Do not claim that every JDK type or all transitive API references
            # are covered: absent catalog entries remain unavailable.
            if name in roots:
                for method in cls['Methods']:
                    pending.extend(re.findall(r'L([^;]+);', method['Desc']))
        return {'release': release, 'jdk_version': jdk_version,
                'archive_sha256': hashlib.sha256(Path(archive).read_bytes()).hexdigest(),
                'classes': {k: classes[k] for k in sorted(classes)},
                'provenance': {k: provenance[k] for k in sorted(provenance)}}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--jdk8', type=Path, required=True)
    parser.add_argument('--jdk11', type=Path, required=True)
    parser.add_argument('--jdk17', type=Path)
    parser.add_argument('--jdk21', type=Path, required=True)
    parser.add_argument('--out', type=Path, default=Path(__file__).with_suffix('.json'))
    args = parser.parse_args()
    def version(home):
        files = [home / name for name in ('release', 'version.txt', 'commitId.txt') if (home / name).is_file()]
        if not files:
            raise ValueError('Missing JDK version provenance')
        return {p.name: p.read_text(encoding='utf-8').strip() for p in files}
    profiles = [profile(8, args.jdk8 / 'jre/lib/rt.jar', '', version(args.jdk8))]
    for release, home in ((11, args.jdk11), (17, args.jdk17), (21, args.jdk21)):
        if home is not None:
            profiles.append(profile(release, home / 'jmods/java.base.jmod', 'classes/', version(home)))
    result = {'schema': 1, 'roots': ['java/lang/Object', 'java/lang/String', 'java/util/Map', 'java/lang/Record (16+)'],
              'profiles': profiles}
    args.out.write_text(json.dumps(result, sort_keys=True, separators=(',', ':')) + '\n', encoding='utf-8')


if __name__ == '__main__':
    main()
