package values

import (
	"testing"
	"unicode/utf16"
)

func TestJavaUnitsStringLiteralLenMatchesT03Table(t *testing.T) {
	eq := func(units []uint16, want string) {
		t.Helper()
		got := JavaUnitsStringLiteralLen(units)
		if got != len(want) {
			t.Fatalf("units %v: len=%d want %d (%q)", units, got, len(want), want)
		}
	}
	eq(nil, `""`)
	eq([]uint16{}, `""`)
	eq([]uint16{'A'}, `"A"`)
	eq([]uint16{'\n'}, `"\n"`)
	eq([]uint16{'\r'}, `"\r"`)
	eq([]uint16{'\t'}, `"\t"`)
	eq([]uint16{'\b'}, `"\b"`)
	eq([]uint16{'\f'}, `"\f"`)
	eq([]uint16{0}, `"\000"`)
	eq([]uint16{'"'}, `"\""`)
	eq([]uint16{'\\'}, `"\\"`)
	eq([]uint16{'\\', 'u'}, `"\\u"`)
	eq([]uint16{'\\', 'n'}, `"\\n"`)
	eq([]uint16{'\\', '\\'}, `"\\\\"`)
	eq([]uint16{0xD800}, `"\uD800"`)
	eq([]uint16{'\\', 'u', '0', '0', '0', 'a'}, "\""+`\\`+`\u0075`+`000a`+"\"")

	if JavaUnitCharLiteralLen(0) != len(`'\000'`) {
		t.Fatalf("char NUL len=%d", JavaUnitCharLiteralLen(0))
	}
	if JavaUnitCharLiteralLen(0xD800) != len(`'\uD800'`) {
		t.Fatalf("char surrogate len=%d", JavaUnitCharLiteralLen(0xD800))
	}
}

func TestJavaStringLiteralOutputBytesNotLenDataPlus2(t *testing.T) {
	units := []uint16{0, '\n', 0xD800, '"', '\\'}
	data := "ignore-me"
	n := JavaStringLiteralOutputBytes(data, units)
	if n == len(data)+2 {
		t.Fatal("used len(Data)+2 instead of Units T03 length")
	}
	want := len(`"\000\n\uD800\"\\"`)
	if n != want {
		t.Fatalf("got %d want %d", n, want)
	}
	fromData := JavaStringLiteralOutputBytes("\n", nil)
	if fromData != len(`"\n"`) {
		t.Fatalf("escaped newline: %d want %d", fromData, len(`"\n"`))
	}
	if fromData == len("\n")+2 {
		t.Fatal("newline undercharged as len+2")
	}
	enc := utf16.Encode([]rune("A"))
	if JavaStringLiteralOutputBytes(nil, enc) != 3 {
		t.Fatal("units A")
	}
}

func TestJavaUnitsStringLiteralCapIsPrinterBound(t *testing.T) {
	if JavaUnitsStringLiteralCap(0) != 2 {
		t.Fatal("empty cap")
	}
	if JavaUnitsStringLiteralCap(1) != 8 {
		t.Fatal("one unit cap")
	}
	if JavaUnitsStringLiteralCap(10) != 2+60 {
		t.Fatal("10 unit cap")
	}
}
