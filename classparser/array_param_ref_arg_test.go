package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestArrayParamRefArgCastIsLoadBearing pins the array-parameter null-Object argument cast
// (arrayParamRefArgCast in renderArgAt; kill-switch JDEC_ARRAY_PARAM_REF_ARG_CAST_OFF). A null-
// initialized local typed Object that is only passed to a typed array parameter (`byte[]`) renders as a
// bare Object argument, which Java rejects ("Object cannot be converted to byte[]"). With the fix ON the
// call site carries a `(byte[])` cast (behaviour-preserving: the value already occupies the array slot in
// bytecode). With the kill-switch the bare argument returns -- the exact recompile blocker the fix removes
// (spring ASM Attribute.computeAttributesSize / putAttributes, cglib Enhancer Object->Object[]).
func TestArrayParamRefArgCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ArrayParamRefArgSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_ARRAY_PARAM_REF_ARG_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "this.sizeOf((byte[])(var1),") {
		t.Errorf("fix ON: expected `(byte[])` cast on the null-Object array argument, got:\n%s", on)
	}

	t.Setenv("JDEC_ARRAY_PARAM_REF_ARG_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "this.sizeOf(var1,") || strings.Contains(off, "this.sizeOf((byte[])(var1),") {
		t.Errorf("fix OFF: expected the bare uncast argument (kill-switch not load-bearing), got:\n%s", off)
	}
}
