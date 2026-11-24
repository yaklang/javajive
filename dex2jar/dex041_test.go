package dex2jar_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"hash/adler32"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar"
	"github.com/yaklang/javajive/inspect"
)

func buildDEX041Section035Upgrade(descriptor string) []byte {
	stringsTable := []string{descriptor, "Ljava/lang/Object;"}
	const (
		headerSizeV041 = 0x78
		stringIDsOff   = headerSizeV041
		typeIDsOff     = stringIDsOff + 8
		classDefsOff   = typeIDsOff + 8
		strStart       = classDefsOff + 32
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
	writeMap := func(items []struct {
		t uint16
		n uint32
		o uint32
	}) {
		putU32(data, mapOff, uint32(len(items)))
		for i, it := range items {
			p := mapOff + 4 + i*12
			data[p], data[p+1] = byte(it.t), byte(it.t>>8)
			putU32(data, p+4, it.n)
			putU32(data, p+8, it.o)
		}
	}
	writeMap([]struct {
		t uint16
		n uint32
		o uint32
	}{
		{0x0000, 1, 0},
		{0x0001, 2, stringIDsOff},
		{0x0002, 2, typeIDsOff},
		{0x0006, 1, classDefsOff},
		{0x1000, 1, uint32(mapOff)},
	})

	total := uint32(len(data))
	copy(data[:8], "dex\n041\x00")
	putU32(data, 32, total)
	putU32(data, 36, headerSizeV041)
	putU32(data, 40, 0x12345678)
	putU32(data, 52, uint32(mapOff))
	putU32(data, 56, 2)
	putU32(data, 60, stringIDsOff)
	putU32(data, 64, 2)
	putU32(data, 68, typeIDsOff)
	putU32(data, 96, 1)
	putU32(data, 100, classDefsOff)
	putU32(data, 112, total)
	putU32(data, 116, 0)
	for i, off := range strOffs {
		putU32(data, stringIDsOff+i*4, off)
		putU32(data, typeIDsOff+i*4, uint32(i))
	}
	putU32(data, classDefsOff, 0)
	putU32(data, classDefsOff+4, 1)
	putU32(data, classDefsOff+8, 1)
	putU32(data, classDefsOff+16, ^uint32(0))
	sealDEX(data)
	return data
}

func buildDEX041MultiSection(desc0, desc1 string) []byte {
	const headerSizeV041 = 0x78
	s0 := buildDEX041Section035Upgrade(desc0)
	s1 := buildDEX041Section035Upgrade(desc1)

	// Rebase every table/item offset in s1 by sec0Len = len(s0) to build a
	// two-section DEX 041 container.
	sec0Len := uint32(len(s0))
	sec1Len := uint32(len(s1))
	total := sec0Len + sec1Len

	out := make([]byte, total)
	copy(out[:sec0Len], s0)
	copy(out[sec0Len:], s1)

	putU32(out, 112, total)

	b := int(sec0Len)
	putU32(out, b+112, total)
	putU32(out, b+116, sec0Len)
	// Header offsets 52, 60, 68, 100 hold map_off, string_ids_off, type_ids_off, class_defs_off.
	for _, hdrField := range []int{52, 60, 68, 100} {
		orig := uint32(out[b+hdrField]) | uint32(out[b+hdrField+1])<<8 | uint32(out[b+hdrField+2])<<16 | uint32(out[b+hdrField+3])<<24
		putU32(out, b+hdrField, orig+sec0Len)
	}
	strOff0 := uint32(out[b+headerSizeV041]) | uint32(out[b+headerSizeV041+1])<<8 | uint32(out[b+headerSizeV041+2])<<16 | uint32(out[b+headerSizeV041+3])<<24
	strOff1 := uint32(out[b+headerSizeV041+4]) | uint32(out[b+headerSizeV041+5])<<8 | uint32(out[b+headerSizeV041+6])<<16 | uint32(out[b+headerSizeV041+7])<<24
	putU32(out, b+headerSizeV041, strOff0+sec0Len)
	putU32(out, b+headerSizeV041+4, strOff1+sec0Len)

	mapOff := int(uint32(out[b+52]) | uint32(out[b+53])<<8 | uint32(out[b+54])<<16 | uint32(out[b+55])<<24)
	count := int(uint32(out[mapOff]) | uint32(out[mapOff+1])<<8 | uint32(out[mapOff+2])<<16 | uint32(out[mapOff+3])<<24)
	for i := 0; i < count; i++ {
		offPos := mapOff + 4 + i*12 + 8
		orig := uint32(out[offPos]) | uint32(out[offPos+1])<<8 | uint32(out[offPos+2])<<16 | uint32(out[offPos+3])<<24
		putU32(out, offPos, orig+sec0Len)
	}

	// Reseal both sections over their logical byte spans.
	sig0 := sha1.Sum(out[32:sec0Len])
	copy(out[12:32], sig0[:])
	putU32(out, 8, adler32.Checksum(out[12:sec0Len]))

	sig1 := sha1.Sum(out[b+32:])
	copy(out[b+12:b+32], sig1[:])
	putU32(out, b+8, adler32.Checksum(out[b+12:]))
	return out
}

func TestDexToJarAndInspectDEX041SingleAndContainer(t *testing.T) {
	single := buildDEX041Section035Upgrade("Lexample/Single041;")
	jar, rep, err := dex2jar.DexToJar(single, dex2jar.Options{})
	if err != nil {
		t.Fatalf("DexToJar(single 041): %v", err)
	}
	if rep.DEXFiles != 1 || rep.Classes != 1 {
		t.Fatalf("single 041 report = %#v", rep)
	}
	inspectSingle := inspect.AnalyzeWithOptions(single, "single041.dex", inspect.Options{Disassembly: false})
	if inspectSingle.Status != inspect.StatusComplete || len(inspectSingle.DEXFiles) != 1 || len(inspectSingle.DEXFiles[0].Classes) != 1 {
		t.Fatalf("inspect single 041 = %#v", inspectSingle)
	}
	_ = jar

	container := buildDEX041MultiSection("Lexample/Alpha041;", "Lexample/Beta041;")
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "raw container041", data: container},
		{name: "apk with container041", data: makeAPK(t, container)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jarBytes, report, err := dex2jar.DexToJar(tc.data, dex2jar.Options{})
			if err != nil {
				t.Fatalf("DexToJar(%s): %v", tc.name, err)
			}
			if report.DEXFiles != 2 || report.Classes != 2 {
				t.Fatalf("report = %#v, want 2 DEXFiles and 2 Classes", report)
			}
			zr, err := zip.NewReader(bytes.NewReader(jarBytes), int64(len(jarBytes)))
			if err != nil {
				t.Fatal(err)
			}
			gotClasses := map[string]bool{}
			for _, f := range zr.File {
				if f.Name == "example/Alpha041.class" || f.Name == "example/Beta041.class" {
					if _, err := classparser.Parse(readZipFile(t, f)); err != nil {
						t.Fatalf("Parse(%s): %v", f.Name, err)
					}
					gotClasses[f.Name] = true
				}
			}
			if !gotClasses["example/Alpha041.class"] || !gotClasses["example/Beta041.class"] {
				t.Fatalf("missing classes in JAR: %+v", gotClasses)
			}

			insp := inspect.AnalyzeWithOptions(tc.data, tc.name, inspect.Options{Disassembly: false})
			if insp.Status != inspect.StatusComplete || len(insp.DEXFiles) != 2 {
				t.Fatalf("inspect(%s) status=%s dex=%d", tc.name, insp.Status, len(insp.DEXFiles))
			}
		})
	}
}
