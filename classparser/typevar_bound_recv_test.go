package javaclassparser

// 承重测试: 接收者静态类型为「带参数化上界的裸型变」时, 需从方法形参上界恢复其容器类型实参。
// `var2` 的静态类型是方法级型变 C, 其上界是参数化容器 `Collection<? super E>`; 字节码里 C 擦成上界擦除(裸
// Collection)、`it.next()` 擦成 Object, 反编译渲染 `var2` 为 C、实参为 Object, javac 按 C 上界
// `Collection<? super E>` 重解析 add 报 `Object cannot be converted to CAP#1`(guava FluentIterable.copyInto
// `var1.add(var3.next())`, tree 152->150; 同解锁 Multimaps.invertFrom `var1.put(...)`)。治法(kill-switch
// JDEC_TYPEVAR_BOUND_RECV_OFF): receiverParamTypeArgs 从当前方法形参段恢复 C 的参数化上界
// `Collection<? super E>`, 再由通配 Collection 接收者造型补 raw `((Collection)(var2))`(unchecked、行为等价)。
// 种子 = TypeVarBoundRecvSeed.copyInto。

import (
	"os"
	"regexp"
	"testing"
)

// typeVarBoundRecvRe matches the raw receiver cast enabled by recovering C's parameterized bound:
// `((Collection)(var2)).add(`.
var typeVarBoundRecvRe = regexp.MustCompile(`\(\(Collection\)\(var\d+\)\)\.add\(`)

func TestTypeVarBoundReceiverResolutionIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/TypeVarBoundRecvSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): C's bound Collection<? super E> is recovered, so the wildcard Collection
	// receiver gets a raw `((Collection)(recv))` cast.
	os.Unsetenv("JDEC_TYPEVAR_BOUND_RECV_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !typeVarBoundRecvRe.MatchString(on) {
		t.Errorf("fix ON: expected raw `((Collection)(recv)).add(` cast, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the bound is not recovered, the receiver stays a bare type variable and
	// the cast disappears (the capture-conflicting bare receiver reappears), proving this fix re-emits it.
	t.Setenv("JDEC_TYPEVAR_BOUND_RECV_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if typeVarBoundRecvRe.MatchString(off) {
		t.Errorf("fix OFF: expected the receiver cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
