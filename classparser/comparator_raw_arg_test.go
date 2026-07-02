package javaclassparser

// 承重测试: 传给 JDK 排序/查找静态方法(Arrays.sort/binarySearch, Collections.sort/binarySearch)的
// `Comparator<? super K>` 实参需补 raw `(Comparator)` 造型。源码 `Arrays.sort((K[]) arr, comparator)`
// 可编译, 但 `(K[])` 数组造型在字节码里擦成 `(Object[])`, 反编译渲染成 `sort((Object[]) arr, comparator)`,
// javac 按 `sort(T[], Comparator<? super T>)`(T 推断为 Object)复解析拒绝该捕获("no suitable method for
// sort(Object[], Comparator<CAP#1>)"; guava ImmutableSortedMap$Builder/ImmutableSortedMultiset$Builder/
// ImmutableList, tree 171->166)。治法(kill-switch JDEC_COMPARATOR_RAW_ARG_OFF): raw `(Comparator)` 造型
// 使其成为 unchecked、行为等价的调用; Comparator 位置以稳定的描述符形参识别; lambda/方法引用实参排除
// (raw 会擦掉其形参推断类型)。种子 = ComparatorRawArgSeed.order。

import (
	"os"
	"regexp"
	"testing"
)

// comparatorRawArgRe matches the re-emitted raw cast on the sort() comparator arg: `(Comparator)(this.comparator)`.
var comparatorRawArgRe = regexp.MustCompile(`\(Comparator\)\(this\.comparator\)`)

func TestComparatorRawArgCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ComparatorRawArgSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the comparator arg gets a raw `(Comparator)` cast so the sort() call resolves.
	os.Unsetenv("JDEC_COMPARATOR_RAW_ARG_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !comparatorRawArgRe.MatchString(on) {
		t.Errorf("fix ON: expected raw `(Comparator)(this.comparator)` cast, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the cast disappears (the capture-conflicting bare arg reappears),
	// proving this fix is what re-emits it.
	t.Setenv("JDEC_COMPARATOR_RAW_ARG_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if comparatorRawArgRe.MatchString(off) {
		t.Errorf("fix OFF: expected the raw cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
