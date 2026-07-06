package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestEnumSetOfNoCastIsLoadBearing pins jdkCalleeParamIsErasedTypeVar's EnumSet.of branch.
// `EnumSet.of(E first, E... rest)` erases its method-scope `E extends Enum<E>` formals to `java.lang.Enum`
// in the descriptor, so the naive arg-cast logic wraps the concrete enum argument as `(Enum)(...)` --
// collapsing javac's inference of E and breaking overload resolution ("no suitable method found for
// of(Enum,Color[])"). The fix suppresses that cast so javac infers E from the argument; the kill-switch
// restores it, proving it load-bearing. Real hit: spring ConcurrentReferenceHashMap$Task's constructor.
func TestEnumSetOfNoCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/EnumSetOfSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): no `(Enum)` upcast on the EnumSet.of argument.
	os.Unsetenv("JDEC_ENUMSET_OF_NOCAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "EnumSet.of(var1[0]") {
		t.Errorf("fix ON: expected a cast-free `EnumSet.of(var1[0], ...)`, got:\n%s", on)
	}

	// Fix OFF: the `(Enum)` upcast reappears (the uncompilable raw invocation), proving it load-bearing.
	t.Setenv("JDEC_ENUMSET_OF_NOCAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "EnumSet.of((Enum)(var1[0])") {
		t.Errorf("fix OFF: expected the legacy `(Enum)` upcast (kill-switch load-bearing), got:\n%s", off)
	}
}
