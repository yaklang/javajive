package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestThisCtorTypeVarArgCastIsLoadBearing pins thisCtorTypeVarArgCast. `ThisCtorTypeVarSeed(String)`
// delegates via `this(name, (T) new Object())` to the sibling constructor whose second formal is the BARE
// class type variable T. The bytecode erases the unchecked `(T)` cast (no checkcast is emitted for a type
// variable), so a naive decompile renders `this(name, new Object())`, which javac rejects ("Object cannot
// be converted to T"). The fix recovers the formal from the recorded constructor Signature and re-emits the
// `(T)` cast; the kill-switch drops it, proving it load-bearing. Real hit: spring PropertySource(String).
func TestThisCtorTypeVarArgCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ThisCtorTypeVarSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the `(T)` cast is present on the this(...) argument.
	os.Unsetenv("JDEC_THIS_CTOR_TYPEVAR_ARG_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "(T)(new Object())") {
		t.Errorf("fix ON: expected a `(T)` cast on the this(...) argument, got:\n%s", on)
	}

	// Fix OFF: the cast disappears (the uncompilable bare argument), proving it is load-bearing.
	t.Setenv("JDEC_THIS_CTOR_TYPEVAR_ARG_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "(T)(new Object())") {
		t.Errorf("fix OFF: expected NO `(T)` cast (kill-switch load-bearing), got:\n%s", off)
	}
}
