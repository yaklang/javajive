package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestIfaceDefaultSuperIsLoadBearing pins the interface-default super-call fix
// (JDEC_IFACE_DEFAULT_SUPER_OFF). A `super.m()` invokespecial whose target is a directly-implemented
// interface's DEFAULT method must be spelled `Iface.super.m()`; a bare `super.m()` resolves against the
// superclass, which does not declare it -> "cannot find symbol" (spring StandardAnnotationMetadata /
// StandardMethodMetadata / SimpleAnnotationMetadata `super.getAnnotationTypes()` family). The seed
// IfaceDefaultSuperSeed implements IfaceDefaultSuper (a default `describe()`) and calls
// `IfaceDefaultSuper.super.describe()`. With the fix ON the qualified form is emitted; with the
// kill-switch OFF it degrades to the broken bare `super.describe()`, proving the fix is load-bearing.
// The cross-class resolver is required so SiblingSuperTypes can confirm the target is a direct interface.
func TestIfaceDefaultSuperIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/IfaceDefaultSuperSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	resolver := func(internalName string) ([]byte, bool) {
		base := internalName[strings.LastIndexByte(internalName, '/')+1:]
		b, e := os.ReadFile("testdata/regression/" + base + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	os.Unsetenv("JDEC_IFACE_DEFAULT_SUPER_OFF")
	on, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "IfaceDefaultSuper.super.describe()") {
		t.Errorf("fix ON: expected `IfaceDefaultSuper.super.describe()`, got:\n%s", on)
	}

	t.Setenv("JDEC_IFACE_DEFAULT_SUPER_OFF", "1")
	off, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "IfaceDefaultSuper.super.describe()") || !strings.Contains(off, "super.describe()") {
		t.Errorf("fix OFF: expected the broken bare `super.describe()` (kill-switch not load-bearing), got:\n%s", off)
	}
}
