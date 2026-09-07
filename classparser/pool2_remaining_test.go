package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestPool2LinkedBlockingDequeThisFirstIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/LinkedBlockingDeque.class", "JDEC_POOL2_REMAINING_OFF",
		"this(2147483647);\n\t\tIterator var2 = null;",
		"Iterator var2 = null;\n\t\tthis(2147483647);")
}

func TestPool2GetGenericTypeSuperclassCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/PoolImplUtils.class", "JDEC_POOL2_REMAINING_OFF",
		"getGenericType(var0,(Class)(var1.getSuperclass()))",
		"getGenericType(var0,var1.getSuperclass())")
}

func TestPool2EvictionPolicyNSMECatchIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/BaseGenericObjectPool.class", "JDEC_ORIG14_REMAINING_OFF",
		"ClassCastException | NoSuchMethodException var4",
		"InvocationTargetException var4){\n\t\t\tthrow new IllegalArgumentException(new StringBuilder().append(\"Unable to create \")")
}

func TestPool2SecurityManagerPrintlnIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/SecurityManagerCallStack.class")
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("JDEC_POOL2_REMAINING_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "l0.get()") {
		t.Errorf("ON still has l0.get()")
	}
	t.Setenv("JDEC_POOL2_REMAINING_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "l0.get()") {
		t.Errorf("OFF missing l0.get()")
	}
}
