package javaclassparser

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestStubMethodNameSanitizedAsSafeIdentifier(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/StubGenericSigSeed.class")
	if err != nil {
		t.Fatalf("read StubGenericSigSeed.class: %v", err)
	}
	obj, err := Parse(seed)
	if err != nil {
		t.Fatalf("parse StubGenericSigSeed.class: %v", err)
	}
	for _, m := range obj.Methods {
		if n, _ := obj.getUtf8(m.NameIndex); n != "<init>" {
			m.AccessFlags &^= 0x0100
		}
	}
	nonNativeSeed := obj.Bytes()
	target := []byte("\x01\x00\x07consume")
	if !bytes.Contains(nonNativeSeed, target) {
		t.Fatalf("seed class missing expected Utf8 entry %q", target)
	}
	mutated := bytes.ReplaceAll(nonNativeSeed, target, []byte("\x01\x00\x07foo-bar"))

	res, err := DecompileWithOptions(mutated, DecompileOptions{RetainUnprovedSource: true})
	if err != nil {
		t.Fatalf("DecompileWithOptions failed: %v", err)
	}
	if len(res.StubMethods) == 0 {
		t.Fatalf("expected dumpStubMethod to record stubbed method, got StubMethods=%v\n%s", res.StubMethods, res.Source)
	}
	if strings.Contains(res.Source, "foo-bar(") {
		t.Fatalf("unsanitized stub identifier `foo-bar(` leaked into Java source:\n%s", res.Source)
	}
	if !strings.Contains(res.Source, "foo_bar(") {
		t.Fatalf("expected sanitized Java identifier `foo_bar(`, got:\n%s", res.Source)
	}
}
