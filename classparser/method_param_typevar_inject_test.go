package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestMethodParamTypeVarInjectIsLoadBearing pins the method-parameter free-type-var injection in the
// enclosing-arity block (dumper.go). `MethodParamTypeVarSeed$1` is an anonymous class created inside a
// static generic method `<I,O> transform(...)`; javac emits its class Signature WITHOUT a formal `<...>`
// section, so O is recovered as free from the `Future<O>` supertype but the free `I` -- referenced only in
// the private method parameter `applyTransformation(I)` -- appears in no supertype/field and rendered
// undeclared ("cannot find symbol: class I"). The fix scans method-parameter signatures too and declares
// `I` as a formal (`<O, I>`). The kill-switch drops it, proving load-bearing. Real hit: guava Futures$2.
func TestMethodParamTypeVarInjectIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/MethodParamTypeVarSeed$1.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the anonymous class declares BOTH captured vars (`<O, I>`), so `I` is in scope.
	os.Unsetenv("JDEC_INNER_METHODPARAM_TYPEVAR_INJECT_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "<O, I> implements Future<O>") {
		t.Errorf("fix ON: expected the anonymous class to declare `<O, I>`, got:\n%s", on)
	}

	// Fix OFF: only `<O>` is declared, leaving the method parameter `I` undeclared -- the uncompilable
	// shape, proving the injection is load-bearing.
	t.Setenv("JDEC_INNER_METHODPARAM_TYPEVAR_INJECT_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "<O> implements Future<O>") {
		t.Errorf("fix OFF: expected only `<O>` declared (kill-switch load-bearing), got:\n%s", off)
	}
}
