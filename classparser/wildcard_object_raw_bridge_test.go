package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestWildcardObjectReturnRawBridgeIsLoadBearing pins wildcardObjectReturnRawBridge.
// Declared `Ser<Object>` vs helper() returning `Ser<?>` needs `(Ser<Object>)(Ser) helper()`.
// Kill-switch: JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF. Real hit: jackson SerializerProvider
// handleSecondaryContextualization → JsonSerializer<Object>.
func TestWildcardObjectReturnRawBridgeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/WildcardObjectRetSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "(Ser<Object>)") || !strings.Contains(on, "(Ser)") {
		t.Errorf("fix ON: expected raw-erasure bridge (Ser<Object>)(Ser), got:\n%s", on)
	}

	t.Setenv("JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "(Ser<Object>)") && strings.Contains(off, "(Ser) (") {
		t.Errorf("fix OFF: expected no raw-erasure bridge, got:\n%s", off)
	}
}

// TestWildcardObjectFieldStoreRawBridgeIsLoadBearing pins the field-store side of
// wildcardObjectAssignRawBridge. A same-class `Ser<Object> field` assigned from
// `this.secondary(...)` (generic return `Ser<?>`) needs `(Ser) this.secondary(...)`.
// Bytecode drops the checkcast (same erasure). Real hit: jackson UntypedObjectDeserializer
// `_mapDeserializer = ctxt.handleSecondaryContextualization(...)` and POJOPropertyBuilder
// `var7._fields = var8.withNext(...)`.
func TestWildcardObjectFieldStoreRawBridgeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/WildcardObjectFieldSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "this.field = (Ser)") && !strings.Contains(on, "this.field = (Ser<Object>)") {
		t.Errorf("fix ON: expected raw field-store cast, got:\n%s", on)
	}

	t.Setenv("JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "this.field = (Ser)") || strings.Contains(off, "this.field = (Ser<Object>)") {
		t.Errorf("fix OFF: expected no field-store raw cast, got:\n%s", off)
	}
}

// TestWildcardClassEnumParamRawCastIsLoadBearing pins Class<?> passed to Class<Enum<?>>
// (jackson EnumSerializer.construct -> EnumValues.constructFromName). A raw `(Class)` cast
// is an unchecked conversion. Kill-switch: JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF.
func TestWildcardClassEnumParamRawCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/WildcardClassEnumSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "take((Class)") && !strings.Contains(on, "take((Class<") {
		t.Errorf("fix ON: expected raw Class arg cast, got:\n%s", on)
	}

	t.Setenv("JDEC_WILDCARD_OBJECT_RAW_BRIDGE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "take((Class)") || strings.Contains(off, "take((Class<") {
		t.Errorf("fix OFF: expected no Class arg cast, got:\n%s", off)
	}
}
