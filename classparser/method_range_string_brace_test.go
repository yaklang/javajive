package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestApplyLineBracesSkipsStringBraces pins the string-aware brace walker. A `{` inside
// `new StringBuilder("{")` must not change depth; the kill-switch restores naive counting.
func TestApplyLineBracesSkipsStringBraces(t *testing.T) {
	os.Unsetenv("JDEC_BRACE_SKIP_STRINGS_OFF")
	ln := "\t\t\tStringBuilder var2 = new StringBuilder(\"{\");"
	d, closed := applyLineBraces(ln, 2)
	if closed || d != 2 {
		t.Fatalf("fix ON: string `{` must not count, depth=%d closed=%v", d, closed)
	}
	d, closed = applyLineBraces("\t}", 1)
	if !closed || d != 0 {
		t.Fatalf("real close brace should close, depth=%d closed=%v", d, closed)
	}

	t.Setenv("JDEC_BRACE_SKIP_STRINGS_OFF", "1")
	d, closed = applyLineBraces(ln, 2)
	if closed || d != 3 {
		t.Fatalf("fix OFF: string `{` should count, depth=%d closed=%v", d, closed)
	}
}

// TestMethodRangeStringBraceIsLoadBearing pins applyLineBraces on the real decompile path.
// Spring SynthesizedMergedAnnotationInvocationHandler.toString reconstructs
// `new StringBuilder("{")`; without string-aware counting the method range swallows
// getAttributeValue and getName, so getName's String local is treated as a lambda capture
// of getAttributeValue's Method parameter (`final String var1_f1 = var1`). Kill-switch:
// JDEC_BRACE_SKIP_STRINGS_OFF.
func TestMethodRangeStringBraceIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/SpringSynthesizedMergedAnnotationInvocationHandler.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_BRACE_SKIP_STRINGS_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if methodParamCopiedAsString(on) {
		t.Errorf("fix ON: Method param must not be copied as String before getReturnType, got:\n%s", on)
	}

	t.Setenv("JDEC_BRACE_SKIP_STRINGS_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !methodParamCopiedAsString(off) {
		t.Errorf("fix OFF: expected final String copy of the Method param used with getReturnType, got:\n%s", off)
	}
}

// methodParamCopiedAsString reports the false-capture shape: a final String copy whose
// source is a Method-typed local, then a lambda reading `.getReturnType()` on that copy.
func methodParamCopiedAsString(src string) bool {
	idx := strings.Index(src, "final String ")
	if idx < 0 {
		return false
	}
	// The copy must be consumed via getReturnType (the Method API the String type cannot host).
	rest := src[idx:]
	return strings.Contains(rest, ".getReturnType()")
}
