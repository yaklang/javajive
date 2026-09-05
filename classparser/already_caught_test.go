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
	data, err := os.ReadFile("testdata/regression/ManagementFactory$FactoryHolder.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_ALREADY_CAUGHT_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Count(on, "catch(NoSuchMethodException") > 0 &&
		strings.Contains(on, "NoSuchMethodException var1") &&
		strings.Contains(on, "| NoSuchMethodException") {
		// A dedicated catch(NSME) alongside a multicatch that already names NSME is the defect.
		if strings.Contains(on, "}catch(NoSuchMethodException") {
			t.Errorf("ON still has dedicated catch(NSME):\n%s", on)
		}
	}
	t.Setenv("JDEC_ALREADY_CAUGHT_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "}catch(NoSuchMethodException") {
		t.Errorf("OFF expected dedicated catch(NSME), got:\n%s", off)
	}
	if on == off {
		t.Fatal("ON/OFF identical")
	}
}
