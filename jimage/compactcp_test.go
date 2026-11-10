package jimage

import (
	"bytes"
	"testing"
)

func compactCPHeader() []byte {
	return []byte{
		0xca, 0xfe, 0xba, 0xbe, // magic
		0x00, 0x00, // minor
		0x00, 0x34, // major 52
		0x00, 0x0a, // constant pool count 10
	}
}

func compactCPTail() []byte {
	return []byte{
		0x00, 0x21, 0x00, 0x01, 0x00, 0x02, 0x00, 0x03,
		0x00, 0x04, 0x00, 0x05, 0x00, 0x06, 0x00, 0x07,
	}
}

func appendUTF8Entry(destination []byte, value string) []byte {
	destination = append(destination, constantUTF8, byte(len(value)>>8), byte(len(value)))
	return append(destination, value...)
}

func compactCPFixture() (compressed, expected []byte, reader *Reader) {
	header := compactCPHeader()
	tail := compactCPTail()

	compressed = append([]byte{}, header...)
	// #1: externalized "Hello" (string table offset 1).
	compressed = append(compressed, compactCPExternalizedString, 0xA1)
	// #2: externalized descriptor "(L;I)V" (offset 7), two indexes:
	// package "java/lang" (offset 21) and class "String" (offset 14).
	compressed = append(compressed, compactCPExternalizedDescriptor, 0xA7, 0xA2, 0xB5, 0xAE)
	// #3: inline UTF8 "raw".
	compressed = append(compressed, constantUTF8, 0x00, 0x03, 'r', 'a', 'w')
	// #4: CONSTANT_Class.
	compressed = append(compressed, 7, 0x00, 0x01)
	// #5: CONSTANT_Long (occupies slots 5 and 6).
	compressed = append(compressed, 5, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88)
	// #7: CONSTANT_NameAndType.
	compressed = append(compressed, 12, 0x00, 0x01, 0x00, 0x02)
	// #8: CONSTANT_InvokeDynamic.
	compressed = append(compressed, 18, 0x00, 0x03, 0x00, 0x04)
	// #9: CONSTANT_Module.
	compressed = append(compressed, 19, 0x00, 0x05)
	compressed = append(compressed, tail...)

	expected = append([]byte{}, header...)
	expected = appendUTF8Entry(expected, "Hello")
	expected = appendUTF8Entry(expected, "(Ljava/lang/String;I)V")
	expected = appendUTF8Entry(expected, "raw")
	expected = append(expected, 7, 0x00, 0x01)
	expected = append(expected, 5, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88)
	expected = append(expected, 12, 0x00, 0x01, 0x00, 0x02)
	expected = append(expected, 18, 0x00, 0x03, 0x00, 0x04)
	expected = append(expected, 19, 0x00, 0x05)
	expected = append(expected, tail...)

	stringsData := []byte{0}
	stringsData = append(stringsData, []byte("Hello")...)
	stringsData = append(stringsData, 0)
	stringsData = append(stringsData, []byte("(L;I)V")...)
	stringsData = append(stringsData, 0)
	stringsData = append(stringsData, []byte("String")...)
	stringsData = append(stringsData, 0)
	stringsData = append(stringsData, []byte("java/lang")...)
	stringsData = append(stringsData, 0)

	reader = &Reader{strings: stringsData, maxResourceSize: defaultMaxResourceSize}
	return compressed, expected, reader
}

func TestDecompressCompactCP(t *testing.T) {
	compressed, expected, reader := compactCPFixture()
	got, err := reader.decompressCompactCP(compressed, uint64(len(expected)), "test/Hello.class")
	if err != nil {
		t.Fatalf("decompressCompactCP: %v", err)
	}
	if !bytes.Equal(got, expected) {
		t.Fatalf("reconstructed class differs:\n got: % x\nwant: % x", got, expected)
	}
}

func TestDecompressCompactCPMalformed(t *testing.T) {
	compressed, expected, reader := compactCPFixture()
	cases := []struct {
		name string
		data []byte
		size uint64
	}{
		{"short header", compressed[:9], uint64(len(expected))},
		{"truncated constant pool", append(append([]byte{}, compressed[:10]...), 7), uint64(len(expected))},
		{"invalid tag", append(append([]byte{}, compressed[:10]...), 2, 0, 0), 12},
		{"string offset", append(append([]byte{}, compressed[:10]...), compactCPExternalizedString, 0xBF), 12},
		{"descriptor indexes truncated", append(append([]byte{}, compressed[:10]...), compactCPExternalizedDescriptor, 0xA7, 0xA5, 0xA0), 12},
		{"invalid compressed integer", append(append([]byte{}, compressed[:10]...), compactCPExternalizedString, 0x80), 12},
		{"length mismatch", compressed, uint64(len(expected) + 1)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := reader.decompressCompactCP(test.data, test.size, "test/Hello.class"); err == nil {
				t.Fatal("decompressCompactCP accepted malformed input")
			}
		})
	}
}

func TestDecompressInt(t *testing.T) {
	cases := []struct {
		name     string
		data     []byte
		want     int
		wantNext int
		wantErr  bool
	}{
		{"one byte zero", []byte{0xA0}, 0, 1, false},
		{"one byte value", []byte{0xA5}, 5, 1, false},
		{"two bytes", []byte{0xC1, 0x02}, 0x102, 2, false},
		{"three bytes", []byte{0xE1, 0x02, 0x03}, 0x10203, 3, false},
		{"uncompressed", []byte{0x00, 0x00, 0x01, 0x00}, 256, 4, false},
		{"invalid length", []byte{0x80}, 0, 0, true},
		{"truncated compressed", []byte{0xC1}, 0, 0, true},
		{"truncated uncompressed", []byte{0x00, 0x01}, 0, 0, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, next, err := decompressInt(test.data, 0)
			if test.wantErr {
				if err == nil {
					t.Fatalf("decompressInt(% x) unexpectedly succeeded", test.data)
				}
				return
			}
			if err != nil {
				t.Fatalf("decompressInt(% x): %v", test.data, err)
			}
			if got != test.want || next != test.wantNext {
				t.Fatalf("decompressInt(% x) = (%d, %d), want (%d, %d)", test.data, got, next, test.want, test.wantNext)
			}
		})
	}
}
