package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestAlreadyCaughtDuplicateCatchRewritesMulticatch(t *testing.T) {
	in := "" +
		"try{\n\tfoo();\n" +
		"}catch(IllegalAccessException | NoSuchMethodException var1){\n\tthrow new RuntimeException(var1);\n" +
		"}catch(IllegalArgumentException var1){\n\tthrow new RuntimeException(var1);\n" +
		"}catch(NoSuchMethodException var1){\n\tthrow new RuntimeException(var1);\n" +
		"}catch(SecurityException var1){\n\tthrow new RuntimeException(var1);\n" +
		"}\n"
	os.Unsetenv("JDEC_ALREADY_CAUGHT_OFF")
	on := fixAlreadyCaughtDuplicateCatch(in)
	if strings.Count(on, "catch(NoSuchMethodException") != 0 {
		t.Errorf("ON expected dedicated NSME catch dropped, got:\n%s", on)
	}
	if !strings.Contains(on, "IllegalAccessException | NoSuchMethodException") {
		t.Errorf("ON must keep multicatch, got:\n%s", on)
	}
	if !strings.Contains(on, "catch(SecurityException") {
		t.Errorf("ON must keep SecurityException catch, got:\n%s", on)
	}
	t.Setenv("JDEC_ALREADY_CAUGHT_OFF", "1")
	if fixAlreadyCaughtDuplicateCatch(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestFactoryHolderAlreadyCaughtIsLoadBearing(t *testing.T) {
	// Original FactoryHolder has a dedicated NoSuchMethodException handler:
	// preserving it is correct; duplicating it or requiring a legacy OFF defect is not.
	raw, err := os.ReadFile("testdata/regression/ManagementFactory$FactoryHolder.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedHandlerMultiplicity(t, raw, "JDEC_ALREADY_CAUGHT_OFF")
}
