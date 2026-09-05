package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestSuperClassTVCtorRawCastIsLoadBearing pins Class<T> super-ctor param vs Class<?> arg.
// jackson StdScalarSerializer(Class<?>, boolean) does super(var1) into StdSerializer(Class<T>).
// Kill-switch: JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF.
func TestSuperClassTVCtorRawCastIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/SuperClassTVSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	sup, err := os.ReadFile("testdata/regression/SuperSer.class")
	if err != nil {
		t.Fatalf("read super: %v", err)
	}
	resolver := func(internalName string) ([]byte, bool) {
		if internalName == "SuperSer" {
			return sup, true
		}
		return nil, false
	}

	os.Unsetenv("JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF")
	on, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "super((Class)") && !strings.Contains(on, "super((Class)") {
		t.Errorf("fix ON: expected super((Class) var1), got:\n%s", on)
	}

	t.Setenv("JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF", "1")
	off, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "super((Class)") {
		t.Errorf("fix OFF: expected no Class cast, got:\n%s", off)
	}
}
