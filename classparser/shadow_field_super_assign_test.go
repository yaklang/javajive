package javaclassparser

// Subclass field this.f = this.f is getfield Super.f; putfield This.f.
// Kill-switch: JDEC_SHADOW_FIELD_SUPER_ASSIGN_OFF.

import (
	"os"
	"strings"
	"testing"
)

func TestShadowFieldSuperAssignRewritesSelfAssign(t *testing.T) {
	in := "this.poolLock = this.poolLock;\nthis.leasedConnections = (Set) (this.leasedConnections);"
	os.Unsetenv("JDEC_SHADOW_FIELD_SUPER_ASSIGN_OFF")
	on := fixShadowFieldSuperAssign(in)
	if !strings.Contains(on, "this.poolLock = super.poolLock;") {
		t.Errorf("ON missing super.poolLock, got:\n%s", on)
	}
	if !strings.Contains(on, "this.leasedConnections = (Set) (super.leasedConnections);") {
		t.Errorf("ON missing super.leasedConnections cast form, got:\n%s", on)
	}
	t.Setenv("JDEC_SHADOW_FIELD_SUPER_ASSIGN_OFF", "1")
	if fixShadowFieldSuperAssign(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestConnPoolByRouteShadowFieldIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/ConnPoolByRoute.class")
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("JDEC_SHADOW_FIELD_SUPER_ASSIGN_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "this.poolLock = this.poolLock") {
		t.Errorf("ON still self-assigns poolLock")
	}
	if !strings.Contains(on, "this.poolLock = super.poolLock") {
		t.Errorf("ON missing super.poolLock:\n%s", on)
	}
	t.Setenv("JDEC_SHADOW_FIELD_SUPER_ASSIGN_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "this.poolLock = this.poolLock") {
		t.Errorf("OFF expected self-assign")
	}
	if on == off {
		t.Fatal("ON/OFF identical")
	}
}
