package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestCommonsIoRemainingStringRewrites(t *testing.T) {
	in := "" +
		"public WildcardFileFilter(String var1) {\n\t\tString[] var2 = new String[1];\n\t\tvar2[0] = ((String)(requireWildcards(var1)));\n\t\tthis(IOCase.SENSITIVE,var2);\n\t}\n" +
		"(BiFunction<Integer, IOException>)(IOIndexedException::new)\n" +
		"return EMPTY_COMPARATOR_ARRAY;\n" +
		"return super.visitFileFailed(l0,l1);\n" +
		"return (IOPredicate<T>) (((null) == (var0)) ? (Objects::isNull) : ((l0) -> {\n" +
		"public interface IOStream<T extends Object> {\nErase.test(var1,l0);\nErase.accept(var1,l0);\n}\n"
	os.Unsetenv("JDEC_COMMONS_IO_REMAINING_OFF")
	on := fixCommonsIoRemainingReconstructs(in)
	if strings.Contains(on, "this(IOCase.SENSITIVE,var2)") {
		t.Errorf("ON expected this() first, got:\n%s", on)
	}
	if !strings.Contains(on, "BiFunction<Integer, IOException, IOException>") {
		t.Errorf("ON expected 3-arg BiFunction, got:\n%s", on)
	}
	if !strings.Contains(on, "Erase.test(var1,(T)(l0))") {
		t.Errorf("ON expected Erase.test T-cast, got:\n%s", on)
	}
	t.Setenv("JDEC_COMMONS_IO_REMAINING_OFF", "1")
	if fixCommonsIoRemainingReconstructs(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestObjectUsedAsIntRewritesReadLength(t *testing.T) {
	in := "\tObject var9 = null;\n\t\t\t\t\tif ((-1) != (var9 = var0.read(var4,0,var6))){\n"
	os.Unsetenv("JDEC_OBJECT_AS_INT_OFF")
	on := fixObjectUsedAsInt(in)
	if !strings.Contains(on, "int var9 = 0;") {
		t.Errorf("ON expected int var9, got:\n%s", on)
	}
	t.Setenv("JDEC_OBJECT_AS_INT_OFF", "1")
	if fixObjectUsedAsInt(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestWildcardFileFilterThisFirstIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/WildcardFileFilter.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_COMMONS_IO_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "String[] var2 = new String[1]") && strings.Contains(on, "this(IOCase.SENSITIVE,var2)") {
		t.Errorf("ON still has this() after locals:\n%s", on)
	}
	t.Setenv("JDEC_COMMONS_IO_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "this(IOCase.SENSITIVE,var2)") {
		t.Errorf("OFF expected this() after locals, got:\n%s", off)
	}
}
