package javaclassparser

// 承重测试: 通配单型参「消费者」接口(`Predicate<? super X>` / `Consumer<? super X>`)接收者调用其消费方法
// (`test`/`apply`/`accept`)时, 参数按擦除类型读入被 capture 拒收, 需对接收者补 raw `((Predicate)(recv))` 造型。
//
// 镜像 guava Maps$FilteredEntryMap / $FilteredMapValues `this.predicate.apply(entry)`(字段
// `Predicate<? super Entry<K,V>>`)与 spring/fastjson2 同族: `? super` 通配令 `test`/`apply` 绑到
// `m(CAP)`, 传入按 raw `Map.Entry` 读入的实参被拒(`Entry cannot be converted to CAP#N`)。治法
// (kill-switch JDEC_WILDCARD_CONSUMER_RECV_OFF): 把接收者裸化为其 raw 类, 成 unchecked、行为等价的
// `m(Object)`。种子: 字段 `Predicate<? super Map.Entry<K,V>>` + `this.predicate.test(e)`(单类即可复现,
// 接收者通配类型经字段签名恢复)。ON=`((Predicate)(recv)).test` / OFF=裸 `recv.test`。

import (
	"os"
	"strings"
	"testing"
)

func TestWildcardConsumerReceiverRawCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/WildcardConsumerSeed.class")
	if err != nil {
		t.Fatalf("read WildcardConsumerSeed seed: %v", err)
	}

	// Fix ON (default): the wildcard consumer receiver is raw-cast so `test` binds to test(Object).
	os.Unsetenv("JDEC_WILDCARD_CONSUMER_RECV_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "((Predicate)(this.predicate)).test(") {
		t.Errorf("fix ON: expected raw receiver cast `((Predicate)(this.predicate)).test(`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the bare `this.predicate.test(...)` returns -- the exact "cannot be converted
	// to CAP#N" recompile blocker the fix removes -- proving it is load-bearing.
	t.Setenv("JDEC_WILDCARD_CONSUMER_RECV_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "((Predicate)(this.predicate)).test(") {
		t.Errorf("fix OFF: expected the raw receiver cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
