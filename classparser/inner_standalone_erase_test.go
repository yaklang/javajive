package javaclassparser

// 承重测试: 扁平内部类对**未声明外层类型变量**在**独立位置**(非类型实参)的擦除
// (JDEC_INNER_STANDALONE_ERASE_OFF), 与 RawEraseSeed 的类型实参 raw-erase 互补。
//
// `StandaloneEraseSeed$Itr<T>` 是泛型外层 `StandaloneEraseSeed<K, V>` 的非静态内部类, 自身又带形参 `<T>`。
// 它把外层变量 K, V 用作**独立类型**(而非 `Foo<K>` 的类型实参): 字段 `K key`、具体方法返回 `K peek()`、
// 抽象方法参数 `out(K, V)`。扁平成顶层单元后:
//
//   - `Foo<K>` 可被 raw-erase 去掉 `<...>` 变 `Foo`; 但裸的独立 `K` 没有 `<...>` 可去, 原样渲染即未声明
//     `K` -> javac "cannot find symbol: class K"。
//
// 治本: 独立位置渲染该变量的 JVM 擦除 (此处无界 -> java.lang.Object), 含抽象方法参数。
// 配套: 自带 K,V 的无形参兄弟子类 (`$Sub` / guava AbstractMapBasedMultimap$1) 经 ForceParamEraseTypeVars
// 把覆写参数同样擦成 Object, 保住 override (不再 "same erasure, yet neither overrides")。
//
// 关键: Itr 的擦除集仅由自身字节码推导, 不依赖跨类 resolver, 故单类 Decompile 即触发。Sub 的
// 参数强制擦除需要 resolver 看见 Itr 是 own-formal 扁平兄弟。kill-switch 置位后回退到裸 `K`。

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var (
	// 字段/具体返回的独立 K 被擦成 Object。
	standaloneEraseFieldOnRe  = regexp.MustCompile(`Object key;`)
	standaloneEraseFieldOffRe = regexp.MustCompile(`\bK key;`)
	standaloneErasePeekOnRe   = regexp.MustCompile(`Object peek\(`)
	standaloneErasePeekOffRe  = regexp.MustCompile(`\bK peek\(`)
	// 抽象方法参数现在也擦成 Object, 与字段/具体返回一致; 配套 ForceParamEraseTypeVars 让
	// 自带 K,V 的兄弟子类覆写同样擦成 Object, 保住 override。
	standaloneEraseAbstractOnRe  = regexp.MustCompile(`abstract T out\(Object var0, Object var1\)`)
	standaloneEraseAbstractOffRe = regexp.MustCompile(`abstract T out\(K var0, V var1\)`)
)

func TestInnerStandaloneEraseIsLoadBearing(t *testing.T) {
	itrBytes, err := os.ReadFile("testdata/regression/StandaloneEraseSeed$Itr.class")
	if err != nil {
		t.Fatalf("read Itr seed: %v", err)
	}

	// Fix ON (default): standalone K in the field, the concrete return, AND the abstract
	// method's parameters all erase to Object.
	os.Unsetenv("JDEC_INNER_STANDALONE_ERASE_OFF")
	on, err := Decompile(itrBytes)
	if err != nil {
		t.Fatalf("decompile Itr (fix ON) failed: %v", err)
	}
	if !standaloneEraseFieldOnRe.MatchString(on) {
		t.Errorf("fix ON: expected erased `Object key;`, got:\n%s", on)
	}
	if standaloneEraseFieldOffRe.MatchString(on) {
		t.Errorf("fix ON: undeclared `K key;` must NOT appear, got:\n%s", on)
	}
	if !standaloneErasePeekOnRe.MatchString(on) {
		t.Errorf("fix ON: expected concrete return erased to `Object peek(`, got:\n%s", on)
	}
	if !standaloneEraseAbstractOnRe.MatchString(on) {
		t.Errorf("fix ON: expected abstract params erased to `out(Object var0, Object var1)`, got:\n%s", on)
	}
	if standaloneEraseAbstractOffRe.MatchString(on) {
		t.Errorf("fix ON: undeclared abstract `out(K, V)` must NOT appear, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the standalone K renders verbatim -- the exact "cannot find symbol: class K"
	// recompile blocker the fix removes -- proving it is load-bearing. Abstract params fall back too.
	t.Setenv("JDEC_INNER_STANDALONE_ERASE_OFF", "1")
	off, err := Decompile(itrBytes)
	if err != nil {
		t.Fatalf("decompile Itr (fix OFF) failed: %v", err)
	}
	if !standaloneEraseFieldOffRe.MatchString(off) {
		t.Errorf("fix OFF: expected verbatim `K key;` fallback, got:\n%s", off)
	}
	if standaloneEraseFieldOnRe.MatchString(off) {
		t.Errorf("fix OFF: erased `Object key;` must NOT appear (kill-switch not load-bearing), got:\n%s", off)
	}
	if !standaloneErasePeekOffRe.MatchString(off) {
		t.Errorf("fix OFF: expected verbatim `K peek(` fallback, got:\n%s", off)
	}
	if !standaloneEraseAbstractOffRe.MatchString(off) {
		t.Errorf("fix OFF: expected verbatim abstract `out(K var0, V var1)` fallback, got:\n%s", off)
	}
	if standaloneEraseAbstractOnRe.MatchString(off) {
		t.Errorf("fix OFF: erased abstract `out(Object, Object)` must NOT appear, got:\n%s", off)
	}
}

func TestInnerStandaloneEraseOverrideParamsIsLoadBearing(t *testing.T) {
	subBytes, err := os.ReadFile("testdata/regression/StandaloneEraseSeed$Sub.class")
	if err != nil {
		t.Fatalf("read Sub seed: %v", err)
	}
	resolver := func(internalName string) ([]byte, bool) {
		b, e := os.ReadFile("testdata/regression/" + internalName + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	os.Unsetenv("JDEC_INNER_STANDALONE_ERASE_OFF")
	on, err := DecompileWithResolver(subBytes, resolver)
	if err != nil {
		t.Fatalf("decompile Sub (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "out(Object") {
		t.Errorf("fix ON: expected override params erased to Object, got:\n%s", on)
	}
	if strings.Contains(on, "out(K ") {
		t.Errorf("fix ON: override must NOT keep bare K, got:\n%s", on)
	}
	if !strings.Contains(on, "(V)") {
		t.Errorf("fix ON: expected `(V)` return cast of the Object-erased param, got:\n%s", on)
	}

	t.Setenv("JDEC_INNER_STANDALONE_ERASE_OFF", "1")
	off, err := DecompileWithResolver(subBytes, resolver)
	if err != nil {
		t.Fatalf("decompile Sub (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "out(K ") {
		t.Errorf("fix OFF: expected verbatim override `out(K ...)`, got:\n%s", off)
	}
}
