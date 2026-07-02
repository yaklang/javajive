package javaclassparser

// 承重测试, 覆盖两个耦合治本:
//
//  1. 非静态内部类构造器的泛型 Signature 省略了合成的首个 this$0 形参, 使 Signature 形参数 = 描述符形参数 - 1。
//     原逻辑仅在数目相等时才用 Signature 覆盖擦除形参, 故内部类构造器真实泛型形参停在 Object。治法
//     (JDEC_INNER_CTOR_SIG_ALIGN_OFF): 把 Signature 形参按尾部对齐到描述符形参 (保留 this$0)。种子
//     InnerGenericCtorSeed$Iter: Iter(T root) 的 root 还原为 T 后, Collections.singletonList(root) 推成
//     List<T> 可存入 List<List<T>> 字段; 关掉则 root 退回 Object, singletonList 推成 List<Object> 无法入库。
//     镜像 guava TreeTraverser$PreOrderIterator。
//
//  2. 上一治本把内部类构造器形参还原成具体参数化 (Iterable<Sub<T>>) 后, 存入声明为另一具体参数化
//     (Iterable<Base<T>>) 的同类字段会因泛型不变性报错; 源码原带一处被字节码擦除的 raw (Iterable) 造型。治法
//     (JDEC_PARAM_FIELD_RAW_CAST_OFF): 同类具体参数化字段的同擦除异参存值补 raw 造型。种子
//     InnerFieldRawCastSeed$Holder。镜像 guava TreeRangeMap$AsMapOfRanges。
//
// raw 造型对同擦除永远合法 (raw -> 参数化为非受检转换), 故该治本只会修好、绝不会新增编译错误。

import (
	"os"
	"strings"
	"testing"
)

func TestInnerCtorSigAlignIsLoadBearing(t *testing.T) {
	defer os.Unsetenv("JDEC_INNER_CTOR_SIG_ALIGN_OFF")
	defer os.Unsetenv("JDEC_PARAM_FIELD_RAW_CAST_OFF")

	// --- 治本 1: 内部类构造器泛型 Signature 尾对齐 ---
	iter, err := os.ReadFile("testdata/regression/InnerGenericCtorSeed$Iter.class")
	if err != nil {
		t.Fatalf("read Iter seed: %v", err)
	}
	os.Unsetenv("JDEC_INNER_CTOR_SIG_ALIGN_OFF")
	on, err := Decompile(iter)
	if err != nil {
		t.Fatalf("Iter decompile (align ON): %v", err)
	}
	if !strings.Contains(on, "T var2)") {
		t.Errorf("align ON: expected ctor param `T var2`, got:\n%s", on)
	}
	os.Setenv("JDEC_INNER_CTOR_SIG_ALIGN_OFF", "1")
	off, err := Decompile(iter)
	if err != nil {
		t.Fatalf("Iter decompile (align OFF): %v", err)
	}
	if strings.Contains(off, "T var2)") {
		t.Errorf("align OFF: expected the recovered `T var2` to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
	if !strings.Contains(off, "Object var2)") {
		t.Errorf("align OFF: expected erased `Object var2`, got:\n%s", off)
	}

	// --- 治本 2: 同擦除异参字段存值补 raw 造型 (保持 align ON, 使 var2 为具体 Iterable<Sub<T>>) ---
	os.Unsetenv("JDEC_INNER_CTOR_SIG_ALIGN_OFF")
	holder, err := os.ReadFile("testdata/regression/InnerFieldRawCastSeed$Holder.class")
	if err != nil {
		t.Fatalf("read Holder seed: %v", err)
	}
	os.Unsetenv("JDEC_PARAM_FIELD_RAW_CAST_OFF")
	hon, err := Decompile(holder)
	if err != nil {
		t.Fatalf("Holder decompile (rawcast ON): %v", err)
	}
	if !strings.Contains(hon, "this.items = (Iterable) (var2)") {
		t.Errorf("rawcast ON: expected `this.items = (Iterable) (var2)`, got:\n%s", hon)
	}
	os.Setenv("JDEC_PARAM_FIELD_RAW_CAST_OFF", "1")
	hoff, err := Decompile(holder)
	if err != nil {
		t.Fatalf("Holder decompile (rawcast OFF): %v", err)
	}
	if strings.Contains(hoff, "(Iterable) (var2)") {
		t.Errorf("rawcast OFF: expected the raw cast to disappear (kill-switch not load-bearing), got:\n%s", hoff)
	}
}
