package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestClassEnumReturnRawBridgeIsLoadBearing pins wildcardObjectReturnRawBridge on
// Class<Enum<?>> vs Class<?>. Real hit: jackson EnumResolver._enumClass.
func TestClassEnumReturnRawBridgeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ClassEnumRetSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "(Class<Enum<?>>)") || !strings.Contains(on, "(Class)") {
		t.Errorf("fix ON: expected (Class<Enum<?>>)(Class) bridge, got:\n%s", on)
	}

	t.Setenv("JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "(Class<Enum<?>>)") && strings.Contains(off, "(Class)") {
		t.Errorf("fix OFF: expected no raw bridge, got:\n%s", off)
	}
}

// TestClassCmpRawCastIsLoadBearing pins incomparableClassCmpRawCast.
// Class<T> vs BigDecimal.class is incomparable. Real hit: jackson NumberSerializer.
// Kill-switch: JDEC_CLASS_CMP_RAW_CAST_OFF.
func TestClassCmpRawCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ClassCmpSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_CLASS_CMP_RAW_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "(Class)(this.handledType())") && !strings.Contains(on, "(Class) (this.handledType())") {
		t.Errorf("fix ON: expected (Class) handledType(), got:\n%s", on)
	}

	t.Setenv("JDEC_CLASS_CMP_RAW_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "(Class)(this.handledType())") || strings.Contains(off, "(Class) (this.handledType())") {
		t.Errorf("fix OFF: expected no Class cast, got:\n%s", off)
	}
}

// TestClassTypeVarFieldStoreCastIsLoadBearing pins classTypeVarFieldStoreCast.
// Class<T> field assigned getRawClass() (Class<?>). Real hit: jackson StdSerializer.
// Kill-switch: JDEC_CLASS_TV_FIELD_CAST_OFF.
func TestClassTypeVarFieldStoreCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ClassTVFieldSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_CLASS_TV_FIELD_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "(Class)") {
		t.Errorf("fix ON: expected (Class) field store cast, got:\n%s", on)
	}

	t.Setenv("JDEC_CLASS_TV_FIELD_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "this._handledType = (Class)") {
		t.Errorf("fix OFF: expected no Class field-store cast, got:\n%s", off)
	}
}
