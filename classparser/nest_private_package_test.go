package javaclassparser

// Flattened nested units cannot see each other's private members. nestDemotePrivate
// widens fields to package-private. Private instance methods retain nonvirtual
// dispatch and use separately proven access bridges when siblings are resolved.

import (
	"os"
	"strings"
	"testing"
)

func TestNestPrivateFieldDemotedOnEnclosingClass(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/NestPrivOuter.class")
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("JDEC_NEST_PRIVATE_PACKAGE_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "private int bag") {
		t.Errorf("ON: enclosing private field was not demoted:\n%s", on)
	}
	if !strings.Contains(on, "int bag") {
		t.Errorf("ON: missing bag field:\n%s", on)
	}
	if !strings.Contains(on, "private void addClient") {
		t.Errorf("ON: private instance dispatch target was widened:\n%s", on)
	}
	t.Setenv("JDEC_NEST_PRIVATE_PACKAGE_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "private int bag") {
		t.Errorf("OFF: expected private field kept:\n%s", off)
	}
	if !strings.Contains(off, "private void addClient") {
		t.Errorf("OFF: private instance dispatch target was widened:\n%s", off)
	}
}

func TestNestPrivateInnerSeesDemotedMembers(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/NestPrivOuter$Inner.class")
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("JDEC_NEST_PRIVATE_PACKAGE_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if !strings.Contains(on, "this.this$0.bag") && !strings.Contains(on, ".bag") {
		t.Errorf("ON: inner does not read bag:\n%s", on)
	}
	t.Setenv("JDEC_NEST_PRIVATE_PACKAGE_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if on == off {
		// Inner dump itself may be identical; the load-bearing change is on the outer.
		t.Log("inner dump identical ON/OFF (expected; demote is on the enclosing class)")
	}
}
