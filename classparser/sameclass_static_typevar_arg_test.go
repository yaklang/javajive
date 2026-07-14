package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestSameClassStaticMethodTypeVarArgCastIsLoadBearing pins sameClassStaticMethodTypeVarArgCast.
// `SameClassStaticTypeVarArgSeed.intersectionWith` reads fields typed by the bare class type variable T
// at their erased Object static type, then calls the same-class static generic method
// `between(T, T)`. The bytecode erases the source's `(T)` cast (no checkcast for a type variable), so a
// naive decompile renders a bare argument that javac feeds as Object to the generic `between`, breaking
// inference ("incompatible bounds"). The fix recovers the formal from the recorded method Signature and
// re-emits the `(T)` cast; the kill-switch drops it, proving it load-bearing. Real hit: commons-lang3
// Range.intersectionWith -> Range.<T>between(T, T, Comparator<T>).
func TestSameClassStaticMethodTypeVarArgCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/SameClassStaticTypeVarArgSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the `(T)` cast is present on the between(...) arguments.
	os.Unsetenv("JDEC_SAMECLASS_STATIC_TYPEVAR_ARG_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "between((T)(") && !strings.Contains(on, "between((T) ") {
		t.Errorf("fix ON: expected a `(T)` cast on the between(...) arguments, got:\n%s", on)
	}

	// Fix OFF: the cast disappears (the uncompilable bare argument), proving it is load-bearing.
	t.Setenv("JDEC_SAMECLASS_STATIC_TYPEVAR_ARG_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "between((T)") {
		t.Errorf("fix OFF: expected NO `(T)` cast (kill-switch load-bearing), got:\n%s", off)
	}
}
