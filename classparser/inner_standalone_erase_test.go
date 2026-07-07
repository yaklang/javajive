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
// 治本: 独立位置渲染该变量的 JVM 擦除 (此处无界 -> java.lang.Object)。
// 例外: **抽象方法参数**保持裸变量 —— 若擦成 Object, 会让自带 K,V 的无形参兄弟子类重写不再 override
// (guava AbstractMapBasedMultimap$1 的 "same erasure, yet neither overrides")。
//
// 关键: 擦除集仅由 Itr 自身字节码推导, 不依赖跨类 resolver, 故单类 Decompile 即触发 (无 resolver 时默认
// Object)。kill-switch 置位后回退到裸 `K`, 证明承重。

import (
	"os"
	"regexp"
	"testing"
)

var (
	// 字段/具体返回的独立 K 被擦成 Object。
	standaloneEraseFieldOnRe  = regexp.MustCompile(`Object key;`)
	standaloneEraseFieldOffRe = regexp.MustCompile(`\bK key;`)
	standaloneErasePeekOnRe   = regexp.MustCompile(`Object peek\(`)
	standaloneErasePeekOffRe  = regexp.MustCompile(`\bK peek\(`)
	// 抽象方法参数在两种状态下都保持裸 K, V (SuppressStandaloneErase)。
	standaloneEraseAbstractRe = regexp.MustCompile(`abstract T out\(K var0, V var1\)`)
)

func TestInnerStandaloneEraseIsLoadBearing(t *testing.T) {
	itrBytes, err := os.ReadFile("testdata/regression/StandaloneEraseSeed$Itr.class")
	if err != nil {
		t.Fatalf("read Itr seed: %v", err)
	}

	// Fix ON (default): standalone K in the field and the concrete return erases to Object; the abstract
	// method's parameters keep the bare K, V.
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
	if !standaloneEraseAbstractRe.MatchString(on) {
		t.Errorf("fix ON: abstract params must stay bare `out(K var0, V var1)`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the standalone K renders verbatim -- the exact "cannot find symbol: class K"
	// recompile blocker the fix removes -- proving it is load-bearing. The abstract params are unaffected
	// by the switch (they are already suppressed) and stay bare in both states.
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
	if !standaloneEraseAbstractRe.MatchString(off) {
		t.Errorf("fix OFF: abstract params must still stay bare `out(K var0, V var1)`, got:\n%s", off)
	}
}
