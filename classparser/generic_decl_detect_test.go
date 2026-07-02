package javaclassparser

// 承重测试: 类型为带空格的多参/通配泛型且以裸通配收尾 (`Map<K, Map<V, ?>> var2 = ...`) 的局部声明,
// 必须被 addMissingGeneratedLocalDecls 识别为"已声明", 否则 generatedLocalDeclRe (类型标识符禁止空格,
// var2 前紧邻的连续串是 `?>>`, 不以标识符字符开头) 认不出它, 误判 var2 未声明而在方法首部注入伪声明
// `Object var2 = null;`, 与真实声明构成重复 (javac "variable var2 is already defined")。
// 真实命中: guava MapMakerInternalMap$StrongKeyWeakValueSegment /
// $WeakKeyWeakValueSegment.setWeakValueReferenceForTesting, 第二个参数类型为
// `WeakValueReference<K, V, ? extends InternalEntry<K, V, ?>>` (guava tree 178 -> 176)。
// 治法 (kill-switch JDEC_GENERIC_DECL_DETECT_OFF): 逐行用兼容空格、行锚定的 castEscapeDeclLineRe
// (跳过关键字开头的伪类型如 `return varN;`) 补充识别此类泛型声明。纯增量: 只会抑制伪声明, 绝不注入。
// 种子 = GenericDeclSeed.set: `Map<K, Map<V, ?>> dst = src` 经中间 read() 调用与 consume(dst) 使用留存。

import (
	"os"
	"regexp"
	"testing"
)

// genericDeclPhantomRe matches the bogus hoisted declaration the fix suppresses: `Object varN = null;`.
var genericDeclPhantomRe = regexp.MustCompile(`Object var\d+ = null;`)

func TestGenericDeclDetectIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/GenericDeclSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the generic-typed declaration is recognized, so NO phantom `Object varN = null;`
	// is injected and there is no duplicate declaration.
	os.Unsetenv("JDEC_GENERIC_DECL_DETECT_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if genericDeclPhantomRe.MatchString(on) {
		t.Errorf("fix ON: expected NO phantom `Object varN = null;` declaration, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the generic declaration goes unrecognized and a phantom `Object varN = null;`
	// reappears (the duplicate declaration), proving this fix is what suppresses it.
	t.Setenv("JDEC_GENERIC_DECL_DETECT_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !genericDeclPhantomRe.MatchString(off) {
		t.Errorf("fix OFF: expected the phantom `Object varN = null;` to reappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
