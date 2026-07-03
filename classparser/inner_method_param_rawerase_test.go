package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestInnerMethodParamRawEraseIsLoadBearing pins the flattened-inner-class method-PARAMETER raw-erase.
// An own-formal non-static inner class (Task<T>) referencing the ENCLOSING class's type variables K/V
// in a method parameter (Ref<K,V>) loses their declaration once flattened to a top-level unit, so the
// parameter must be raw-erased (Ref<K,V> -> Ref) to avoid "cannot find symbol: class K". The return
// type is not erased (it does not participate in erasure). With the kill-switch OFF the undeclared
// K/V reappear. Real hit: spring-core ConcurrentReferenceHashMap$Task.execute(Reference<K,V>,...).
func TestInnerMethodParamRawEraseIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/MethodParamEnclosingSeed$Task.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): enclosing K/V raw-erased from the method parameter.
	os.Unsetenv("JDEC_INNER_RAW_ERASE_METHOD_PARAM_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "execute(MethodParamEnclosingSeed$Ref var0)") {
		t.Errorf("fix ON: expected raw-erased `execute(MethodParamEnclosingSeed$Ref var0)`, got:\n%s", on)
	}
	if strings.Contains(on, "execute(MethodParamEnclosingSeed$Ref<K, V> var0)") {
		t.Errorf("fix ON: expected NO undeclared `<K, V>` on the parameter, got:\n%s", on)
	}

	// Fix OFF: the undeclared enclosing K/V reappear, proving the erase is load-bearing.
	t.Setenv("JDEC_INNER_RAW_ERASE_METHOD_PARAM_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "execute(MethodParamEnclosingSeed$Ref<K, V> var0)") {
		t.Errorf("fix OFF: expected `Ref<K, V>` to reappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
