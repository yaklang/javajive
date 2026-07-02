package javaclassparser

// 承重测试: 通配 `Collection<? super E>` 接收者的 `recv.add(x)` 需补 raw `((Collection)(recv))` 接收者造型。
// add 在通配接收者上绑定为 `add(CAP)`; 源码给元素加造型 `c.add((E) o)`, 该造型擦成 no-op 消失, 反编译渲染裸
// `c.add(o)`(o: Object), javac 报 `Object cannot be converted to CAP#1`(guava Queues.drainUninterruptibly
// `var1.add(var7)`, tree 154->152)。治法(kill-switch JDEC_COLLECTION_ADD_RAW_RECV_OFF): 对通配 JDK
// Collection 接收者的 add/offer 调用补 raw `((Collection)(recv))` 造型(unchecked、行为等价 add(Object));
// 与 comparator.compare 通配接收者同理(唯一值形参即被捕获的元素类型变量)。种子 = CollectionAddWildcardSeed.drain。

import (
	"os"
	"regexp"
	"testing"
)

// collectionAddWildcardRe matches the re-emitted raw receiver cast: `((Collection)(var1)).add(`.
var collectionAddWildcardRe = regexp.MustCompile(`\(\(Collection\)\(var\d+\)\)\.add\(`)

func TestCollectionAddWildcardReceiverCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/CollectionAddWildcardSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the wildcard Collection receiver gets a raw `((Collection)(recv))` cast.
	os.Unsetenv("JDEC_COLLECTION_ADD_RAW_RECV_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !collectionAddWildcardRe.MatchString(on) {
		t.Errorf("fix ON: expected raw `((Collection)(recv)).add(` cast, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the receiver cast disappears (the capture-conflicting bare receiver
	// reappears), proving this fix is what re-emits it.
	t.Setenv("JDEC_COLLECTION_ADD_RAW_RECV_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if collectionAddWildcardRe.MatchString(off) {
		t.Errorf("fix OFF: expected the receiver cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
