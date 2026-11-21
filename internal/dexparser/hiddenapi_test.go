package dexparser

import (
	"crypto/sha1"
	"encoding/binary"
	"hash/adler32"
	"strings"
	"testing"
)

func putTestU32(b []byte, off int, v uint32) {
	binary.LittleEndian.PutUint32(b[off:off+4], v)
}

func putTestU16(b []byte, off int, v uint16) {
	binary.LittleEndian.PutUint16(b[off:off+2], v)
}

func testDEXWithMapItem(itemType uint16) []byte {
	const headerSize = 0x70
	const mapSize = 4 + 2*12
	dataOff := uint32(headerSize)
	mapOff := dataOff
	total := int(headerSize) + mapSize
	data := make([]byte, total)

	copy(data[:8], []byte("dex\n035\x00"))
	putTestU32(data, 32, uint32(total))
	putTestU32(data, 36, headerSize)
	putTestU32(data, 40, 0x12345678)
	putTestU32(data, 52, mapOff)
	putTestU32(data, 104, uint32(total)-dataOff)
	putTestU32(data, 108, dataOff)

	// map_list at mapOff: 2 entries (header_item + itemType)
	putTestU32(data, int(mapOff), 2)
	putTestU16(data, int(mapOff)+4, 0x0000) // header_item
	putTestU32(data, int(mapOff)+8, 1)
	putTestU32(data, int(mapOff)+12, 0)
	putTestU16(data, int(mapOff)+16, itemType)
	putTestU32(data, int(mapOff)+20, 1)
	putTestU32(data, int(mapOff)+24, mapOff)

	sig := sha1.Sum(data[32:])
	copy(data[12:32], sig[:])
	putTestU32(data, 8, adler32.Checksum(data[12:]))
	return data
}

func TestParseAllowsHiddenAPIMapItemAndRejectsCallSite(t *testing.T) {
	hidden := testDEXWithMapItem(0xf000)
	if _, err := Parse(hidden, "classes.dex"); err != nil {
		t.Fatalf("Parse(hiddenapi_class_data 0xf000) unexpected error: %v", err)
	}

	callSite := testDEXWithMapItem(0x0007)
	if _, err := Parse(callSite, "classes.dex"); err == nil || strings.Contains(err.Error(), "unsupported call_site_id") {
		t.Fatalf("Parse(call_site_id 0x0007 truncated) error = %v, want table parse error", err)
	}

	unknown := testDEXWithMapItem(0x9999)
	if _, err := Parse(unknown, "classes.dex"); err == nil || !strings.Contains(err.Error(), "unsupported section type=0x9999") {
		t.Fatalf("Parse(unknown 0x9999) error = %v, want unsupported section type=0x9999", err)
	}
}
