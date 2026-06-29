package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestDecompileSmoke(t *testing.T) {
	data, err := os.ReadFile("testdata/invisible_anno.class")
	if err != nil {
		t.Fatalf("read testdata class: %v", err)
	}

	src, err := Decompile(data)
	if err != nil {
		t.Fatalf("Decompile failed: %v", err)
	}

	for _, want := range []string{"class InvisibleAnnoSeed", "public int run()"} {
		if !strings.Contains(src, want) {
			t.Fatalf("decompiled source missing %q; got:\n%s", want, src)
		}
	}
}

func TestParseClassStructure(t *testing.T) {
	data, err := os.ReadFile("testdata/invisible_anno.class")
	if err != nil {
		t.Fatalf("read testdata class: %v", err)
	}

	obj, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if got := obj.GetClassName(); got != "InvisibleAnnoSeed" {
		t.Fatalf("unexpected class name: %q", got)
	}
	if got := obj.GetSupperClassName(); got != "java/lang/Object" {
		t.Fatalf("unexpected super class: %q", got)
	}
	if len(obj.Methods) == 0 {
		t.Fatalf("expected at least one method")
	}
}
