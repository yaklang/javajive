package javaclassparser

// 承重测试(见证反推的跨类 + 裸型变见证扩展): 调用另一个类的泛型方法 `<N> N pick(N a, N b)` 时, 若第 2
// 实参按擦除读成 Object、而第 1 实参是「裸型变见证」(其静态类型即某作用域内型变 N), 需从见证实参反推 N 并
// 给第 2 实参补 `(N)` 造型。
//
// 镜像 guava `EndpointPair.ordered(N, N)`: `AbstractBaseGraph$2$1.apply` / `DirectedGraphConnections$4.apply`
// 里 `EndpointPair.ordered(var1, capturedObj)` 中 var1 形参 N 是裸型变见证、capturedObj 是被擦成 Object 的
// 捕获字段 → `Object cannot be converted to N`(inference variable N has incompatible bounds: N, Object)。
// 既有见证反推只认「同类 + `SomeClass<...N...>` 参数化见证」, 不覆盖此「跨类 + 裸型变见证」情形。治法
// (同一 kill-switch JDEC_GENERIC_METHOD_WITNESS_OFF): 跨类经 SiblingClassSig 取被调方法 (name,arity) 键签名;
// 见证匹配额外接受「裸型变形参」(其实参整体静态类型即绑定), 要求绑定为当前作用域内可写型变 IsTypeParam。
// 种子 = CrossWitnessSeed<N>.run 调用兄弟类 CrossWitnessPair.pick(node, (N) captured)(编译擦除), 需
// CrossWitnessPair 作为兄弟单元由 resolver 提供。ON=`pick(var1,(N)(this.captured))` / OFF=裸 `pick(var1,this.captured)`。

import (
	"os"
	"strings"
	"testing"
)

func TestCrossWitnessArgCastIsLoadBearing(t *testing.T) {
	sub, err := os.ReadFile("testdata/regression/CrossWitnessSeed.class")
	if err != nil {
		t.Fatalf("read CrossWitnessSeed seed: %v", err)
	}
	// Resolver for the sibling callee unit so CrossWitnessPair.pick's generic Signature (`<N>(N,N)N`) is
	// resolvable; the flattened seed unit alone cannot see it.
	resolver := func(internalName string) ([]byte, bool) {
		base := internalName[strings.LastIndexByte(internalName, '/')+1:]
		b, e := os.ReadFile("testdata/regression/" + base + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}

	// Fix ON (default): the method-scope type variable N is inferred from the bare type-variable witness
	// argument (node : N) and the erased `(N)` cast on the Object argument is re-emitted.
	os.Unsetenv("JDEC_GENERIC_METHOD_WITNESS_OFF")
	on, err := DecompileWithResolver(sub, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "pick(var1,(N)(this.captured))") {
		t.Errorf("fix ON: expected cross-class witness cast `pick(var1,(N)(this.captured))`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the bare `pick(var1,this.captured)` returns -- the exact "Object cannot be
	// converted to N" recompile blocker the fix removes -- proving it is load-bearing.
	t.Setenv("JDEC_GENERIC_METHOD_WITNESS_OFF", "1")
	off, err := DecompileWithResolver(sub, resolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "pick(var1,(N)(this.captured))") {
		t.Errorf("fix OFF: expected the `(N)` cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
