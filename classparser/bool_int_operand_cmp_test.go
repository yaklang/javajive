package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestBoolIntOperandCmpIsLoadBearing pins boolVsIntOperandCollapse.
// A boolean field compared against an int 0/1 materialization local decompiles to
// `boolField != intVar`, which javac rejects ("incomparable types: int and boolean").
// The reconstruct is `boolField != (intVar != 0)`. Real hit: jackson
// UntypedObjectDeserializer.createContextual. Kill-switch: JDEC_BOOL_INT_OPERAND_CMP_OFF.
func TestBoolIntOperandCmpIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/UntypedObjectDeserializer.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_BOOL_INT_OPERAND_CMP_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "_nonMerging) != ((var3) != (0))") &&
		!strings.Contains(on, "(var3) != (0)) != (this._nonMerging)") {
		t.Errorf("fix ON: expected bool vs (int != 0), got snippet:\n%s", snippetNonMerging(on))
	}

	t.Setenv("JDEC_BOOL_INT_OPERAND_CMP_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "_nonMerging) != ((var3) != (0))") ||
		strings.Contains(off, "(var3) != (0)) != (this._nonMerging)") {
		t.Errorf("fix OFF: expected raw int vs boolean comparison, got:\n%s", snippetNonMerging(off))
	}
	if !strings.Contains(off, "_nonMerging") {
		t.Errorf("fix OFF: missing _nonMerging comparison:\n%s", snippetNonMerging(off))
	}
}

func snippetNonMerging(src string) string {
	var b strings.Builder
	for _, ln := range strings.Split(src, "\n") {
		if strings.Contains(ln, "_nonMerging") || strings.Contains(ln, "var3") {
			b.WriteString(ln)
			b.WriteByte('\n')
		}
	}
	s := b.String()
	if len(s) > 1200 {
		return s[:1200]
	}
	return s
}
