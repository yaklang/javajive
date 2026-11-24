package dexparser

import (
	"crypto/sha1"
	"encoding/binary"
	"hash/adler32"
	"strings"
	"testing"
)

func put041U16(b []byte, off int, v uint16) {
	binary.LittleEndian.PutUint16(b[off:off+2], v)
}

func put041U32(b []byte, off int, v uint32) {
	binary.LittleEndian.PutUint32(b[off:off+4], v)
}

func write041MapList(b []byte, mapOff int, items []struct {
	typ  uint16
	size uint32
	off  uint32
}) {
	put041U32(b, mapOff, uint32(len(items)))
	for i, it := range items {
		p := mapOff + 4 + i*12
		put041U16(b, p, it.typ)
		put041U32(b, p+4, it.size)
		put041U32(b, p+8, it.off)
	}
}

func seal041Section(b []byte, secStart, secEnd int) {
	sig := sha1.Sum(b[secStart+32 : secEnd])
	copy(b[secStart+12:secStart+32], sig[:])
	put041U32(b, secStart+8, adler32.Checksum(b[secStart+12:secEnd]))
}

func syntheticDEX041Single(descriptor string) []byte {
	stringsTable := []string{descriptor, "Ljava/lang/Object;"}
	const (
		stringIDsOff = headerSizeV041
		typeIDsOff   = stringIDsOff + 8
		classDefsOff = typeIDsOff + 8
		strStart     = classDefsOff + 32
	)
	data := make([]byte, strStart)
	strOffs := make([]uint32, len(stringsTable))
	for i, s := range stringsTable {
		strOffs[i] = uint32(len(data))
		data = append(data, byte(len(s)))
		data = append(data, s...)
		data = append(data, 0)
	}
	for len(data)%4 != 0 {
		data = append(data, 0)
	}
	mapOff := len(data)
	data = append(data, make([]byte, 4+5*12)...)
	write041MapList(data, mapOff, []struct {
		typ  uint16
		size uint32
		off  uint32
	}{
		{0x0000, 1, 0},
		{0x0001, 2, stringIDsOff},
		{0x0002, 2, typeIDsOff},
		{0x0006, 1, classDefsOff},
		{0x1000, 1, uint32(mapOff)},
	})

	total := uint32(len(data))
	copy(data[:8], "dex\n041\x00")
	put041U32(data, 32, total)
	put041U32(data, 36, headerSizeV041)
	put041U32(data, 40, 0x12345678)
	put041U32(data, 52, uint32(mapOff))
	put041U32(data, 56, 2)
	put041U32(data, 60, stringIDsOff)
	put041U32(data, 64, 2)
	put041U32(data, 68, typeIDsOff)
	put041U32(data, 96, 1)
	put041U32(data, 100, classDefsOff)
	put041U32(data, 112, total)
	put041U32(data, 116, 0)

	for i, off := range strOffs {
		put041U32(data, stringIDsOff+i*4, off)
		put041U32(data, typeIDsOff+i*4, uint32(i))
	}
	put041U32(data, classDefsOff, 0)
	put041U32(data, classDefsOff+4, 1)
	put041U32(data, classDefsOff+8, 1)
	put041U32(data, classDefsOff+16, ^uint32(0))

	seal041Section(data, 0, len(data))
	return data
}

func syntheticDEX041Container(desc0, desc1 string) []byte {
	const (
		sec0StrIDs  = headerSizeV041
		sec0TypeIDs = sec0StrIDs + 8
		sec0ClsDefs = sec0TypeIDs + 8
		sec0MapOff  = sec0ClsDefs + 32
		sec0End     = sec0MapOff + 4 + 5*12
	)
	sec1StrIDs := sec0End + headerSizeV041
	sec1TypeIDs := sec1StrIDs + 8
	sec1ClsDefs := sec1TypeIDs + 8
	sec1Data := sec1ClsDefs + 32

	data := make([]byte, sec1Data)
	// Store all string payloads in section 1 so section 0 exercises physical
	// container-relative cross-section offsets.
	offDesc0 := uint32(len(data))
	data = append(data, byte(len(desc0)))
	data = append(data, desc0...)
	data = append(data, 0)

	offDesc1 := uint32(len(data))
	data = append(data, byte(len(desc1)))
	data = append(data, desc1...)
	data = append(data, 0)

	const obj = "Ljava/lang/Object;"
	offObj := uint32(len(data))
	data = append(data, byte(len(obj)))
	data = append(data, obj...)
	data = append(data, 0)

	for len(data)%4 != 0 {
		data = append(data, 0)
	}
	sec1MapOff := len(data)
	data = append(data, make([]byte, 4+5*12)...)
	total := uint32(len(data))

	// Section 0
	copy(data[:8], "dex\n041\x00")
	put041U32(data, 32, uint32(sec0End))
	put041U32(data, 36, headerSizeV041)
	put041U32(data, 40, 0x12345678)
	put041U32(data, 52, uint32(sec0MapOff))
	put041U32(data, 56, 2)
	put041U32(data, 60, uint32(sec0StrIDs))
	put041U32(data, 64, 2)
	put041U32(data, 68, uint32(sec0TypeIDs))
	put041U32(data, 96, 1)
	put041U32(data, 100, uint32(sec0ClsDefs))
	put041U32(data, 112, total)
	put041U32(data, 116, 0)
	put041U32(data, sec0StrIDs, offDesc0)
	put041U32(data, sec0StrIDs+4, offObj)
	put041U32(data, sec0TypeIDs, 0)
	put041U32(data, sec0TypeIDs+4, 1)
	put041U32(data, sec0ClsDefs, 0)
	put041U32(data, sec0ClsDefs+4, 1)
	put041U32(data, sec0ClsDefs+8, 1)
	put041U32(data, sec0ClsDefs+16, ^uint32(0))
	write041MapList(data, sec0MapOff, []struct {
		typ  uint16
		size uint32
		off  uint32
	}{
		{0x0000, 1, 0},
		{0x0001, 2, uint32(sec0StrIDs)},
		{0x0002, 2, uint32(sec0TypeIDs)},
		{0x0006, 1, uint32(sec0ClsDefs)},
		{0x1000, 1, uint32(sec0MapOff)},
	})

	// Section 1
	copy(data[sec0End:sec0End+8], "dex\n041\x00")
	put041U32(data, sec0End+32, total-uint32(sec0End))
	put041U32(data, sec0End+36, headerSizeV041)
	put041U32(data, sec0End+40, 0x12345678)
	put041U32(data, sec0End+52, uint32(sec1MapOff))
	put041U32(data, sec0End+56, 2)
	put041U32(data, sec0End+60, uint32(sec1StrIDs))
	put041U32(data, sec0End+64, 2)
	put041U32(data, sec0End+68, uint32(sec1TypeIDs))
	put041U32(data, sec0End+96, 1)
	put041U32(data, sec0End+100, uint32(sec1ClsDefs))
	put041U32(data, sec0End+112, total)
	put041U32(data, sec0End+116, uint32(sec0End))
	put041U32(data, sec1StrIDs, offDesc1)
	put041U32(data, sec1StrIDs+4, offObj)
	put041U32(data, sec1TypeIDs, 0)
	put041U32(data, sec1TypeIDs+4, 1)
	put041U32(data, sec1ClsDefs, 0)
	put041U32(data, sec1ClsDefs+4, 1)
	put041U32(data, sec1ClsDefs+8, 1)
	put041U32(data, sec1ClsDefs+16, ^uint32(0))
	write041MapList(data, sec1MapOff, []struct {
		typ  uint16
		size uint32
		off  uint32
	}{
		{0x0000, 1, uint32(sec0End)},
		{0x0001, 2, uint32(sec1StrIDs)},
		{0x0002, 2, uint32(sec1TypeIDs)},
		{0x0006, 1, uint32(sec1ClsDefs)},
		{0x1000, 1, uint32(sec1MapOff)},
	})

	seal041Section(data, 0, sec0End)
	seal041Section(data, sec0End, len(data))
	return data
}

func TestParseDEX041SingleAndContainer(t *testing.T) {
	single := syntheticDEX041Single("Lexample/Single041;")
	file, err := Parse(single, "classes.dex")
	if err != nil {
		t.Fatalf("Parse(single 041): %v", err)
	}
	if file.Header.Version != "041" || file.Header.HeaderSize != headerSizeV041 ||
		file.Header.ContainerSize != uint32(len(single)) || file.Header.HeaderOffset != 0 {
		t.Fatalf("single 041 header = %+v", file.Header)
	}
	if len(file.Classes) != 1 || file.Classes[0].Descriptor != "Lexample/Single041;" {
		t.Fatalf("single 041 classes = %+v", file.Classes)
	}

	container := syntheticDEX041Container("Lexample/SecZero;", "Lexample/SecOne;")
	if _, err := Parse(container, "classes.dex"); err == nil || !strings.Contains(err.Error(), "use container parser") {
		t.Fatalf("Parse(multi-section container) error = %v, want container parser error", err)
	}
	files, err := ParseContainer(container, "classes.dex")
	if err != nil {
		t.Fatalf("ParseContainer(041): %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(files))
	}
	if files[0].Entry != "classes.dex" || files[0].Header.HeaderOffset != 0 || files[0].Classes[0].Descriptor != "Lexample/SecZero;" {
		t.Fatalf("section 0 = %+v", files[0])
	}
	if files[1].Entry != "classes.dex#1" || files[1].Header.HeaderOffset != files[0].Header.FileSize || files[1].Classes[0].Descriptor != "Lexample/SecOne;" {
		t.Fatalf("section 1 header=%+v classes=%+v", files[1].Header, files[1].Classes)
	}
}

func TestParseDEX041RejectsMalformedContainers(t *testing.T) {
	single := syntheticDEX041Single("Lexample/Single041;")
	container := syntheticDEX041Container("Lexample/SecZero;", "Lexample/SecOne;")

	tests := []struct {
		name   string
		base   []byte
		mutate func([]byte)
		want   string
	}{
		{
			name: "wrong header_size 0x70",
			base: single,
			mutate: func(b []byte) {
				put041U32(b, 36, 0x70)
			},
			want: "header_size=112, want 120",
		},
		{
			name: "container_size mismatch",
			base: single,
			mutate: func(b []byte) {
				put041U32(b, 112, uint32(len(b)+4))
			},
			want: "container_size=",
		},
		{
			name: "header_offset mismatch",
			base: single,
			mutate: func(b []byte) {
				put041U32(b, 116, 4)
			},
			want: "header_offset=4, want 0",
		},
		{
			name: "truncated file_size exceeds remaining",
			base: single,
			mutate: func(b []byte) {
				put041U32(b, 32, uint32(len(b)+8))
			},
			want: "exceeds remaining container",
		},
		{
			name: "bad section 1 magic",
			base: container,
			mutate: func(b []byte) {
				sec1 := int(binary.LittleEndian.Uint32(b[32:36]))
				copy(b[sec1+4:sec1+7], "040")
			},
			want: `unexpected version "040"`,
		},
		{
			name: "bad section 1 checksum",
			base: container,
			mutate: func(b []byte) {
				sec1 := int(binary.LittleEndian.Uint32(b[32:36]))
				b[sec1+8] ^= 0xff
			},
			want: "Adler-32 checksum=",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := append([]byte(nil), tc.base...)
			tc.mutate(raw)
			if _, err := ParseContainer(raw, "classes.dex"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ParseContainer error = %v, want %q", err, tc.want)
			}
		})
	}
}
