package javaclassparser

// 承重测试: lambda 体返回类型变量造型 (kill-switch JDEC_LAMBDA_RETURN_TYPEVAR_CAST_OFF)。
//
// 一个泛型方法返回 `Supplier<T>` / `Function<T,R>` 等, 其 lambda 体经 raw 接收者(或 Object 返回
// 的方法)取到一个擦除成 Object 的值; 字节码里 lambda 的实现方法是擦除的 `Object m(...)`, 故
// instantiatedMethodType 的返回是 Object, 反编译器原样渲染 `() -> { return expr; }` 会被 javac 拒
// ("bad return type in lambda expression: Object cannot be converted to T")。源码原带 unchecked
// `return (T) expr;` 造型, 必须补回。判定: enclosing 方法的 Signature 返回类型是带类型变量的
// 参数化 FI, lambda 的 FI raw 类匹配且其 instantiatedMethodType 返回 Object。
//
// 镜像 fastjson2 ObjectReaderProvider.createObjectCreator `return () -> (T) objectReader.createInstance(0);`
// 与 ObjectReaderCreator.createBuildFunctionLambda `return (l0) -> (R) var1.invoke(...);`。

import (
	"os"
	"strings"
	"testing"
)

func TestLambdaReturnTypevarCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/LambdaReturnTypevarSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the Supplier/Function lambda bodies carry the unchecked `(T)`/`(R)` cast on the
	// erased-Object return expression, matching the source.
	os.Unsetenv("JDEC_LAMBDA_RETURN_TYPEVAR_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "return (T) (") {
		t.Errorf("fix ON: expected the Supplier lambda to cast its return with `(T)`, got:\n%s", on)
	}
	if !strings.Contains(on, "return (R) (") {
		t.Errorf("fix ON: expected the Function lambda to cast its return with `(R)`, got:\n%s", on)
	}

	// Fix OFF: the bare (uncastable) Object-returning lambda body reappears, proving the cast is
	// load-bearing -- javac would reject it as "bad return type in lambda expression: Object cannot be
	// converted to T".
	t.Setenv("JDEC_LAMBDA_RETURN_TYPEVAR_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "return (T) (") {
		t.Errorf("fix OFF: expected NO `(T)` return cast, got:\n%s", off)
	}
	if strings.Contains(off, "return (R) (") {
		t.Errorf("fix OFF: expected NO `(R)` return cast, got:\n%s", off)
	}
}
