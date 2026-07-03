package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestBoolIntTernaryCmpCollapseIsLoadBearing pins the boolean-vs-int-ternary comparison collapse
// (JDEC_BOOL_INT_TERNARY_CMP_OFF). javac materializes a boolean sub-expression that feeds an int
// comparison as `cond ? 1 : 0`; a boolean local/param compared against it decompiles to
// `(boolVar) != ((cond) ? (1) : (0))`, which javac rejects with "incomparable types: boolean and int".
// The `? 1 : 0` ternary is exactly the int encoding of the boolean `cond`, so the comparison collapses
// to `boolVar != cond`. Seed is spring-core's real ASM MethodVisitor.class (visitMethodInsn:
// `var5 != ((var1 == 185) ? 1 : 0)`). With the fix ON the comparison is collapsed to a boolean operand;
// with the kill-switch OFF the broken int-ternary comparison reappears.
func TestBoolIntTernaryCmpCollapseIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/BoolIntTernaryCmpSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_BOOL_INT_TERNARY_CMP_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	// The collapsed form compares the boolean param against the plain boolean condition.
	if !strings.Contains(on, "(var5) != ((var1) == (185))") {
		t.Errorf("fix ON: expected collapsed `(var5) != ((var1) == (185))`, got:\n%s", on)
	}
	// The broken int-ternary comparison must be gone.
	if strings.Contains(on, "(var5) != ((var1) == (185)) ? (1) : (0)") ||
		strings.Contains(on, "(var5) != (((var1) == (185)) ? (1) : (0))") {
		t.Errorf("fix ON: broken int-ternary comparison still present:\n%s", on)
	}

	t.Setenv("JDEC_BOOL_INT_TERNARY_CMP_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "(var5) != (((var1) == (185)) ? (1) : (0))") {
		t.Errorf("fix OFF: expected the broken int-ternary comparison to reappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
