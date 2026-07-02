package javaclassparser

// 承重测试: 同类泛型方法的形参是「方法作用域型变」(`<N> sink(List<N>, N)` 的第 2 形参 N)时, 若实参按
// 擦除读成 Object, 需从「见证实参」(另一形参 `List<N>` 的实参携带具体型实参)反推 N 并补 `(N)` 造型。
//
// 镜像 guava `ImmutableGraph.connectionsOf(Graph<N>, N)` / `Graphs.reachableNodes(Graph<N>, N)`: 调用
// `connectionsOf(var0, var3)` 中 var0 是 `Graph<N>`(见证)、var3 是从 raw `nodes().iterator()` 读出的
// Object → `Object cannot be converted to N`。既有 `sameClassMethodParamType`(只认类作用域型变、跳过
// static)与 `resolvedParamType`(只认实例接收者)都不覆盖此「方法作用域 + 见证反推」情形。治法
// (kill-switch JDEC_GENERIC_METHOD_WITNESS_OFF): 取同类泛型方法签名, 若形参 i 是方法作用域裸型变, 在其余
// 形参里找 `SomeClass<...N...>` 见证, 用见证实参对应位的具体型实参绑定 N(要求绑定结果为当前作用域内可写
// 型变 IsTypeParam), 复用既有造型链路补 `(N)`。种子: `sink(List<N>, N)` + `sink(list, (N) o)`(编译擦除)。
// ON=`sink(var0,(N)(var1))` / OFF=裸 `sink(var0,var1)`。

import (
	"os"
	"strings"
	"testing"
)

func TestGenericMethodWitnessArgCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/WitnessInferSeed.class")
	if err != nil {
		t.Fatalf("read WitnessInferSeed seed: %v", err)
	}

	// Fix ON (default): the method-scope type variable N is inferred from the witness `List<N>` argument
	// and the erased `(N)` cast on the Object argument is re-emitted.
	os.Unsetenv("JDEC_GENERIC_METHOD_WITNESS_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "sink(var0,(N)(var1))") {
		t.Errorf("fix ON: expected witness-inferred cast `sink(var0,(N)(var1))`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the bare `sink(var0,var1)` returns -- the exact "Object cannot be converted
	// to N" recompile blocker the fix removes -- proving it is load-bearing.
	t.Setenv("JDEC_GENERIC_METHOD_WITNESS_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "sink(var0,(N)(var1))") {
		t.Errorf("fix OFF: expected the `(N)` cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
