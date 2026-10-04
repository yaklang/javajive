#!/usr/bin/env python3
"""Generate bounded invocation metadata from installed, trusted JDK classfiles.
Reads ZIP/classfile bytes only; does not load or execute any Java code.
"""
import argparse
from contextlib import ExitStack
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
        attrs = {}
        for _ in range(r.u2()):
            name = utf(r.u2())
            if name in attrs: raise ValueError('duplicate classfile attribute ' + name)
            raw = r.take(r.u4())
            if name == 'Signature':
                if len(raw) != 2: raise ValueError('invalid Signature attribute')
                attrs[name] = utf(struct.unpack('>H', raw)[0])
            elif name == 'Exceptions':
                er = Reader(raw)
                attrs[name] = [cls(er.u2()) for _ in range(er.u2())]
                if er.offset != len(raw): raise ValueError('invalid Exceptions attribute')
            else:
                attrs[name] = None
        return attrs
    methods = []
    for fields in (True, False):
        for _ in range(r.u2()):
            access, name, desc = r.u2(), utf(r.u2()), utf(r.u2())
            attrs = attributes()
            if not fields and name != '<clinit>':
                methods.append({'Name': name, 'Desc': desc, 'Public': bool(access & 1),
                                'Static': bool(access & 8), 'Generic': 'Signature' in attrs,
                                'Varargs': bool(access & 0x80), 'Bridge': bool(access & 0x40),
                                'ExceptionsKnown': True, 'Exceptions': attrs.get('Exceptions', [])})
                if attrs.get('Signature'):
                    methods[-1]['Signature'] = attrs['Signature']
    class_attrs = attributes()
    if r.offset != len(data):
        raise ValueError('trailing classfile data')
    return {**({'Signature': class_attrs['Signature']} if class_attrs.get('Signature') else {}), 'Name': cls(own), 'Parents': parents, 'Methods': methods,
            'Public': bool(flags & 1), 'IsInterface': bool(flags & 0x200), 'Final': bool(flags & 0x10),
            'MembersComplete': True, 'ParentsComplete': True}, major


class PlatformArchives:
    """An exact classfile namespace across modules of one trusted JDK image."""
    def __init__(self, archives):
        self.archives = [Path(p) for p in archives]

    def __enter__(self):
        self.stack = ExitStack()
        self.entries = {}
        try:
            for path in self.archives:
                archive = self.stack.enter_context(zipfile.ZipFile(path))
                for entry in archive.namelist():
                    if not entry.endswith('.class') or entry.endswith('/module-info.class'):
                        continue
                    if entry in self.entries:
                        raise ValueError('ambiguous platform classfile ' + entry)
                    self.entries[entry] = (path, archive)
        except BaseException:
            self.stack.close()
            raise
        return self

    def __exit__(self, *args):
        return self.stack.__exit__(*args)

    def read(self, entry):
        return self.entries[entry][1].read(entry)

    def namelist(self):
        return sorted(self.entries)

    def provenance(self, entry, raw, major):
        return {'archive': self.entries[entry][0].name, 'entry': entry,
                'sha256': hashlib.sha256(raw).hexdigest(), 'major': major}


def profile(release, archive, prefix, jdk_version, modules=()):
    with PlatformArchives([archive, *modules]) as source:
        classes, provenance = {}, {}
        roots = ['java/lang/Object', 'java/lang/String', 'java/lang/Runnable', 'java/io/FilterInputStream', 'java/util/Map',
                 'java/util/Optional', 'java/util/Collections', 'java/util/Arrays',
                 'java/util/Iterator', 'java/util/EnumMap', 'java/util/List',
                 'java/util/Set', 'java/util/Collection', 'java/util/Stack', 'java/util/stream/Stream',
                 'java/util/stream/Collectors', 'java/util/function/Function',
                 'java/util/function/Consumer', 'java/util/function/Supplier',
                 'java/util/function/Predicate',
                 # Complete namespaces for standard I/O, channels, reflection,
                 # concurrent and TLS extension points. They are declarations,
                 # never guessed uniqueness or exception rules.
                 'java/io/FileFilter', 'java/io/FilenameFilter', 'java/io/Reader',
                 'java/io/Writer', 'java/io/Externalizable',
                 'java/nio/file/PathMatcher', 'java/nio/file/FileVisitor',
                 'java/nio/channels/ReadableByteChannel',
                 'java/nio/channels/SeekableByteChannel',
                 'java/lang/reflect/InvocationHandler', 'java/lang/ClassLoader',
                 'java/lang/Thread', 'java/util/LinkedHashMap',
                 'java/util/concurrent/Callable',
                 'java/util/concurrent/atomic/AtomicInteger',
                 'java/util/concurrent/atomic/AtomicReference',
                 'javax/net/ssl/SSLEngine', 'javax/net/ssl/KeyManagerFactorySpi',
                 'javax/net/ssl/TrustManagerFactorySpi',
                 # Standard extension namespaces can live outside java.base.
                 # Read their actual declarations from the same JDK image.
                 'java/security/PrivilegedAction',
                 'java/beans/PropertyEditorSupport',
                 'javax/naming/spi/ObjectFactory', 'javax/sql/DataSource',
                 'org/xml/sax/helpers/DefaultHandler',
                 'org/xml/sax/ext/LexicalHandler', 'org/xml/sax/XMLReader']
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
            provenance[name] = source.provenance(entry, raw, major)
            pending.extend(cls['Parents'])
            # Include source-denotability metadata for every reference type in
            # the root method descriptors, plus each type's complete ancestry.
            # Do not claim that every JDK type or all transitive API references
            # are covered: absent catalog entries remain unavailable.
            if name in roots:
                for method in cls['Methods']:
                    pending.extend(re.findall(r'L([^;]+);', method['Desc']))
        # Exception classification needs actual ancestry, not an exception-name
        # suffix or a small guessed list. Read all declarations in this pinned
        # platform archive, then retain only the closed Throwable descendant
        # graph. This is ancestry evidence, never a complete member table.
        declarations_by_name, source_entries = {}, {}
        for entry in source.namelist():
            if not entry.startswith(prefix) or not entry.endswith('.class'):
                continue
            raw = source.read(entry)
            cls, major = declarations(raw)
            declarations_by_name[cls['Name']] = cls
            # Pinned images can retain older precompiled implementation stubs.
            # Keep their real version; never certify code newer than the image.
            if not 45 <= major <= release + 44:
                raise ValueError('platform classfile version mismatch ' + entry)
            source_entries[cls['Name']] = source.provenance(entry, raw, major)
        rooted, active = {}, set()
        def throwable(name):
            if name == 'java/lang/Throwable':
                return True
            if name in rooted:
                return rooted[name]
            if name in active or name not in declarations_by_name:
                return False
            active.add(name)
            result = any(throwable(parent) for parent in declarations_by_name[name]['Parents'])
            active.remove(name)
            rooted[name] = result
            return result
        exception_names = sorted(name for name in declarations_by_name if throwable(name))
        ancestors, pending = set(exception_names), list(exception_names)
        while pending:
            name = pending.pop()
            for parent in declarations_by_name[name]['Parents']:
                if parent not in declarations_by_name:
                    raise ValueError('missing exception ancestor ' + parent)
                if parent not in ancestors:
                    ancestors.add(parent)
                    pending.append(parent)
        exception_names = sorted(ancestors)
        return {'release': release, 'jdk_version': jdk_version,
                'archive_sha256': hashlib.sha256(Path(archive).read_bytes()).hexdigest(),
                'archives': {p.name: hashlib.sha256(p.read_bytes()).hexdigest()
                             for p in source.archives},
                'classes': {k: classes[k] for k in sorted(classes)},
                'provenance': {k: provenance[k] for k in sorted(provenance)},
                'throwable_hierarchy': {name: declarations_by_name[name]['Parents'] for name in exception_names},
                'throwable_provenance': {name: source_entries[name] for name in exception_names}}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--jdk8', type=Path, required=True)
    parser.add_argument('--jdk9', type=Path)
    parser.add_argument('--jdk11', type=Path, required=True)
    parser.add_argument('--jdk16', type=Path)
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
    for release, home in ((9, args.jdk9), (11, args.jdk11), (16, args.jdk16), (17, args.jdk17), (21, args.jdk21)):
        if home is not None:
            archives = sorted((home / 'jmods').glob('*.jmod'))
            base = home / 'jmods/java.base.jmod'
            profiles.append(profile(release, base, 'classes/', version(home),
                                    [p for p in archives if p != base]))
    result = {'schema': 2, 'roots': ['Object/String/Map/Record and standard collection, stream, functional, I/O, channel, reflection, concurrency, TLS, XML, JavaBeans, naming and SQL APIs; see generator roots'],
              'profiles': profiles}
    args.out.write_text(json.dumps(result, sort_keys=True, separators=(',', ':')) + '\n', encoding='utf-8')


if __name__ == '__main__':
    main()
