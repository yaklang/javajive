package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestEnumSetNoneOfClassCastIsLoadBearing pins enumSetNoneOfClassArgCast.
// EnumSet.noneOf(Class<E extends Enum<E>>) rejects Class<?>. A raw `(Class)`
// makes the call unchecked. Real hit: jackson EnumSetDeserializer.constructSet.
// Kill-switch: JDEC_ENUMSET_NONEOF_CLASS_CAST_OFF.
func TestEnumSetNoneOfClassCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/EnumSetNoneOfSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_ENUMSET_NONEOF_CLASS_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "noneOf((Class)") && !strings.Contains(on, "noneOf((Class)") {
		t.Errorf("fix ON: expected noneOf((Class)...), got:\n%s", on)
	}

	t.Setenv("JDEC_ENUMSET_NONEOF_CLASS_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "noneOf((Class)") {
		t.Errorf("fix OFF: expected no Class cast, got:\n%s", off)
	}
}
