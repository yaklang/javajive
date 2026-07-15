package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestThrowTypeVarCastIsLoadBearing pins the fixThrowTypeVarCast post-processing fix. A generic
// method `<R, T extends Throwable> R typeErasure(Throwable t) throws T { throw (T) t; }` has
// its `(T)` cast erased by the bytecode (a no-op checkcast). Without the fix, the decompiler
// renders `throw t;` (where t is Throwable), which fails "unreported exception Throwable"
// because the method declares `throws T` (narrower). The fix detects `throw <var>;` inside a
// method whose `throws` clause is a single type variable and re-emits `throw (<TypeVar>) <var>;`.
// Kill-switch: JDEC_FIX_THROW_TYPEVAR_CAST_OFF. Real hit: commons-lang3 ExceptionUtils.typeErasure.
func TestThrowTypeVarCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ThrowTypeVarCastSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_FIX_THROW_TYPEVAR_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "throw (T) var0") {
		t.Errorf("fix ON: expected `throw (T) var0;` in decompiled output, got:\n%s", on)
	}

	os.Setenv("JDEC_FIX_THROW_TYPEVAR_CAST_OFF", "1")
	defer os.Unsetenv("JDEC_FIX_THROW_TYPEVAR_CAST_OFF")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "throw var0") || strings.Contains(off, "throw (T) var0") {
		t.Errorf("fix OFF: expected bare `throw var0;` (no cast) in decompiled output, got:\n%s", off)
	}
}