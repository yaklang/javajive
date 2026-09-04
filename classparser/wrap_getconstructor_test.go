package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestWrapUncaughtGetConstructorIsLoadBearing pins wrapUncaughtGetConstructor on a fixture
// matching spring cglib AddDelegateTransformer: a constructor body with a bare
// `cls.getConstructor(...)` and no subsequent this()/super(). Kill-switch:
// JDEC_WRAP_GETCONSTRUCTOR_OFF. The dump pipeline applies this only when the call is not
// followed by this()/super() and is not a return.
func TestWrapUncaughtGetConstructorIsLoadBearing(t *testing.T) {
	in := "" +
		"public class AddDelegateTransformer {\n" +
		"\tpublic AddDelegateTransformer(Class var2) {\n" +
		"\t\tvar2.getConstructor(new Class[]{Object.class});\n" +
		"\t\tthis.x = var2;\n" +
		"\t}\n" +
		"}\n"

	os.Unsetenv("JDEC_WRAP_GETCONSTRUCTOR_OFF")
	on := wrapUncaughtGetConstructor(in)
	if !strings.Contains(on, "catch(NoSuchMethodException") {
		t.Errorf("fix ON: expected try/catch(NoSuchMethodException), got:\n%s", on)
	}

	t.Setenv("JDEC_WRAP_GETCONSTRUCTOR_OFF", "1")
	off := wrapUncaughtGetConstructor(in)
	if strings.Contains(off, "catch(NoSuchMethodException") {
		t.Errorf("fix OFF: expected no wrap, got:\n%s", off)
	}
}

func TestWrapUncaughtGetConstructorInstantiatorIsLoadBearing(t *testing.T) {
	in := "" +
		"package org.springframework.objenesis.instantiator.basic;\n" +
		"public class ConstructorInstantiator<T> {\n" +
		"\tprotected Constructor<T> constructor;\n" +
		"\tpublic ConstructorInstantiator(Class<T> var1) {\n" +
		"\t\tthis.constructor = var1.getDeclaredConstructor(((Class[])(null)));\n" +
		"\t}\n" +
		"}\n"
	os.Unsetenv("JDEC_WRAP_GETCONSTRUCTOR_OFF")
	on := wrapUncaughtGetConstructor(in)
	if !strings.Contains(on, "catch(NoSuchMethodException") {
		t.Errorf("fix ON: expected try/catch around getDeclaredConstructor, got:\n%s", on)
	}
	t.Setenv("JDEC_WRAP_GETCONSTRUCTOR_OFF", "1")
	off := wrapUncaughtGetConstructor(in)
	if strings.Contains(off, "catch(NoSuchMethodException") {
		t.Errorf("fix OFF: expected no wrap, got:\n%s", off)
	}
}

func TestWrapUncaughtGetConstructorGenericCtorIsLoadBearing(t *testing.T) {
	in := "" +
		"package org.springframework.objenesis.strategy;\n" +
		"public class SingleInstantiatorStrategy {\n" +
		"\tpublic <T extends ObjectInstantiator<?>> SingleInstantiatorStrategy(Class<T> var1) {\n" +
		"\t\tthis.constructor = var1.getConstructor(new Class[]{Class.class});\n" +
		"\t}\n" +
		"}\n"
	os.Unsetenv("JDEC_WRAP_GETCONSTRUCTOR_OFF")
	on := wrapUncaughtGetConstructor(in)
	if !strings.Contains(on, "catch(NoSuchMethodException") {
		t.Errorf("fix ON: expected wrap of generic-ctor getConstructor, got:\n%s", on)
	}
	t.Setenv("JDEC_WRAP_GETCONSTRUCTOR_OFF", "1")
	off := wrapUncaughtGetConstructor(in)
	if strings.Contains(off, "catch(NoSuchMethodException") {
		t.Errorf("fix OFF: expected no wrap, got:\n%s", off)
	}
}

func TestWrapUncaughtGetConstructorDecompileIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/SpringAddDelegateTransformer.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_WRAP_GETCONSTRUCTOR_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !ctorGetConstructorWrapped(on) {
		t.Errorf("fix ON: expected try/catch around constructor getConstructor, got:\n%s", on)
	}
	t.Setenv("JDEC_WRAP_GETCONSTRUCTOR_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if ctorGetConstructorWrapped(off) {
		t.Errorf("fix OFF: expected no wrap around constructor getConstructor, got:\n%s", off)
	}
}

func ctorGetConstructorWrapped(src string) bool {
	idx := strings.Index(src, "public AddDelegateTransformer(")
	if idx < 0 {
		return false
	}
	end := strings.Index(src[idx:], "public void begin_class")
	if end < 0 {
		end = len(src) - idx
	}
	ctor := src[idx : idx+end]
	return strings.Contains(ctor, "try{") && strings.Contains(ctor, "getConstructor") &&
		strings.Contains(ctor, "catch(NoSuchMethodException")
}

func TestWrapUncaughtGetConstructorSkipsThisCall(t *testing.T) {
	in := "" +
		"\tpublic Foo(Class var1) {\n" +
		"\t\tvar1.getConstructor(new Class[]{Object.class});\n" +
		"\t\tthis(var1, true);\n" +
		"\t}\n"
	os.Unsetenv("JDEC_WRAP_GETCONSTRUCTOR_OFF")
	got := wrapUncaughtGetConstructor(in)
	if strings.Contains(got, "try{") {
		t.Errorf("must not wrap getConstructor before this(...):\n%s", got)
	}
}

func TestWrapUncaughtGetConstructorSkipsReturn(t *testing.T) {
	in := "\t\treturn var1.getConstructor(new Class[]{Object.class});\n"
	os.Unsetenv("JDEC_WRAP_GETCONSTRUCTOR_OFF")
	got := wrapUncaughtGetConstructor(in)
	if strings.Contains(got, "try{") {
		t.Errorf("must not wrap return getConstructor:\n%s", got)
	}
}
