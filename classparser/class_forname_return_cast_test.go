package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestClassForNameReturnCastIsLoadBearing pins classForNameReturnCast. `ClassForNameRetSeed.load` declares
// a `Class<Wrapper<T>>` return (a `Class<...>` parameterization mentioning the type variable T) and returns
// `Class.forName(name)`. The JDK signature is `Class<?> forName(String)`, so javac captures the wildcard to
// CAP#1 and rejects `Class<CAP#1>` -> `Class<Wrapper<T>>`; the source carried an unchecked `(Class<Wrapper<T>>)`
// cast the bytecode dropped. The fix re-inserts it; the kill-switch drops it, proving it load-bearing. Real
// hit: spring objenesis DelegatingToExoticInstantiator.instantiatorClass().
func TestClassForNameReturnCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ClassForNameRetSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the `(Class<...>)` cast is present on the Class.forName() return.
	os.Unsetenv("JDEC_CLASS_FORNAME_RET_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "<T>>) (Class.forName(") {
		t.Errorf("fix ON: expected a `(Class<...<T>>)` cast on the Class.forName() return, got:\n%s", on)
	}

	// Fix OFF: the cast disappears (the uncompilable bare return), proving it is load-bearing.
	t.Setenv("JDEC_CLASS_FORNAME_RET_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, ">) (Class.forName(") {
		t.Errorf("fix OFF: expected NO cast on the Class.forName() return (kill-switch load-bearing), got:\n%s", off)
	}
}
