package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestNestedProtectedPublicIsLoadBearing pins the cross-package flattened-nested-type visibility fix.
// A `protected static` nested type (cglib's AbstractClassGenerator.Source / .ClassLoaderData) is
// reachable from subclasses in OTHER packages via inheritance; once JavaJive flattens it to a
// standalone top-level unit that inheritance relationship is gone, so leaving it package-private makes
// it unreachable cross-package ("AbstractClassGenerator$Source is not public in ...; cannot be
// accessed from outside package" -- the single biggest spring-core cglib blocker, 44 error lines). The
// fix widens it to `public` (recompile-safe). With the kill-switch OFF it regresses to package-private.
func TestNestedProtectedPublicIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/NestedProtectedSeed$Source.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the protected nested type is widened to public at the top level.
	os.Unsetenv("JDEC_NESTED_PROTECTED_PUBLIC_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "public class NestedProtectedSeed$Source") {
		t.Errorf("fix ON: expected `public class NestedProtectedSeed$Source`, got:\n%s", on)
	}

	// Fix OFF: the class regresses to package-private, proving the widening is load-bearing.
	t.Setenv("JDEC_NESTED_PROTECTED_PUBLIC_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "public class NestedProtectedSeed$Source") {
		t.Errorf("fix OFF: expected package-private (no `public`), but got public (kill-switch not load-bearing):\n%s", off)
	}
	if !strings.Contains(off, "class NestedProtectedSeed$Source") {
		t.Errorf("fix OFF: expected a `class NestedProtectedSeed$Source` declaration, got:\n%s", off)
	}
}
