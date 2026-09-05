package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestCatchRethrowUnreachableReturnRewrites(t *testing.T) {
	in := "" +
		"\t\ttry{\n" +
		"\t\t\treturn var0.getClass().getMethod(var0.getName(),new Class[0]).getDeclaredAnnotations();\n" +
		"\t\t}catch(SecurityException | NoSuchMethodException var1){\n" +
		"\t\tthrow new RuntimeException(var1);\n\n" +
		"\t\t}\n" +
		"\t\treturn new Annotation[0];\n"
	os.Unsetenv("JDEC_CATCH_RETHROW_UNREACHABLE_RETURN_OFF")
	on := fixCatchRethrowUnreachableReturn(in)
	if strings.Contains(on, "throw new RuntimeException") {
		t.Errorf("ON expected empty catch, got:\n%s", on)
	}
	if !strings.Contains(on, "return new Annotation[0]") {
		t.Errorf("ON must keep fallback return, got:\n%s", on)
	}
	t.Setenv("JDEC_CATCH_RETHROW_UNREACHABLE_RETURN_OFF", "1")
	if fixCatchRethrowUnreachableReturn(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestJUnit38ClassRunnerGetAnnotationsIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/JUnit38ClassRunner.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_CATCH_RETHROW_UNREACHABLE_RETURN_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "throw new RuntimeException") && strings.Contains(on, "return new Annotation[0]") {
		t.Errorf("ON still has throw+unreachable return:\n%s", on)
	}
	if !strings.Contains(on, "return new Annotation[0]") {
		t.Errorf("ON expected fallback return, got:\n%s", on)
	}
	t.Setenv("JDEC_CATCH_RETHROW_UNREACHABLE_RETURN_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "throw new RuntimeException") {
		t.Errorf("OFF expected RuntimeException rethrow, got:\n%s", off)
	}
	if on == off {
		t.Fatal("ON/OFF identical")
	}
}
