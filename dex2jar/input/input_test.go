package input

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/adler32"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/yaklang/javajive/dex2jar/model"
	"github.com/yaklang/javajive/dex2jar/report"
	"github.com/yaklang/javajive/internal/mutf8"
)

func TestParseDEXFoundation(t *testing.T) {
	raw := buildDEX(t, "Lexample/Main;")
	file, err := ParseDEX(raw, "classes.dex")
	if err != nil {
		t.Fatalf("ParseDEX: %v", err)
	}
	if file.Entry != "classes.dex" || file.Header.Version != "035" {
		t.Fatalf("unexpected source/header: entry=%q version=%q", file.Entry, file.Header.Version)
	}
	if len(file.Strings) != 10 || len(file.Types) != 6 || len(file.Protos) != 1 || len(file.Fields) != 1 || len(file.Methods) != 1 || len(file.Classes) != 1 {
		t.Fatalf("table sizes: strings=%d types=%d protos=%d fields=%d methods=%d classes=%d", len(file.Strings), len(file.Types), len(file.Protos), len(file.Fields), len(file.Methods), len(file.Classes))
	}
	proto := file.Protos[0]
	if proto.Shorty != "VI" || proto.ReturnType != "V" || len(proto.Parameters) != 1 || proto.Parameters[0] != "I" {
		t.Fatalf("proto = %+v", proto)
	}
	if file.Fields[0].Class != "Lexample/Main;" || file.Fields[0].Type != "I" || file.Fields[0].Name != "field" {
		t.Fatalf("field = %+v", file.Fields[0])
	}
	if file.Methods[0].Class != "Lexample/Main;" || file.Methods[0].Name != "method" {
		t.Fatalf("method = %+v", file.Methods[0])
	}
	class := file.Classes[0]
	if class.Descriptor != "Lexample/Main;" || class.Superclass != "Ljava/lang/Object;" || class.SourceFile != "Source.java" {
		t.Fatalf("class identity = %+v", class)
	}
	if len(class.Interfaces) != 1 || class.Interfaces[0] != "Lx/Iface;" {
		t.Fatalf("interfaces = %v", class.Interfaces)
	}
	if len(class.StaticFields) != 1 || class.StaticFields[0].Field.Name != "field" {
		t.Fatalf("static fields = %+v", class.StaticFields)
	}
	if len(class.StaticValues) != 1 || class.StaticValues[0].Type != model.ValueInt || class.StaticValues[0].Signed != 42 {
		t.Fatalf("static values = %+v", class.StaticValues)
	}
	if len(class.DirectMethods) != 1 || class.DirectMethods[0].Code == nil {
		t.Fatalf("direct methods = %+v", class.DirectMethods)
	}
	code := class.DirectMethods[0].Code
	if code.RegistersSize != 1 || code.InsSize != 1 || len(code.Instructions) != 1 || code.Instructions[0] != 0x000e {
		t.Fatalf("code = %+v", code)
	}
	if len(code.Tries) != 1 || code.Tries[0].StartAddress != 0 || code.Tries[0].InstructionCount != 1 {
		t.Fatalf("tries = %+v", code.Tries)
	}
	handler := code.Tries[0].Handler
	if len(handler.Handlers) != 1 || handler.Handlers[0].Type != "Ljava/lang/Exception;" || handler.Handlers[0].Address != 0 || handler.CatchAllAddress == nil || *handler.CatchAllAddress != 0 {
		t.Fatalf("handler = %+v", handler)
	}
}

func TestParseDEXDecodesDebugInfo(t *testing.T) {
	fixture := buildDEXFixture(t, "Ldebug/Feature;")
	file, err := ParseDEX(fixture.data, "debug.dex")
	if err != nil {
		t.Fatalf("ParseDEX: %v", err)
	}
	if len(file.Classes) != 1 || len(file.Classes[0].DirectMethods) != 1 {
		t.Fatalf("classes = %+v", file.Classes)
	}
	code := file.Classes[0].DirectMethods[0].Code
	if code == nil || code.Debug == nil {
		t.Fatalf("debug info not decoded: %+v", code)
	}
	if code.Debug.LineStart != 1 {
		t.Fatalf("line_start = %d", code.Debug.LineStart)
	}
	if len(code.Debug.ParameterNames) != 0 {
		t.Fatalf("parameter names = %v", code.Debug.ParameterNames)
	}
	if len(code.Debug.Positions) != 1 || code.Debug.Positions[0].Address != 0 || code.Debug.Positions[0].Line != 5 {
		t.Fatalf("positions = %+v", code.Debug.Positions)
	}
}

// Debug metadata is auxiliary: a broken stream must degrade to "no debug
// information" instead of rejecting the class.
func TestParseDEXToleratesMalformedDebugInfo(t *testing.T) {
	fixture := buildDEXFixture(t, "Ldebug/Broken;")
	fixture.data[len(fixture.data)-1] = 0x05 // DBG_END_LOCAL without its operand
	refreshDEXIntegrity(fixture.data)
	file, err := ParseDEX(fixture.data, "broken.dex")
	if err != nil {
		t.Fatalf("ParseDEX rejected a class for broken debug metadata: %v", err)
	}
	code := file.Classes[0].DirectMethods[0].Code
	if code == nil {
		t.Fatal("code item was dropped")
	}
	// Any surviving debug data must still be structurally valid: the truncation
	// itself may only cut the tail short.
	if code.Debug != nil {
		for _, position := range code.Debug.Positions {
			if position.Address > uint32(len(code.Instructions)) || position.Line == 0 {
				t.Fatalf("truncated debug stream kept an invalid position: %#v", position)
			}
		}
	}
}

func TestParseDEXRejectsUnsupportedFirstSliceData(t *testing.T) {
	const descriptor = "Lunsupported/Feature;"
	base := buildDEXFixture(t, descriptor)
	tests := []struct {
		name    string
		mutate  func([]byte)
		offset  int64
		class   string
		method  string
		op      string
		message string
	}{
		{
			// Annotations are decoded now; an offset pointing into unrelated
			// bytes must still be rejected at the annotation directory.
			name: "annotations outside the directory",
			mutate: func(data []byte) {
				putU32(data, base.classDefsOff+20, 1)
			},
			offset: 1,
			class:  descriptor,
			op:     "annotations_directory",
		},
		{
			name: "version 036",
			mutate: func(data []byte) {
				copy(data[4:7], "036")
			},
			offset:  4,
			op:      "header",
			message: `unsupported DEX version "036"; supported versions are 035, 037, 038, and 039`,
		},
		{
			name: "version 039 without map evidence",
			mutate: func(data []byte) {
				copy(data[4:7], "039")
			},
			offset:  52,
			op:      "header",
			message: `DEX version "039" requires map_list evidence for feature admission`,
		},
		{
			name: "version before 035",
			mutate: func(data []byte) {
				copy(data[4:7], "034")
			},
			offset:  4,
			op:      "header",
			message: `unsupported DEX version "034"; supported versions are 035, 037, 038, and 039`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := append([]byte(nil), base.data...)
			test.mutate(raw)
			diagnostic := requireDiagnosticWithoutPanic(t, raw, "unsupported.dex")
			if diagnostic.Entry != "unsupported.dex" || diagnostic.Class != test.class || diagnostic.Method != test.method || diagnostic.Offset != test.offset || diagnostic.Op != test.op {
				t.Fatalf("diagnostic context = %#v", diagnostic)
			}
			if diagnostic.Err == nil {
				t.Fatal("diagnostic has no error")
			}
			if test.message != "" && diagnostic.Err.Error() != test.message {
				t.Fatalf("diagnostic error = %v, want %q", diagnostic.Err, test.message)
			}
		})
	}
}

func TestParseDEXSurrogateValidation(t *testing.T) {
	t.Run("paired", func(t *testing.T) {
		const descriptor = "Lsurrogate/\U0001f600;"
		fixture := buildDEXFixtureUnits(t, utf16.Encode([]rune(descriptor)))
		file, err := ParseDEX(fixture.data, "paired.dex")
		if err != nil {
			t.Fatalf("ParseDEX: %v", err)
		}
		if file.Strings[0] != descriptor || file.Classes[0].Descriptor != descriptor {
			t.Fatalf("surrogate pair decoded as string=%q class=%q", file.Strings[0], file.Classes[0].Descriptor)
		}
	})

	prefix := utf16.Encode([]rune("Lbad/"))
	for _, test := range []struct {
		name      string
		surrogate uint16
		kind      string
	}{
		{name: "isolated high", surrogate: 0xd800, kind: "high"},
		{name: "isolated low", surrogate: 0xdc00, kind: "low"},
	} {
		t.Run(test.name, func(t *testing.T) {
			units := append(append([]uint16(nil), prefix...), test.surrogate, ';')
			fixture := buildDEXFixtureUnits(t, units)
			diagnostic := requireDiagnosticWithoutPanic(t, fixture.data, "surrogate.dex")
			wantOffset := int64(fixture.descriptorPayloadOff + len(prefix))
			if diagnostic.Entry != "surrogate.dex" || diagnostic.Class != "" || diagnostic.Method != "" || diagnostic.Offset != wantOffset || diagnostic.Op != "string_data" {
				t.Fatalf("diagnostic context = %#v", diagnostic)
			}
			want := fmt.Sprintf("unpaired %s surrogate at UTF-16 unit %d", test.kind, len(prefix))
			if diagnostic.Err == nil || diagnostic.Err.Error() != want {
				t.Fatalf("diagnostic error = %v, want %q", diagnostic.Err, want)
			}
		})
	}
}

func TestParseDEXMalformedNeverPanics(t *testing.T) {
	raw := buildDEX(t, "Lbad/Target;")
	for size := 0; size < len(raw); size++ {
		t.Run(fmt.Sprintf("truncate_%d", size), func(t *testing.T) {
			assertErrorWithoutPanic(t, raw[:size])
		})
	}

	cases := map[string][]byte{}
	badMagic := append([]byte(nil), raw...)
	badMagic[0] = 'x'
	cases["magic"] = badMagic
	badEndian := append([]byte(nil), raw...)
	binary.LittleEndian.PutUint32(badEndian[40:44], 0x78563412)
	cases["endian"] = badEndian
	badStrings := append([]byte(nil), raw...)
	binary.LittleEndian.PutUint32(badStrings[60:64], uint32(len(raw)-2))
	cases["string_table_range"] = badStrings
	badStringData := append([]byte(nil), raw...)
	stringIDsOff := binary.LittleEndian.Uint32(badStringData[60:64])
	binary.LittleEndian.PutUint32(badStringData[stringIDsOff:stringIDsOff+4], uint32(len(raw)+1))
	cases["string_data_offset"] = badStringData
	for name, data := range cases {
		t.Run(name, func(t *testing.T) { assertErrorWithoutPanic(t, data) })
	}
}

func TestParseAPKDiscoversNumericMultidexOrder(t *testing.T) {
	apk := buildAPK(t, []zipEntry{
		{name: "classes10.dex", data: buildDEX(t, "Lexample/Ten;")},
		{name: "assets/ignored.dex", data: []byte("not dex")},
		{name: "classes2.dex", data: buildDEX(t, "Lexample/Two;")},
		{name: "classes.dex", data: buildDEX(t, "Lexample/One;")},
	})
	app, err := ParseAPK(apk)
	if err != nil {
		t.Fatalf("ParseAPK: %v", err)
	}
	want := []string{"classes.dex", "classes2.dex", "classes10.dex"}
	if len(app.Files) != len(want) {
		t.Fatalf("files=%d want %d", len(app.Files), len(want))
	}
	for i := range want {
		if app.Files[i].Entry != want[i] {
			t.Fatalf("files[%d].Entry=%q want %q", i, app.Files[i].Entry, want[i])
		}
	}
}

func TestParseAPKRejectsDuplicateClassDescriptor(t *testing.T) {
	apk := buildAPK(t, []zipEntry{
		{name: "classes.dex", data: buildDEX(t, "Lduplicate/Class;")},
		{name: "classes2.dex", data: buildDEX(t, "Lduplicate/Class;")},
	})
	_, err := ParseAPK(apk)
	if err == nil {
		t.Fatal("ParseAPK accepted duplicate class descriptor")
	}
	diagnostic, ok := report.AsDiagnostic(err)
	if !ok || diagnostic.Entry != "classes2.dex" || diagnostic.Class != "Lduplicate/Class;" {
		t.Fatalf("diagnostic = %#v, %v", diagnostic, err)
	}
}

func TestParseAPKRejectsMalformedAndOversizedEntries(t *testing.T) {
	t.Run("noncanonical index", func(t *testing.T) {
		apk := buildAPK(t, []zipEntry{{name: "classes01.dex", data: buildDEX(t, "Lbad/Name;")}})
		if _, err := ParseAPK(apk); err == nil {
			t.Fatal("ParseAPK accepted classes01.dex")
		}
	})
	t.Run("size limit", func(t *testing.T) {
		raw := buildDEX(t, "Llarge/Class;")
		apk := buildAPK(t, []zipEntry{{name: "classes.dex", data: raw}})
		if _, err := ParseAPKWithOptions(apk, Options{MaxDEXSize: int64(len(raw) - 1)}); err == nil {
			t.Fatal("ParseAPK accepted oversized DEX")
		}
	})
	t.Run("APK size limit", func(t *testing.T) {
		raw := buildDEX(t, "Llarge/Archive;")
		apk := buildAPK(t, []zipEntry{{name: "classes.dex", data: raw}})
		if _, err := ParseAPKWithOptions(apk, Options{MaxAPKSize: int64(len(apk) - 1)}); err == nil {
			t.Fatal("ParseAPK accepted oversized APK")
		}
	})
	t.Run("DEX count limit", func(t *testing.T) {
		apk := buildAPK(t, []zipEntry{
			{name: "classes.dex", data: buildDEX(t, "Lcount/One;")},
			{name: "classes2.dex", data: buildDEX(t, "Lcount/Two;")},
		})
		if _, err := ParseAPKWithOptions(apk, Options{MaxDEXFiles: 1}); err == nil {
			t.Fatal("ParseAPK accepted too many DEX files")
		}
	})
	t.Run("total DEX size limit", func(t *testing.T) {
		first := buildDEX(t, "Ltotal/One;")
		second := buildDEX(t, "Ltotal/Two;")
		apk := buildAPK(t, []zipEntry{
			{name: "classes.dex", data: first},
			{name: "classes2.dex", data: second},
		})
		if _, err := ParseAPKWithOptions(apk, Options{MaxTotalDEXSize: int64(len(first) + len(second) - 1)}); err == nil {
			t.Fatal("ParseAPK accepted excessive total DEX data")
		}
	})
	t.Run("no dex", func(t *testing.T) {
		apk := buildAPK(t, []zipEntry{{name: "asset.txt", data: []byte("x")}})
		if _, err := ParseAPK(apk); err == nil {
			t.Fatal("ParseAPK accepted archive without DEX")
		}
	})
}

func assertErrorWithoutPanic(t *testing.T, raw []byte) {
	t.Helper()
	diagnostic := requireDiagnosticWithoutPanic(t, raw, "broken.dex")
	if diagnostic.Offset < 0 {
		t.Fatalf("error lacks source offset: %v", diagnostic)
	}
}

func requireDiagnosticWithoutPanic(t *testing.T, raw []byte, entry string) *report.Diagnostic {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("ParseDEX panicked: %v", recovered)
		}
	}()
	file, err := ParseDEX(raw, entry)
	if err == nil {
		t.Fatalf("ParseDEX succeeded: %+v", file)
	}
	var diagnostic *report.Diagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Entry != entry {
		t.Fatalf("error lacks source diagnostic: %v", err)
	}
	return diagnostic
}

type zipEntry struct {
	name string
	data []byte
}

func buildAPK(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for _, entry := range entries {
		file, err := writer.Create(entry.name)
		if err != nil {
			t.Fatalf("Create(%q): %v", entry.name, err)
		}
		if _, err := file.Write(entry.data); err != nil {
			t.Fatalf("Write(%q): %v", entry.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close APK: %v", err)
	}
	return out.Bytes()
}

type dexFixture struct {
	data                 []byte
	classDefsOff         int
	codeOff              int
	descriptorPayloadOff int
}

func buildDEX(t *testing.T, descriptor string) []byte {
	t.Helper()
	return buildDEXFixture(t, descriptor).data
}

func buildDEXFixture(t *testing.T, descriptor string) dexFixture {
	t.Helper()
	return buildDEXFixtureUnits(t, utf16.Encode([]rune(descriptor)))
}

func buildDEXFixtureUnits(t *testing.T, descriptor []uint16) dexFixture {
	t.Helper()
	stringUnits := [][]uint16{
		descriptor,
		utf16.Encode([]rune("Ljava/lang/Object;")),
		utf16.Encode([]rune("I")),
		utf16.Encode([]rune("V")),
		utf16.Encode([]rune("field")),
		utf16.Encode([]rune("method")),
		utf16.Encode([]rune("VI")),
		utf16.Encode([]rune("Source.java")),
		utf16.Encode([]rune("Ljava/lang/Exception;")),
		utf16.Encode([]rune("Lx/Iface;")),
	}
	const (
		headerSize  = 0x70
		typeCount   = 6
		protoCount  = 1
		fieldCount  = 1
		methodCount = 1
		classCount  = 1
	)
	stringIDsOff := headerSize
	typeIDsOff := stringIDsOff + len(stringUnits)*4
	protoIDsOff := typeIDsOff + typeCount*4
	fieldIDsOff := protoIDsOff + protoCount*12
	methodIDsOff := fieldIDsOff + fieldCount*8
	classDefsOff := methodIDsOff + methodCount*8
	dataOff := classDefsOff + classCount*32

	data := make([]byte, dataOff)
	stringOffsets := make([]uint32, len(stringUnits))
	descriptorPayloadOff := 0
	for i, units := range stringUnits {
		stringOffsets[i] = uint32(len(data))
		data = appendULEB(data, uint32(len(units)))
		if i == 0 {
			descriptorPayloadOff = len(data)
		}
		data = append(data, mutf8.Encode(units)...)
		data = append(data, 0)
	}
	data = align(data, 4)
	protoParametersOff := uint32(len(data))
	data = appendU32(data, 1)
	data = appendU16(data, 2)
	data = appendU16(data, 0)
	interfacesOff := uint32(len(data))
	data = appendU32(data, 1)
	data = appendU16(data, 5)
	data = appendU16(data, 0)
	data = align(data, 4)
	codeOff := uint32(len(data))
	data = appendU16(data, 1) // registers_size
	data = appendU16(data, 1) // ins_size
	data = appendU16(data, 0) // outs_size
	data = appendU16(data, 1) // tries_size
	data = appendU32(data, 0) // debug_info_off
	data = appendU32(data, 1) // insns_size
	data = appendU16(data, 0x000e)
	data = appendU16(data, 0) // alignment padding
	data = appendU32(data, 0) // try start_addr
	data = appendU16(data, 1) // insn_count
	data = appendU16(data, 1) // handler_off, after handler-list size
	data = append(data, 1)    // encoded_catch_handler_list size
	data = append(data, 0x7f) // handler size -1: one typed plus catch-all
	data = append(data, 4)    // exception type index
	data = append(data, 0)    // typed handler address
	data = append(data, 0)    // catch-all address
	classDataOff := uint32(len(data))
	data = append(data, 1, 0, 1, 0) // class_data counts
	data = append(data, 0, 8)       // static field delta and flags
	data = append(data, 0, 9)       // direct method delta and flags
	data = appendULEB(data, codeOff)
	staticValuesOff := uint32(len(data))
	data = append(data, 1, byte(model.ValueInt), 42)
	debugOff := uint32(len(data))
	data = appendULEB(data, 1)      // line_start
	data = appendULEB(data, 0)      // parameters_size
	data = append(data, 0x02, 0x04) // DBG_ADVANCE_LINE +4 -> line 5
	data = append(data, 0x0e)       // special: address 0, line unchanged -> position (0,5)
	data = append(data, 0x00)       // DBG_END_SEQUENCE
	putU32(data, int(codeOff)+8, debugOff)

	copy(data[:8], []byte("dex\n035\x00"))
	putU32(data, 32, uint32(len(data)))
	putU32(data, 36, headerSize)
	putU32(data, 40, 0x12345678)
	putU32(data, 56, uint32(len(stringUnits)))
	putU32(data, 60, uint32(stringIDsOff))
	putU32(data, 64, typeCount)
	putU32(data, 68, uint32(typeIDsOff))
	putU32(data, 72, protoCount)
	putU32(data, 76, uint32(protoIDsOff))
	putU32(data, 80, fieldCount)
	putU32(data, 84, uint32(fieldIDsOff))
	putU32(data, 88, methodCount)
	putU32(data, 92, uint32(methodIDsOff))
	putU32(data, 96, classCount)
	putU32(data, 100, uint32(classDefsOff))
	putU32(data, 104, uint32(len(data)-dataOff))
	putU32(data, 108, uint32(dataOff))
	for i, off := range stringOffsets {
		putU32(data, stringIDsOff+i*4, off)
	}
	typeStringIndices := []uint32{0, 1, 2, 3, 8, 9}
	for i, index := range typeStringIndices {
		putU32(data, typeIDsOff+i*4, index)
	}
	putU32(data, protoIDsOff, 6)
	putU32(data, protoIDsOff+4, 3)
	putU32(data, protoIDsOff+8, protoParametersOff)
	binary.LittleEndian.PutUint16(data[fieldIDsOff:fieldIDsOff+2], 0)
	binary.LittleEndian.PutUint16(data[fieldIDsOff+2:fieldIDsOff+4], 2)
	putU32(data, fieldIDsOff+4, 4)
	binary.LittleEndian.PutUint16(data[methodIDsOff:methodIDsOff+2], 0)
	binary.LittleEndian.PutUint16(data[methodIDsOff+2:methodIDsOff+4], 0)
	putU32(data, methodIDsOff+4, 5)
	putU32(data, classDefsOff, 0)
	putU32(data, classDefsOff+4, 1)
	putU32(data, classDefsOff+8, 1)
	putU32(data, classDefsOff+12, interfacesOff)
	putU32(data, classDefsOff+16, 7)
	putU32(data, classDefsOff+20, 0)
	putU32(data, classDefsOff+24, classDataOff)
	putU32(data, classDefsOff+28, staticValuesOff)
	refreshDEXIntegrity(data)
	return dexFixture{
		data:                 data,
		classDefsOff:         classDefsOff,
		codeOff:              int(codeOff),
		descriptorPayloadOff: descriptorPayloadOff,
	}
}

func refreshDEXIntegrity(data []byte) {
	signature := sha1.Sum(data[32:])
	copy(data[12:32], signature[:])
	putU32(data, 8, adler32.Checksum(data[12:]))
}

func align(data []byte, alignment int) []byte {
	for len(data)%alignment != 0 {
		data = append(data, 0)
	}
	return data
}

func appendU16(data []byte, value uint16) []byte {
	var raw [2]byte
	binary.LittleEndian.PutUint16(raw[:], value)
	return append(data, raw[:]...)
}

func appendU32(data []byte, value uint32) []byte {
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], value)
	return append(data, raw[:]...)
}

func appendULEB(data []byte, value uint32) []byte {
	for {
		b := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			b |= 0x80
		}
		data = append(data, b)
		if value == 0 {
			return data
		}
	}
}

func putU32(data []byte, offset int, value uint32) {
	binary.LittleEndian.PutUint32(data[offset:offset+4], value)
}

// The external fixture is a real-world DEX carrying annotations and debug info;
// it must parse into a complete class model rather than stopping at a gate.
func TestParseDEXDecodesExternalAnnotationsAndDebug(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "external", "arsylk-dexfile-tiny.dex"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	file, err := ParseDEX(raw, "classes.dex")
	if err != nil {
		t.Fatalf("ParseDEX: %v", err)
	}
	if len(file.Classes) != 1 {
		t.Fatalf("classes = %d", len(file.Classes))
	}
	class := file.Classes[0]
	if class.SourceFile != "Hello.java" {
		t.Fatalf("SourceFile = %q", class.SourceFile)
	}
	if len(class.DirectMethods) == 0 || class.DirectMethods[0].Code == nil {
		t.Fatalf("methods = %#v", class.DirectMethods)
	}
	if debug := class.DirectMethods[0].Code.Debug; debug == nil || len(debug.Positions) == 0 {
		t.Fatalf("debug info = %#v", debug)
	}
}

func TestParseDEXRejectsOversizedInput(t *testing.T) {
	_, err := ParseDEXWithOptions(make([]byte, 64), "large.dex", Options{MaxDEXSize: 8})
	if err == nil {
		t.Fatal("ParseDEXWithOptions accepted oversized input")
	}
	var diagnostic *report.Diagnostic
	if !errors.As(err, &diagnostic) || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("diagnostic = %#v, err = %v", diagnostic, err)
	}
}
