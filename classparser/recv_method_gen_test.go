package javaclassparser

// 承重测试: 同类 `this.method()` 接收者的泛型返回恢复。`box()` 声明返回 `List<E>`, 但调用点的值被
// 擦成裸 `List`(jar 内部方法返回不做实例化), 于是下游参数解析看不到类型实参, 无法给 `box().add((E) o)`
// 补回被擦除的 `(E)` 造型; javac 按 `Collection<E>.add(E)` 复解析裸 Object 实参 → `Object cannot be
// converted to E`(guava Multisets$EntrySet `this.multiset().setCount(objVal, ...)`, tree 175->171)。
// 治法(kill-switch JDEC_GENERIC_PARAM_RECV_METHOD_OFF): receiverParamTypeArgs 从同类 `this.m()` 的
// 泛型返回 Signature 恢复 `List<E>`, 既有 JDK 参数表(Collection.add)遂补出 `(E)`。种子用 JDK `List<E>`
// 接收者, 单类模式即可复现(无需 sibling-jar 上下文)。

import (
	"os"
	"regexp"
	"testing"
)

// recvMethodCastRe matches the re-emitted element cast on the chained add() call: `.add((E)(var1))`.
var recvMethodCastRe = regexp.MustCompile(`\.add\(\(E\)\(var\d+\)\)`)

func TestRecvMethodGenericReturnIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/RecvMethodGenSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): box()'s generic return List<E> is recovered, so the erased `(E)` cast is
	// re-emitted on the chained add() call.
	os.Unsetenv("JDEC_GENERIC_PARAM_RECV_METHOD_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !recvMethodCastRe.MatchString(on) {
		t.Errorf("fix ON: expected `.add((E)(...))` cast, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the receiver stays raw, so no type args are available and the cast
	// disappears -- proving this fix is what re-emits it.
	t.Setenv("JDEC_GENERIC_PARAM_RECV_METHOD_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if recvMethodCastRe.MatchString(off) {
		t.Errorf("fix OFF: expected the cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
