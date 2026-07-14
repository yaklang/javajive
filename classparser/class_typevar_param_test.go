package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestClassTypeVarParamCastIsLoadBearing pins the sameClassMethodParamType Class<L> parameterized
// formal recovery + resolvedParameterizedArgCast same-erasure Class<L> cast. A same-class instance
// method whose generic Signature formal is `Class<L>` (L a class-scope type variable), fed a raw
// `Class` argument (from getComponentType's erased descriptor return), needs an unchecked
// `(Class<L>)` cast that the bytecode erases. The fix recovers the parameterized formal and
// re-emits the cast via resolvedParameterizedArgCast; the kill-switch JDEC_PARAM_ARG_CAST_OFF
// drops it. Real hit: commons-lang3 EventListenerSupport.readObject.
func TestClassTypeVarParamCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ClassTypeVarParamSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_PARAM_ARG_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "(Class<L>)") {
		t.Errorf("fix ON: expected (Class<L>) cast, got:\n%s", on)
	}

	t.Setenv("JDEC_PARAM_ARG_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "(Class<L>)") {
		t.Errorf("fix OFF: expected no (Class<L>) cast, got:\n%s", off)
	}
}
