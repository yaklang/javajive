package javaclassparser

// 承重测试: JDEC_SAME_ERASURE_FIELD_RET_BRIDGE_OFF。泛型类 X<C> 的泛型方法 `X<C> all()` 体返回同类静态
// 字段 ALL(声明 X<Comparable>)。源码 raw 桥接 `(X<C>)(X)ALL` 被字节码擦除(全 no-op), 反编译得裸
// `return ALL;`; typeVarReturnCast 补 `(X<C>)`, 但 ALL 的值类型是 raw X(字段读值无实参), 看不出需桥接,
// javac 却按 ALL 声明泛型 `X<Comparable>` 判 `(X<C>)(X<Comparable>)` inconvertible。治法: 经同类
// FieldSignature 取 ALL 声明泛型, 同擦除异具体参时补 raw 桥接 `(X)`。镜像 guava Range.all()。
// 单类即复现(字段签名在本类、同类静态字段读), 无需 resolver。raw 桥接对同擦除恒合法, 只会修好绝不新增错误。

import (
	"os"
	"strings"
	"testing"
)

func TestSameErasureFieldReturnBridgeIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/SameErasureRetBridgeSeed.class")
	if err != nil {
		t.Fatalf("read SameErasureRetBridgeSeed seed: %v", err)
	}

	os.Unsetenv("JDEC_SAME_ERASURE_FIELD_RET_BRIDGE_OFF")
	on, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (bridge ON) failed: %v", err)
	}
	if !strings.Contains(on, "(SameErasureRetBridgeSeed<C>) (SameErasureRetBridgeSeed) (ALL)") {
		t.Errorf("bridge ON: expected raw bridge `(SameErasureRetBridgeSeed<C>) (SameErasureRetBridgeSeed) (ALL)`, got:\n%s", on)
	}

	t.Setenv("JDEC_SAME_ERASURE_FIELD_RET_BRIDGE_OFF", "1")
	off, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (bridge OFF) failed: %v", err)
	}
	if strings.Contains(off, "(SameErasureRetBridgeSeed) (ALL)") {
		t.Errorf("bridge OFF: expected the raw bridge to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
