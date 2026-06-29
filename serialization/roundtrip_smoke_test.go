package yserx

import (
	"encoding/hex"
	"testing"
)

// helloStringStream is the Java serialization wire form of writeObject("hello"):
// magic ac ed, version 00 05, TC_STRING 74, length 00 05, "hello".
const helloStringStream = "aced000574000568656c6c6f"

func TestParseHexAndMarshalRoundTrip(t *testing.T) {
	objs, err := ParseHexJavaSerialized(helloStringStream)
	if err != nil {
		t.Fatalf("ParseHexJavaSerialized failed: %v", err)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}

	marshaled := MarshalJavaObjects(objs...)
	if got := hex.EncodeToString(marshaled); got != helloStringStream {
		t.Fatalf("round-trip mismatch:\n have %s\n want %s", got, helloStringStream)
	}
}

func TestToJsonFromJsonRoundTrip(t *testing.T) {
	raw, err := hex.DecodeString(helloStringStream)
	if err != nil {
		t.Fatalf("decode hex: %v", err)
	}
	objs, err := ParseJavaSerialized(raw)
	if err != nil {
		t.Fatalf("ParseJavaSerialized failed: %v", err)
	}

	jsonBytes, err := ToJson(objs)
	if err != nil {
		t.Fatalf("ToJson failed: %v", err)
	}

	restored, err := FromJson(jsonBytes)
	if err != nil {
		t.Fatalf("FromJson failed: %v", err)
	}

	if got := hex.EncodeToString(MarshalJavaObjects(restored...)); got != helloStringStream {
		t.Fatalf("json round-trip mismatch:\n have %s\n want %s", got, helloStringStream)
	}
}
