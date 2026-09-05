package javaclassparser

// 承重测试: commons-collections4 剩余 dump 重构 (TreeBidiMap.compare / IterableUtils.singletonList /
// MultiValueMap ArrayList.class / RangeEntryMap inToRange)。
// kill-switch: JDEC_COLLECTIONS4_REMAINING_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestCollections4RemainingReconstructsAreLoadBearing(t *testing.T) {
	in := strings.Join([]string{
		"public class TreeBidiMap<K extends Comparable<K>, V extends Comparable<V>> {",
		"	int var3 = compare(var1.getValue(),var2.getValue());",
		"}",
		"public class IterableUtils {",
		"	return Collections.singletonList(var6);",
		"}",
		"public class MultiValueMap<K, V> {",
		"	return multiValueMap(var0,ArrayList.class);",
		"}",
		" class AbstractPatriciaTrie$RangeEntryMap<K, V> {",
		"	this.inToRange(var2,false);",
		"	this.inFromRange(var2,false);",
		"}",
		"public class IteratorUtils {",
		"	Method var1 = var0.getClass().getMethod(\"iterator\",((Class[])(null)));",
		"												if (Iterator.class.isAssignableFrom(var1.getReturnType())){",
		"													Iterator var2 = ((Iterator)(var1.invoke(var0,((Object[])(null)))));",
		"													if ((var2) != (null)){",
		"														return (Iterator<?>) (var2);",
		"													}",
		"												}",
		"												}catch(RuntimeException var1){",
		"}",
		"public class Flat3Map {",
		"	public boolean containsValue(Object var1) {",
		"				case 3:",
		"					if (var1.equals(this.value3)){",
		"						return true;",
		"					}",
		"				default:",
		"				throw new RuntimeException();",
		"",
		"				}",
		"			return false;",
		"	}",
		"	public V put(K var1, V var2) {",
		"	}",
		"}",
	}, "\n")

	os.Unsetenv("JDEC_COLLECTIONS4_REMAINING_OFF")
	on := fixCollections4RemainingReconstructs(in)
	if !strings.Contains(on, "compare((V)(var1.getValue()),(V)(var2.getValue()))") {
		t.Errorf("ON: missing TreeBidiMap compare cast, got:\n%s", on)
	}
	if !strings.Contains(on, "Collections.singletonList((R)(var6))") {
		t.Errorf("ON: missing IterableUtils singletonList cast, got:\n%s", on)
	}
	if !strings.Contains(on, "multiValueMap(var0,(Class)(ArrayList.class))") {
		t.Errorf("ON: missing MultiValueMap Class cast, got:\n%s", on)
	}
	if !strings.Contains(on, "this.inToRange((K)(var2),false)") || !strings.Contains(on, "this.inFromRange((K)(var2),false)") {
		t.Errorf("ON: missing RangeEntryMap (K) casts, got:\n%s", on)
	}
	if strings.Contains(on, "public boolean containsValue") && strings.Contains(on, "throw new RuntimeException()") {
		t.Errorf("ON: expected Flat3Map containsValue default throw dropped, got:\n%s", on)
	}

	t.Setenv("JDEC_COLLECTIONS4_REMAINING_OFF", "1")
	off := fixCollections4RemainingReconstructs(in)
	if off != in {
		t.Errorf("OFF: expected identity, got:\n%s", off)
	}
}

func TestCollections4TreeBidiMapCompareCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/TreeBidiMap.class")
	if err != nil {
		t.Fatalf("read TreeBidiMap: %v", err)
	}
	os.Unsetenv("JDEC_COLLECTIONS4_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "compare((V)(var1.getValue()),(V)(var2.getValue()))") {
		t.Errorf("ON: expected (V) compare casts, got:\n%s", on)
	}

	t.Setenv("JDEC_COLLECTIONS4_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "compare((V)(var1.getValue()),(V)(var2.getValue()))") {
		t.Errorf("OFF: expected no (V) compare reconstruct, got:\n%s", off)
	}
	if !strings.Contains(off, "compare(var1.getValue(),var2.getValue())") {
		t.Errorf("OFF: expected raw compare, got:\n%s", off)
	}
}

func TestCollections4IterableUtilsSingletonListCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/IterableUtils.class")
	if err != nil {
		t.Fatalf("read IterableUtils: %v", err)
	}
	os.Unsetenv("JDEC_COLLECTIONS4_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "Collections.singletonList((R)(var6))") {
		t.Errorf("ON: expected singletonList((R)(var6)), got:\n%s", on)
	}

	t.Setenv("JDEC_COLLECTIONS4_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "singletonList((R)(var6))") {
		t.Errorf("OFF: expected no (R) reconstruct, got:\n%s", off)
	}
}

func TestCollections4MultiValueMapClassCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/MultiValueMap.class")
	if err != nil {
		t.Fatalf("read MultiValueMap: %v", err)
	}
	os.Unsetenv("JDEC_COLLECTIONS4_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "multiValueMap(var0,(Class)(ArrayList.class))") {
		t.Errorf("ON: expected raw Class cast, got:\n%s", on)
	}

	t.Setenv("JDEC_COLLECTIONS4_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "multiValueMap(var0,(Class)(ArrayList.class))") {
		t.Errorf("OFF: expected no Class reconstruct, got:\n%s", off)
	}
	if !strings.Contains(off, "multiValueMap(var0,ArrayList.class)") {
		t.Errorf("OFF: expected raw ArrayList.class, got:\n%s", off)
	}
}

func TestCollections4RangeEntryMapKeyCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/AbstractPatriciaTrie$RangeEntryMap.class")
	if err != nil {
		t.Fatalf("read AbstractPatriciaTrie$RangeEntryMap: %v", err)
	}
	os.Unsetenv("JDEC_COLLECTIONS4_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "this.inToRange((K)(var2),false)") || !strings.Contains(on, "this.inFromRange((K)(var2),false)") {
		t.Errorf("ON: expected (K) inToRange/inFromRange, got:\n%s", on)
	}

	t.Setenv("JDEC_COLLECTIONS4_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "inToRange((K)(var2),false)") || strings.Contains(off, "inFromRange((K)(var2),false)") {
		t.Errorf("OFF: expected no (K) reconstruct, got:\n%s", off)
	}
}

func TestCollections4Flat3MapContainsValueDefaultIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/Flat3Map.class")
	if err != nil {
		t.Fatalf("read Flat3Map: %v", err)
	}
	os.Unsetenv("JDEC_COLLECTIONS4_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	onCV := on
	if i := strings.Index(on, "public boolean containsValue"); i >= 0 {
		j := strings.Index(on[i:], "\n\tpublic ")
		if j > 0 {
			onCV = on[i : i+j]
		}
	}
	if strings.Contains(onCV, "throw new RuntimeException()") {
		t.Errorf("ON: containsValue must drop default throw, got:\n%s", onCV)
	}

	t.Setenv("JDEC_COLLECTIONS4_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	offCV := off
	if i := strings.Index(off, "public boolean containsValue"); i >= 0 {
		j := strings.Index(off[i:], "\n\tpublic ")
		if j > 0 {
			offCV = off[i : i+j]
		}
	}
	if !strings.Contains(offCV, "throw new RuntimeException()") {
		t.Errorf("OFF: containsValue expected default throw, got:\n%s", offCV)
	}
}
