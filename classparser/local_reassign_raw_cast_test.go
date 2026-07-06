package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestLocalReassignRawCastIsLoadBearing pins parameterizedLocalReassignRawCast. The seed declares its
// local at the METHOD return's wildcard type `Class<? super T>`, but the decompiler re-types the slot at
// the invariant `Class<T>` (from the initial `= type` store), then reassigns `result = result.getSuperclass()`.
// getSuperclass() returns `Class<? super T>` (captured), NOT assignable to the invariant `Class<T>`, so a
// bare reassignment fails "Class<CAP#1> cannot be converted to Class<T>". The fix re-inserts the source's
// raw `(Class)` cast. With the kill-switch the cast disappears. Real hit: objenesis
// SerializationInstantiatorHelper / PercSerializationInstantiator.
func TestLocalReassignRawCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/LocalReassignRawSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the raw `(Class)` cast is present on the getSuperclass() reassignment.
	os.Unsetenv("JDEC_PARAM_LOCAL_REASSIGN_RAW_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "(Class) (") || !strings.Contains(on, ".getSuperclass()") {
		t.Errorf("fix ON: expected a raw `(Class)` cast on the getSuperclass() reassignment, got:\n%s", on)
	}

	// Fix OFF: the cast disappears (the uncompilable bare reassignment), proving it is load-bearing.
	t.Setenv("JDEC_PARAM_LOCAL_REASSIGN_RAW_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "(Class) (") {
		t.Errorf("fix OFF: expected NO raw `(Class)` cast (kill-switch load-bearing), got:\n%s", off)
	}
}
