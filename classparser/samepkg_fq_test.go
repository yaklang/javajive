package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestSamePkgFQNameIsLoadBearing pins the same-package simple-name shadowing fix. With the fix ON the
// class's OWN-package generic type fqseed.Widget<String> is fully-qualified (because fqseed.other.Widget
// is imported and shadows the bare simple name); with the kill-switch OFF it regresses to the bare
// `Widget<String>` that javac rejects ("type Widget does not take parameters"). Real hit: fastjson2
// ObjectWriterCreatorASM (com.alibaba.fastjson2.writer.FieldWriter<T> vs internal.asm.FieldWriter).
func TestSamePkgFQNameIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/SamePkgFQSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the same-package generic type is fully-qualified.
	os.Unsetenv("JDEC_SAMEPKG_FQ_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "fqseed.Widget<String>") {
		t.Errorf("fix ON: expected fully-qualified `fqseed.Widget<String>`, got:\n%s", on)
	}
	if !strings.Contains(on, "import fqseed.other.Widget;") {
		t.Errorf("fix ON: expected the other-package Widget to keep its import, got:\n%s", on)
	}

	// Fix OFF: the ambiguous bare `Widget<String>` reappears, proving the FQ rendering is load-bearing.
	t.Setenv("JDEC_SAMEPKG_FQ_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "fqseed.Widget<String>") {
		t.Errorf("fix OFF: expected the bare (shadowed) form, but got the FQ name (kill-switch not load-bearing):\n%s", off)
	}
	if !strings.Contains(off, "Widget<String>") {
		t.Errorf("fix OFF: expected a `Widget<String>` return type, got:\n%s", off)
	}
}
