package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestWildcardArgInvariantAddCastIsLoadBearing pins wildcardArgInvariantAddCast.
// Kill-switch: JDEC_WILDCARD_ARG_ADD_CAST_OFF. Real hit: guava ImmutableTable$Builder.put.
func TestWildcardArgInvariantAddCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/WildcardArgAddCastSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_WILDCARD_ARG_ADD_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, ".add((WildcardArgAddCastSeed$Cell)") &&
		!strings.Contains(on, ".add((Cell)") &&
		!strings.Contains(on, "add((WildcardArgAddCastSeed$Cell)(") &&
		!strings.Contains(on, "add((Cell)(") {
		t.Errorf("fix ON: expected raw Cell cast on add(), got:\n%s", on)
	}

	t.Setenv("JDEC_WILDCARD_ARG_ADD_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, ".add((WildcardArgAddCastSeed$Cell)") || strings.Contains(off, ".add((Cell)") ||
		strings.Contains(off, "add((Cell)(") || strings.Contains(off, "add((WildcardArgAddCastSeed$Cell)(") {
		t.Errorf("fix OFF: expected no raw Cell add cast, got:\n%s", off)
	}
}
