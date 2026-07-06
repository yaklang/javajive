package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestConcreteParamReturnRawCastIsLoadBearing pins concreteParamReturnSubtypeRawCast. The seed's method
// declares a CONCRETE parameterized return (`Map<String,Object>`) and returns `System.getProperties()`,
// whose static type `Properties` is a non-generic subtype of Map with a FIXED, distinct parameterization
// (`Map<Object,Object>`). A bare `return System.getProperties();` fails javac ("Properties cannot be
// converted to Map<String,Object>"), and a direct `(Map<String,Object>)` cast is inconvertible; only the
// raw `(Map)` cast the source carried compiles. The erased checkcast is a no-op the bytecode drops, so the
// decompiler must SYNTHESIZE the cast. With the kill-switch the cast disappears, proving it load-bearing.
// Real hit: spring AbstractEnvironment.getSystemProperties/getSystemEnvironment.
func TestConcreteParamReturnRawCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ConcreteParamReturnSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the raw `(Map)` cast is present on the getProperties() return.
	os.Unsetenv("JDEC_CONCRETE_PARAM_RET_SUBTYPE_RAW_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "(Map) (") || !strings.Contains(on, "getProperties()") {
		t.Errorf("fix ON: expected a raw `(Map)` cast on the getProperties() return, got:\n%s", on)
	}

	// Fix OFF: the cast disappears (the uncompilable bare return), proving it is load-bearing.
	t.Setenv("JDEC_CONCRETE_PARAM_RET_SUBTYPE_RAW_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "(Map) (") {
		t.Errorf("fix OFF: expected NO raw `(Map)` cast (kill-switch load-bearing), got:\n%s", off)
	}
}
