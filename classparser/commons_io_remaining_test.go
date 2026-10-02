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
	// javap identifies seven same-owner invokespecial <init> delegations.
	// Every emitted delegation must precede declarations; the String constructor
	// also retains requireWildcards inside the delegated array argument.
	assertReviewedConstructorDelegations(t, "testdata/regression/WildcardFileFilter.class", "WildcardFileFilter", "JDEC_COMMONS_IO_REMAINING_OFF", 7,
		"this(IOCase.SENSITIVE,new String[]{", "requireWildcards")
}

func TestWildcardThisRepairPreservesDescriptorPin(t *testing.T) {
	t.Setenv("JDEC_COMMONS_IO_REMAINING_OFF", "")
	in := "public WildcardFileFilter(String var1) {\n\t\tString[] var2 = new String[1];\n\t\tvar2[0] = ((String)(requireWildcards((Object)(var1))));\n\t\tthis(IOCase.SENSITIVE,var2);\n\t}"
	out := fixCommonsIoRemainingReconstructs(in)
	if strings.Contains(out, "this(IOCase.SENSITIVE,var2)") || strings.Count(out, "requireWildcards((Object)(var1))") != 1 {
		t.Fatalf("lost binding/evaluation or kept prelude: %s", out)
	}
}

func TestIORecoveryDoesNotAppendReturnAfterTerminalLoop(t *testing.T) {
	body := "package org.apache.commons.io;\nclass Probe {\n boolean read() {\n\t\tdo{}while(true);\n\t\t}\n\t}\n\tprivate static boolean contentEquals(Iterator<?> var0, Iterator<?> var1) { return true; }"
	if got := fixCommonsIoRemainingReconstructs(body); got != body {
		t.Fatalf("appended an unreachable return:\n%s", got)
	}
}
