package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestTernaryClassLiteralDeclIsLoadBearing pins the class-literal ternary declaration-type fix
// (JDEC_NO_CLASSLIT_SLOT_TYPE). A ternary `cond ? Foo.class : classField` is a java.lang.Class value,
// but a class-literal arm's Type() reports the REFERENCED class (Foo) to drive bare `Foo.class`
// rendering, so the naive arm-merge collapses to the arms' LUB (Object for an Object.class-vs-Class
// pair). The capturing local would then be declared `Object c` and a later `c.getModifiers()` /
// `c.getName()` fails to recompile ("cannot find symbol"; spring-core cglib Enhancer.generateClass).
// The fix counts a class-literal arm as java.lang.Class in the ternary arm-merge AND prefers the slot
// ref's resolved type on the declaration, so the local is declared `Class`. With the fix ON the
// declaration is `Class`; with the kill-switch it degrades to the broken `Object`, proving the fix is
// load-bearing.
func TestTernaryClassLiteralDeclIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/TernaryClassLitSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_NO_CLASSLIT_SLOT_TYPE")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "Class var1 = ((this.superclass) == (null)) ? (Object.class) : (this.superclass);") {
		t.Errorf("fix ON: expected `Class var1 = ...` declaration, got:\n%s", on)
	}

	t.Setenv("JDEC_NO_CLASSLIT_SLOT_TYPE", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "Object var1 = ((this.superclass) == (null)) ? (Object.class) : (this.superclass);") {
		t.Errorf("fix OFF: expected the broken `Object var1 = ...` declaration (kill-switch not load-bearing), got:\n%s", off)
	}
}
