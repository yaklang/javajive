package javaclassparser

// 承重测试: 通配 `Comparator<?>` 接收者的 `recv.compare(a, b)` 需补 raw `((Comparator)(recv))` 接收者造型。
// 通配接收者使 compare 绑定为 `compare(CAP, CAP)`, 拒绝 Object 实参。源码先加宽接收者
// (`((Comparator<Object>) c).compare(a, b)`), 该造型擦成 no-op checkcast 从字节码消失, 反编译渲染成裸
// `c.compare(a, b)`, javac 报 `Object cannot be converted to CAP#1`(guava ImmutableSortedSet.unsafeCompare/
// TreeMultiset/ImmutableSortedMap$Builder, tree 166->154; commons-lang3 -1)。治法(kill-switch
// JDEC_COMPARATOR_RAW_RECV_OFF): 对通配 Comparator 接收者的 compare 调用补 raw `((Comparator)(recv))` 造型
// (unchecked、行为等价的 compare(Object, Object)); 三种接收者形态(参数/局部值、this.field、this.method())
// 皆由 receiverParamTypeArgs 恢复; lambda 接收者排除。种子 = ComparatorRawRecvSeed.cmp。

import (
	"os"
	"regexp"
	"testing"
)

// comparatorRawRecvRe matches the re-emitted raw receiver cast: `((Comparator)(var0)).compare(`.
var comparatorRawRecvRe = regexp.MustCompile(`\(\(Comparator\)\(var\d+\)\)\.compare\(`)

func TestComparatorRawReceiverCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ComparatorRawRecvSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the wildcard Comparator receiver gets a raw `((Comparator)(recv))` cast so
	// compare accepts the Object arguments.
	os.Unsetenv("JDEC_COMPARATOR_RAW_RECV_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !comparatorRawRecvRe.MatchString(on) {
		t.Errorf("fix ON: expected raw `((Comparator)(recv)).compare(` cast, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the receiver cast disappears (the capture-conflicting bare receiver
	// reappears), proving this fix is what re-emits it.
	t.Setenv("JDEC_COMPARATOR_RAW_RECV_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if comparatorRawRecvRe.MatchString(off) {
		t.Errorf("fix OFF: expected the receiver cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
