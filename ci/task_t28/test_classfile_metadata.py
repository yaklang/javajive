"""Structural metadata regressions; no JVM execution is involved."""
import random
import struct
import unittest

from tools.generators_holdout.classfile import class_enclosing_info


def u2(n):
    return struct.pack(">H", n)


def utf8(s):
    b = s.encode("utf-8")
    return b"\x01" + u2(len(b)) + b


def fixture(*, extra=(), attrs=(), this=2):
    cp = [utf8("demo/Foo"), b"\x07\x00\x01", utf8("java/lang/Object"), b"\x07\x00\x03", *extra]
    header = bytes.fromhex("cafebabe00000034") + u2(len(cp) + 1) + b"".join(cp)
    body = u2(0x21) + u2(this) + u2(4) + u2(0) + u2(0) + u2(0) + u2(len(attrs))
    return header + body + b"".join(u2(name) + struct.pack(">I", len(data)) + data for name, data in attrs)


class TestClassfileMetadata(unittest.TestCase):
    def assertInvalid(self, data):
        info = class_enclosing_info(data)
        self.assertEqual(info["kind"], "invalid", info)
        self.assertIsNone(info["this_name"])
        self.assertTrue(info["error"])

    def test_valid_fixture_and_every_truncation(self):
        data = fixture()
        self.assertEqual(class_enclosing_info(data)["this_name"], "demo.Foo")
        for n in range(len(data)):
            self.assertInvalid(data[:n])
        self.assertInvalid(data + b"\0")

    def test_self_cycle_wrong_tag_and_indirect_class_chain(self):
        self.assertInvalid(bytes.fromhex("cafebabe0000003400020700010000000100000000000000000000"))
        self.assertInvalid(fixture(extra=[b"\x07\x00\x06", b"\x07\x00\x05"]))
        self.assertInvalid(fixture(extra=[b"\x07\x00\x02"]))
        self.assertInvalid(fixture(extra=[b"\x07\xff\xff"]))
        self.assertInvalid(fixture(this=1))  # Utf8 is not CONSTANT_Class.
        self.assertInvalid(fixture(extra=[b"\x08\x00\x01"], this=5))

    def test_attribute_type_bounds_and_enclosing_references(self):
        self.assertInvalid(fixture(extra=[utf8("InnerClasses")], attrs=[(5, u2(1))]))
        self.assertInvalid(fixture(extra=[utf8("InnerClasses")], attrs=[(5, u2(0) + b"\0")]))
        self.assertInvalid(fixture(extra=[utf8("EnclosingMethod")], attrs=[(5, u2(4))]))
        self.assertInvalid(fixture(extra=[utf8("EnclosingMethod")], attrs=[(5, u2(1) + u2(0))]))
        self.assertInvalid(fixture(attrs=[(2, b"")]))  # attribute_name must be Utf8.
        data = fixture(extra=[utf8("Unknown")], attrs=[(5, b"1234")])
        self.assertInvalid(data[:-1])

    def test_enclosing_relation_cycles_are_invalid(self):
        entries = [utf8("InnerClasses"), utf8("demo/Other"), b"\x07\x00\x06"]
        records = u2(2) + u2(2) + u2(7) + u2(0) + u2(0) + u2(7) + u2(2) + u2(0) + u2(0)
        self.assertInvalid(fixture(extra=entries, attrs=[(5, records)]))

    def test_reserved_wide_slot_and_strict_modified_utf8(self):
        self.assertInvalid(fixture(extra=[b"\x05" + b"\0" * 8]))
        for bad in (b"\x00", b"\xc1\x81", b"\xf0\x9f\x98\x80", b"\xe0\x80\x80"):
            self.assertInvalid(fixture(extra=[b"\x01" + u2(len(bad)) + bad]))
        data = fixture(extra=[b"\x01\x00\x02\xc0\x80", b"\x01\x00\x06\xed\xa0\xbd\xed\xb8\x80"])
        self.assertEqual(class_enclosing_info(data)["kind"], "top_level")

    def test_seeded_metadata_mutations_never_raise(self):
        rng = random.Random(20260922)
        original = fixture(extra=[utf8("InnerClasses")], attrs=[(5, u2(0))])
        for _ in range(1000):
            data = bytearray(original)
            for _ in range(1 + rng.randrange(5)):
                data[rng.randrange(len(data))] = rng.randrange(256)
            info = class_enclosing_info(bytes(data))
            self.assertIn(info["kind"], {"invalid", "top_level", "nested_member", "nested_local"})


if __name__ == "__main__":
    unittest.main()
