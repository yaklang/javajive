package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestEnumValueOfClassCastIsLoadBearing pins enumValueOfClassArgCast.
// Enum.valueOf(Class<T extends Enum<T>>, String) rejects Class<?>; the source's
// unchecked `(Class)` cast is a no-op checkcast dropped by javac. Kill-switch:
// JDEC_ENUM_VALUEOF_CLASS_CAST_OFF. Real hit: spring MergedAnnotationReadingVisitor.
func TestEnumValueOfClassCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/EnumValueOfClassCastSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_ENUM_VALUEOF_CLASS_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "Enum.valueOf") {
		t.Fatalf("expected Enum.valueOf in decompile, got:\n%s", on)
	}
	if !strings.Contains(on, "(Class)(") && !strings.Contains(on, "(Class) (") {
		t.Errorf("fix ON: expected raw (Class) cast on valueOf first arg, got:\n%s", on)
	}

	t.Setenv("JDEC_ENUM_VALUEOF_CLASS_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "Enum.valueOf((Class)") || strings.Contains(off, "Enum.valueOf((Class) (") {
		t.Errorf("fix OFF: expected no raw (Class) cast, got:\n%s", off)
	}
}

func TestEnumValueOfClassCastSpringFixtureIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/SpringMergedAnnotationReadingVisitor.class")
	if err != nil {
		t.Skipf("spring fixture missing: %v", err)
	}
	os.Unsetenv("JDEC_ENUM_VALUEOF_CLASS_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "Enum.valueOf") {
		t.Fatalf("expected Enum.valueOf, got:\n%s", on)
	}
	if !strings.Contains(on, "Enum.valueOf((Class)") && !strings.Contains(on, "Enum.valueOf((Class) (") {
		t.Errorf("fix ON: expected Enum.valueOf((Class)...), got:\n%s", on)
	}
	t.Setenv("JDEC_ENUM_VALUEOF_CLASS_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "Enum.valueOf((Class)") || strings.Contains(off, "Enum.valueOf((Class) (") {
		t.Errorf("fix OFF: expected no (Class) cast, got:\n%s", off)
	}
}
