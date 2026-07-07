package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestAtomicRefVParamArgCastIsLoadBearing pins the AtomicReference<V> V-parameter argument cast
// (jdkMethodParamTypeArgIndex AtomicReference branch; kill-switch JDEC_ATOMIC_REF_PARAM_OFF). A field
// `AtomicReference<T>` whose get() is read into an Object-typed local, then passed back to
// `compareAndSet(V, V)` / `getAndSet(V)` / `set(V)` (descriptor erased to Object), renders as a bare
// Object argument and javac rejects "Object cannot be converted to T" (commons-lang3
// AtomicInitializer<T> `this.reference.compareAndSet(null, var1)`). With the fix ON the call site
// re-emits the source's unchecked `(T)` cast. With the kill-switch the bare argument returns -- the
// exact recompile blocker the fix removes.
func TestAtomicRefVParamArgCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/AtomicRefVParamSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_ATOMIC_REF_PARAM_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	// The V-typed argument (var2) must carry a `(T)` cast at the compareAndSet call site.
	if !strings.Contains(on, "this.reference.compareAndSet((T)(null),(T)(var2))") {
		t.Errorf("fix ON: expected `(T)` casts on the AtomicReference V-args, got:\n%s", on)
	}

	t.Setenv("JDEC_ATOMIC_REF_PARAM_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	// With the kill-switch the bare uncast argument returns -- reproducing the recompile blocker.
	if !strings.Contains(off, "this.reference.compareAndSet(null,var2)") ||
		strings.Contains(off, "compareAndSet((T)") {
		t.Errorf("fix OFF: expected the bare uncast argument (kill-switch not load-bearing), got:\n%s", off)
	}
}
