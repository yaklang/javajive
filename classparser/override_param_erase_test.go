package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestOverrideParamEraseIsLoadBearing pins the case-(b) override-parameter raw-erase together with the
// raw-receiver typevar-local bail. `OverrideParamEraseSeed$Sub` is a no-own-formal inner class that
// extends the own-formal `$Task<V>` (which raw-erases its execute parameters) and overrides execute.
//
// With the fix ON: Sub's override renders raw parameters (`execute(Ref, Ent)`) so it PROPERLY overrides
// the raw base (no "name clash"), keeps its `V` return, and declares the raw-receiver call result as
// `Object v` with a `(V)` return cast (no uncompilable `V v = e.getValue()`).
//
// With the kill-switch OFF: the override parameters reappear generic (`Ref<K, V>`), the exact shape that
// javac rejects as a name clash against the raw base -- proving the erase is load-bearing.
//
// Real hit: spring-core ConcurrentReferenceHashMap$1..$5 extends ConcurrentReferenceHashMap$Task.
func TestOverrideParamEraseIsLoadBearing(t *testing.T) {
	sub, err := os.ReadFile("testdata/regression/OverrideParamEraseSeed$Sub.class")
	if err != nil {
		t.Fatalf("read Sub seed: %v", err)
	}
	resolver := func(internalName string) ([]byte, bool) {
		b, e := os.ReadFile("testdata/regression/" + internalName + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	// Fix ON (default): raw override params + Object local + (V) return cast.
	os.Unsetenv("JDEC_INNER_RAW_ERASE_METHOD_PARAM_OFF")
	on, err := DecompileWithResolver(sub, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if strings.Contains(on, "execute(OverrideParamEraseSeed$Ref<K, V>") {
		t.Errorf("fix ON: override parameters must be raw-erased, got generic:\n%s", on)
	}
	if !strings.Contains(on, "protected V execute(") {
		t.Errorf("fix ON: expected the `V` return type to be preserved on the override, got:\n%s", on)
	}
	// The raw-receiver `getValue()` returns the erased bound, so the `V` result carries an explicit
	// `(V)` unchecked cast (single-use folding may inline the local straight into the return).
	if !strings.Contains(on, "(V) (var2.getValue())") && !strings.Contains(on, "(V)(var2.getValue())") {
		t.Errorf("fix ON: expected a `(V)` unchecked cast on the raw-receiver getValue() result, got:\n%s", on)
	}

	// Fix OFF: the generic override parameters reappear (the name-clash shape).
	t.Setenv("JDEC_INNER_RAW_ERASE_METHOD_PARAM_OFF", "1")
	off, err := DecompileWithResolver(sub, resolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "execute(OverrideParamEraseSeed$Ref<K, V>") {
		t.Errorf("fix OFF: expected generic `Ref<K, V>` params to reappear (kill-switch load-bearing), got:\n%s", off)
	}
}
