package javaclassparser

// 承重测试: commons-collections4 剩余 dump 重构 (TreeBidiMap.compare / IterableUtils.singletonList /
// MultiValueMap ArrayList.class / RangeEntryMap inToRange)。
// kill-switch: JDEC_COLLECTIONS4_REMAINING_OFF。

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"regexp"
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

	path := "testdata/regression/MultiValueMap.class"
	desc := "(Ljava/util/Map;)Lorg/apache/commons/collections4/map/MultiValueMap;"
	raw, _, _ := reviewedFixtureMethod(t, path, "multiValueMap", desc)
	assertReviewedTypeVarMethod(t, raw, "multiValueMap", desc, "<K:Ljava/lang/Object;V:Ljava/lang/Object;>(Ljava/util/Map<TK;-Ljava/util/Collection<TV;>;>;)Lorg/apache/commons/collections4/map/MultiValueMap<TK;TV;>;")
	target := "(Ljava/util/Map;Ljava/lang/Class;)Lorg/apache/commons/collections4/map/MultiValueMap;"
	assertReviewedTypeVarMethod(t, raw, "multiValueMap", target, "<K:Ljava/lang/Object;V:Ljava/lang/Object;C::Ljava/util/Collection<TV;>;>(Ljava/util/Map<TK;-TC;>;Ljava/lang/Class<TC;>;)Lorg/apache/commons/collections4/map/MultiValueMap<TK;TV;>;")
	assertReviewedTypeVarInvoke(t, path, "multiValueMap", desc, 3, 184, "org/apache/commons/collections4/map/MultiValueMap", "multiValueMap", target)
	reviewedSeedSources(t, path, "JDEC_COLLECTIONS4_REMAINING_OFF", false, func(source string) {
		body := reviewedSourceMethod(t, source, `multiValueMap\(Map<K, \? super Collection<V>> [^)]*\)`)
		compact := compactReviewedGenericSource(body)
		if !strings.Contains(compact, "multiValueMap((Map)(") || !strings.Contains(compact, ",(Class)(ArrayList.class))") || !strings.Contains(compact, "(MultiValueMap<K,V>)(MultiValueMap)") {
			t.Fatal("lost original erased Map/Class factory tuple or generic result view: " + body)
		}
	})
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

// The former OFF expectation synthesized a RuntimeException which does not
// occur in this member. Both original switch defaults reach ICONST_0/IRETURN.
// TestAdversarialSiblingSwitchDefaultEqualityIdentityRoundTrip independently
// checks fallthrough effects, unchanged receiver fields and thrown identity.
func TestCollections4Flat3MapContainsValueDefaultIsLoadBearing(t *testing.T) {
	const path = "testdata/regression/Flat3Map.class"
	const descriptor = "(Ljava/lang/Object;)Z"
	raw, code, _ := reviewedFixtureMethod(t, path, "containsValue", descriptor)
	decoder := core.NewDecompiler(code.Code, nil)
	if err := decoder.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	for _, sw := range []struct {
		pc       uint16
		fallback int32
		targets  [3]int32
	}{{24, 79, [3]int32{70, 61, 52}}, {86, 151, [3]int32{138, 125, 112}}} {
		op := decoder.OpcodeByPC(sw.pc)
		if op == nil || op.Instr.OpCode != core.OP_TABLESWITCH || op.SwitchDefaultOffset != sw.fallback || op.SwitchJmpCase.Len() != 3 {
			t.Fatal("original size switch/default tuple changed")
		}
		for key, target := range sw.targets {
			actual, ok := op.SwitchJmpCase.Get(key + 1)
			if !ok || actual != target {
				t.Fatal("original case fallthrough entry changed")
			}
		}
	}
	assertReviewedOpcode(t, code, 79, core.OP_GOTO, 0, 72)
	assertReviewedOpcode(t, code, 151, core.OP_ICONST_0)
	assertReviewedOpcode(t, code, 152, core.OP_IRETURN)
	for _, op := range decoder.Opcodes() {
		if op.Instr.OpCode == core.OP_NEW || op.Instr.OpCode == core.OP_ATHROW {
			t.Fatal("original member now allocates or throws; review default contract again")
		}
	}
	for _, pc := range []uint16{117, 130, 143} {
		assertReviewedTypeVarInvoke(t, path, "containsValue", descriptor, pc, core.OP_INVOKEVIRTUAL, "java/lang/Object", "equals", descriptor)
	}
	assertReviewedTypeVarInvoke(t, path, "containsValue", descriptor, 12, core.OP_INVOKEVIRTUAL, "org/apache/commons/collections4/map/AbstractHashedMap", "containsValue", descriptor)
	assertReviewedSources(t, raw, "JDEC_COLLECTIONS4_REMAINING_OFF", func(source string) {
		body := reviewedSourceMethod(t, source, `public boolean containsValue\(Object (\w+)\)`)
		needle := requireReviewedPattern(t, body, `public boolean containsValue\(Object (\w+)\)`)[1]
		if strings.Contains(body, "throw ") || strings.Contains(body, "new RuntimeException") {
			t.Fatalf("invented exceptional default:\n%s", body)
		}
		starts := regexp.MustCompile(`switch\s*\(this\.size\)\s*\{`).FindAllStringIndex(body, -1)
		if len(starts) != 2 {
			t.Fatalf("original two size switches lost:\n%s", body)
		}
		for index, span := range starts {
			open := span[1] - 1
			close := javaMatchBrace(body, open)
			if close < 0 {
				t.Fatal("unclosed size switch")
			}
			sw := body[open+1 : close]
			requireReviewedPattern(t, sw, `default:\s*return false;`)
			positions := make([]int, 3)
			for slot := 1; slot <= 3; slot++ {
				label := []string{"", "1", "2", "3"}[slot]
				marker := "case " + label + ":"
				positions[slot-1] = strings.Index(sw, marker)
				if positions[slot-1] < 0 {
					t.Fatal("original size case lost")
				}
				next := len(sw)
				if slot > 1 {
					next = strings.Index(sw, "case "+[]string{"", "1", "2"}[slot-1]+":")
				}
				arm := sw[positions[slot-1]:next]
				if index == 0 {
					requireReviewedPattern(t, arm, `this\.value`+label+`\)\s*==\s*\(null\)`)
				} else {
					requireReviewedPattern(t, arm, regexp.QuoteMeta(needle)+`\.equals\(this\.value`+label+`\)`)
				}
				requireReviewedPattern(t, arm, `return true;`)
				if slot > 1 && strings.Contains(arm, "break;") {
					t.Fatal("original equality fallthrough became break")
				}
			}
			if !(positions[2] < positions[1] && positions[1] < positions[0]) {
				t.Fatal("original reverse case evaluation order changed")
			}
		}
		requireReviewedPattern(t, body, `return this\.delegateMap\.containsValue\(`+regexp.QuoteMeta(needle)+`\);`)
	})
}
