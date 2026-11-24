package dexparser

import (
	"crypto/sha1"
	"encoding/binary"
	"hash/adler32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/dex2jar/model"
)

func TestParseExternalDEX038And039(t *testing.T) {
	dex038Path := filepath.Join("..", "..", "testdata", "external", "dex2jar-dex038.dex")
	raw038, err := os.ReadFile(dex038Path)
	if err != nil {
		t.Fatalf("read dex038: %v", err)
	}
	f038, err := Parse(raw038, "dex2jar-dex038.dex")
	if err != nil {
		t.Fatalf("Parse(dex038): %v", err)
	}
	if f038.Header.Version != "038" {
		t.Fatalf("dex038 Version = %q, want 038", f038.Header.Version)
	}
	if len(f038.CallSites) != 2 || len(f038.MethodHandles) != 2 {
		t.Fatalf("dex038 CallSites=%d MethodHandles=%d, want 2 and 2", len(f038.CallSites), len(f038.MethodHandles))
	}
	if f038.CallSites[0].MethodName == "" || len(f038.CallSites[1].Arguments) == 0 {
		t.Fatalf("unexpected decoded call sites: %#v", f038.CallSites)
	}

	dex039Path := filepath.Join("..", "..", "testdata", "external", "dex2jar-dex039.dex")
	raw039, err := os.ReadFile(dex039Path)
	if err != nil {
		t.Fatalf("read dex039: %v", err)
	}
	f039, err := Parse(raw039, "dex2jar-dex039.dex")
	if err != nil {
		t.Fatalf("Parse(dex039): %v", err)
	}
	if f039.Header.Version != "039" {
		t.Fatalf("dex039 Version = %q, want 039", f039.Header.Version)
	}
	if len(f039.MethodHandles) != 9 {
		t.Fatalf("dex039 MethodHandles=%d, want 9", len(f039.MethodHandles))
	}
	seenKind := make(map[model.MethodHandleKind]bool)
	for _, mh := range f039.MethodHandles {
		seenKind[mh.Kind] = true
	}
	for kind := model.MethodHandleStaticPut; kind <= model.MethodHandleInvokeInterface; kind++ {
		if !seenKind[kind] {
			t.Fatalf("dex039 missing MethodHandle kind %#x", kind)
		}
	}
}

func TestParseDEX040SyntheticMethodHandleCallSiteAndVersion041Rejection(t *testing.T) {
	basePath := filepath.Join("..", "..", "testdata", "external", "dex2jar-dex038.dex")
	raw, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("read base fixture: %v", err)
	}

	resign := func(b []byte) {
		sig := sha1.Sum(b[32:])
		copy(b[12:32], sig[:])
		binary.LittleEndian.PutUint32(b[8:12], adler32.Checksum(b[12:]))
	}

	v040 := append([]byte(nil), raw...)
	copy(v040[4:8], "040\x00")
	resign(v040)
	parsed040, err := Parse(v040, "classes.dex")
	if err != nil {
		t.Fatalf("Parse(DEX 040): %v", err)
	}
	if parsed040.Header.Version != "040" || len(parsed040.CallSites) != 2 || len(parsed040.MethodHandles) != 2 {
		t.Fatalf("DEX 040 parsed = %#v", parsed040.Header.Version)
	}

	v042 := append([]byte(nil), raw...)
	copy(v042[4:8], "042\x00")
	resign(v042)
	if _, err := Parse(v042, "classes.dex"); err == nil || !strings.Contains(err.Error(), `unsupported DEX version "042"`) {
		t.Fatalf("Parse(DEX 042) error = %v, want unsupported DEX version 042", err)
	}

	// Mutate first method_handle_item handle_type to invalid 0x0009.
	mapOff := int(binary.LittleEndian.Uint32(v040[52:56]))
	mapSize := int(binary.LittleEndian.Uint32(v040[mapOff : mapOff+4]))
	mhOff := -1
	for i := 0; i < mapSize; i++ {
		itemOff := mapOff + 4 + i*12
		itemType := binary.LittleEndian.Uint16(v040[itemOff : itemOff+2])
		if itemType == 0x0008 {
			mhOff = int(binary.LittleEndian.Uint32(v040[itemOff+8 : itemOff+12]))
		}
	}
	if mhOff < 0 {
		t.Fatal("method_handle map item missing in fixture")
	}
	badKind := append([]byte(nil), v040...)
	binary.LittleEndian.PutUint16(badKind[mhOff:mhOff+2], 0x0009)
	resign(badKind)
	if _, err := Parse(badKind, "classes.dex"); err == nil || !strings.Contains(err.Error(), "unsupported method_handle_type") {
		t.Fatalf("Parse(bad method_handle_type) error = %v, want unsupported method_handle_type", err)
	}

	// Duplicate map item 0x0007 (call_site_id) by rewriting a trailing entry or appending a new map_list.
	dupMap := make([]byte, len(v040)+4+12*3)
	copy(dupMap, v040)
	newMapOff := uint32(len(v040))
	for newMapOff%4 != 0 {
		dupMap = append(dupMap, 0)
		newMapOff++
	}
	binary.LittleEndian.PutUint32(dupMap[len(dupMap)-40:], 3) // map count=3
	base := len(dupMap) - 40
	// header_item
	binary.LittleEndian.PutUint16(dupMap[base+4:], 0x0000)
	binary.LittleEndian.PutUint32(dupMap[base+8:], 1)
	binary.LittleEndian.PutUint32(dupMap[base+12:], 0)
	// duplicate call_site_id x2
	binary.LittleEndian.PutUint16(dupMap[base+16:], 0x0007)
	binary.LittleEndian.PutUint32(dupMap[base+20:], 1)
	binary.LittleEndian.PutUint32(dupMap[base+24:], uint32(mhOff))
	binary.LittleEndian.PutUint16(dupMap[base+28:], 0x0007)
	binary.LittleEndian.PutUint32(dupMap[base+32:], 1)
	binary.LittleEndian.PutUint32(dupMap[base+36:], uint32(mhOff))
	binary.LittleEndian.PutUint32(dupMap[32:36], uint32(len(dupMap)))
	binary.LittleEndian.PutUint32(dupMap[52:56], newMapOff)
	resign(dupMap)
	if _, err := Parse(dupMap, "classes.dex"); err == nil || !strings.Contains(err.Error(), "duplicate call_site_id section") {
		t.Fatalf("Parse(duplicate call_site_id) error = %v, want duplicate call_site_id section", err)
	}
}
