package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestBoolArrayElementStoreCoerceIsLoadBearing pins the boolean[] element-store int->boolean coercion
// (values.CoerceBooleanAssignRHS applied in arrayStoreRHS; kill-switch JDEC_BOOL_TO_INT_COERCE_OFF). A
// boolean value stored into a boolean[] element compiles to a materialized int diamond `cond ? 1 : 0`
// (the JVM has no boolean storage; bastore serves both byte[] and boolean[]). With the fix ON the RHS is
// retyped to boolean and folded to the connective (`out[i] = a || b || c`). With the kill-switch the raw
// int diamond `? (1) : (0)` returns — the exact "int cannot be converted to boolean" recompile blocker
// the fix removes (spring ASM ClassReader/AttributeMethods, commons-lang3 Conversion).
func TestBoolArrayElementStoreCoerceIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/BoolArrayElementStoreSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_BOOL_TO_INT_COERCE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if strings.Contains(on, "? (1) : (0)") {
		t.Errorf("fix ON: expected the int diamond folded to a boolean connective, got:\n%s", on)
	}
	if !strings.Contains(on, "var2[var3] = ((var4) == (Class.class)) || (((var4) == (Class[].class)) || (var4.isEnum()));") {
		t.Errorf("fix ON: expected `var2[var3] = ... || ... || ...` boolean store, got:\n%s", on)
	}

	t.Setenv("JDEC_BOOL_TO_INT_COERCE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "? (1) : (0)") {
		t.Errorf("fix OFF: expected the raw int diamond `? (1) : (0)` (kill-switch not load-bearing), got:\n%s", off)
	}
}
