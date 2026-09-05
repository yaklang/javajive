package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestIdentSelfCastRewritesParenForms(t *testing.T) {
	in := "throw (var1) var1;\nthrow new Foo((var2)(var2));\nreturn (Throwable) var3;\n"
	os.Unsetenv("JDEC_IDENT_SELF_CAST_OFF")
	on := fixIdentSelfCast(in)
	if strings.Contains(on, "(var1) var1") || strings.Contains(on, "(var2)(var2)") {
		t.Errorf("ON still has self-cast:\n%s", on)
	}
	if !strings.Contains(on, "throw var1;") {
		t.Errorf("ON missing throw var1:\n%s", on)
	}
	if !strings.Contains(on, "new Foo(var2)") {
		t.Errorf("ON missing new Foo(var2):\n%s", on)
	}
	if !strings.Contains(on, "(Throwable) var3") {
		t.Errorf("ON must keep real type cast, got:\n%s", on)
	}
	t.Setenv("JDEC_IDENT_SELF_CAST_OFF", "1")
	if fixIdentSelfCast(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestCallableStatementSelfCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/FailOnTimeout$CallableStatement.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_IDENT_SELF_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "(var1) var1") || strings.Contains(on, "(var1)(var1)") {
		t.Errorf("ON still self-casts:\n%s", on)
	}
	t.Setenv("JDEC_IDENT_SELF_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "(var1) var1") && !strings.Contains(off, "(var1)(var1)") {
		t.Errorf("OFF expected self-cast, got:\n%s", off)
	}
	if on == off {
		t.Fatal("ON/OFF identical")
	}
}

func TestCategoryFilterFactorySelfCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/CategoryFilterFactory.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_IDENT_SELF_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "(var2)(var2)") || strings.Contains(on, "(var2) var2") {
		t.Errorf("ON still self-casts:\n%s", on)
	}
	t.Setenv("JDEC_IDENT_SELF_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "(var2)(var2)") && !strings.Contains(off, "(var2) var2") {
		t.Errorf("OFF expected self-cast, got:\n%s", off)
	}
}

func TestTestCaseDupThrowableCatchIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/TestCase.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_DUP_THROWABLE_CATCH_FINALLY_OFF")
	os.Unsetenv("JDEC_ALREADY_CAUGHT_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Count(on, "catch(Throwable var2)") > 2 {
		// runBare has an inner catch(Throwable var2) plus one outer; a third is the defect.
		t.Errorf("ON still has duplicate outer catch(Throwable var2):\n%s", on)
	}
	t.Setenv("JDEC_DUP_THROWABLE_CATCH_FINALLY_OFF", "1")
	t.Setenv("JDEC_ALREADY_CAUGHT_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if strings.Count(off, "catch(Throwable var2)") < 3 {
		t.Errorf("OFF expected duplicate catch(Throwable var2), got %d:\n%s", strings.Count(off, "catch(Throwable var2)"), off)
	}
}
