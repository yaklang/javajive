import struct
import tempfile
import unittest
import zipfile
from pathlib import Path
from jdk_metadata_catalog_generate import declarations, PlatformArchives


class PlatformArchiveTest(unittest.TestCase):
    def test_exact_cross_module_lookup_and_provenance(self):
        with tempfile.TemporaryDirectory() as directory:
            paths = [Path(directory) / name for name in ('java.base.jmod', 'java.xml.jmod')]
            for path, entry, data in zip(paths, ('classes/java/lang/Object.class', 'classes/org/xml/sax/XMLReader.class'), (b'base', b'xml')):
                with zipfile.ZipFile(path, 'w') as archive:
                    archive.writestr(entry, data)
                    archive.writestr('classes/module-info.class', b'module')
            with PlatformArchives(paths) as source:
                self.assertEqual(source.namelist(), ['classes/java/lang/Object.class', 'classes/org/xml/sax/XMLReader.class'])
                self.assertEqual(source.read('classes/org/xml/sax/XMLReader.class'), b'xml')
                self.assertEqual(source.provenance('classes/org/xml/sax/XMLReader.class', b'xml', 52)['archive'], 'java.xml.jmod')
                with self.assertRaises(KeyError):
                    source.read('classes/missing/Declaration.class')

    def test_ambiguous_original_classfiles_fail_closed(self):
        with tempfile.TemporaryDirectory() as directory:
            paths = [Path(directory) / name for name in ('first.jmod', 'second.jmod')]
            for path in paths:
                with zipfile.ZipFile(path, 'w') as archive:
                    archive.writestr('classes/p/Owner.class', b'original')
            with self.assertRaisesRegex(ValueError, 'ambiguous platform classfile'):
                with PlatformArchives(paths):
                    self.fail('duplicate classfile accepted')


class CatalogDeclarationTest(unittest.TestCase):
    def fixture(self, exceptions=None):
        u2 = lambda v: struct.pack(">H", v)
        utf = lambda s: b"\x01" + u2(len(s)) + s.encode("ascii")
        pool = [utf("p/C"), b"\x07"+u2(1), utf("java/lang/Object"), b"\x07"+u2(3),
                utf("<init>"), utf("()V"), utf("read"), utf("Exceptions"),
                utf("java/io/IOException"), b"\x07"+u2(9)]
        header = bytes.fromhex("cafebabe00000034") + u2(11) + b"".join(pool)
        header += u2(0x21)+u2(2)+u2(4)+u2(0)+u2(0)+u2(2)
        ctor = u2(1)+u2(5)+u2(6)+u2(0)
        if exceptions is None:
            attrs = u2(0)
        else:
            attrs = u2(1)+u2(8)+struct.pack(">I",len(exceptions))+exceptions
        return header+ctor+u2(1)+u2(7)+u2(6)+attrs+u2(0)

    def test_exact_constructor_and_known_empty_exceptions(self):
        cls, major = declarations(self.fixture())
        self.assertEqual(major,52)
        self.assertEqual([(m["Name"],m["Desc"]) for m in cls["Methods"]],[("<init>","()V"),("read","()V")])
        self.assertTrue(all(m["ExceptionsKnown"] and m["Exceptions"]==[] for m in cls["Methods"]))

    def test_original_exceptions_attribute_identity(self):
        cls, _ = declarations(self.fixture(bytes.fromhex("0001000a")))
        self.assertEqual(cls["Methods"][1]["Exceptions"],["java/io/IOException"])
        self.assertEqual(cls["Methods"][0]["Exceptions"],[])

    def test_malformed_exceptions_are_not_known_empty(self):
        for attr in (b"",bytes.fromhex("0001"),bytes.fromhex("00000000")):
            with self.subTest(attr=attr),self.assertRaises(ValueError):
                declarations(self.fixture(attr))


if __name__ == "__main__":
    unittest.main()
