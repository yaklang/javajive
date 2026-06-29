package yserx

import (
	"encoding/hex"
	"testing"
)

func TestBuildStringRoundTrip(t *testing.T) {
	built := NewJavaString("javajive")
	raw := MarshalJavaObjects(built)

	objs, err := ParseJavaSerialized(raw)
	if err != nil {
		t.Fatalf("ParseJavaSerialized: %v", err)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}

	again := MarshalJavaObjects(objs...)
	if hex.EncodeToString(again) != hex.EncodeToString(raw) {
		t.Fatalf("round-trip mismatch:\n have %x\n want %x", again, raw)
	}
}

func TestParseInvalidStream(t *testing.T) {
	if _, err := ParseJavaSerialized([]byte{0x00, 0x01, 0x02}); err == nil {
		t.Fatal("expected error parsing non-serialized bytes")
	}
}

func TestFromJsonInvalid(t *testing.T) {
	if _, err := FromJson([]byte("not-json")); err == nil {
		t.Fatal("expected error from invalid json")
	}
}
