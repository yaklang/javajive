package javaclassparser

// 承重测试: lambda / 方法引用作实参传给 RAW 泛型接收者的方法时, 必须补回函数式接口造型
// (kill-switch JDEC_LAMBDA_RAWRECV_CAST_OFF)。
//
// 镜像 fastjson2 JSONSchema.of `((ObjectReaderAdapter) reader).apply((Consumer<FieldReader>) e -> ...)`:
// 通过 RAW 引用调用方法会擦除整个方法签名 (JLS 4.8), `Consumer<Elem>` 退化成原始 `Consumer`
// (SAM accept(Object)), 显式类型 lambda `(Elem l0) -> ...` 于是被拒 ("incompatible parameter types
// in lambda expression")。补回 `(Consumer<Elem>)` 造型可恢复编译。种子里 RawRecvBox<T> 是泛型类,
// 由 resolver 提供其 class 使 SiblingClassSig 能确认其泛型 (造型门控条件之一)。

import (
	"os"
	"strings"
	"testing"
)

func rawRecvLambdaDecompile(t *testing.T) string {
	t.Helper()
	seed, err := os.ReadFile("testdata/regression/RawRecvLambdaSeed.class")
	if err != nil {
		t.Fatalf("read RawRecvLambdaSeed seed: %v", err)
	}
	// Expose the sibling generic class RawRecvBox so SiblingClassSig can confirm it is generic.
	resolver := func(internalName string) ([]byte, bool) {
		base := internalName[strings.LastIndexByte(internalName, '/')+1:]
		b, e := os.ReadFile("testdata/regression/" + base + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}
	out, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	return out
}

func TestLambdaRawReceiverCastIsLoadBearing(t *testing.T) {
	// Fix ON (default): the lambda argument carries the functional-interface cast, so the raw receiver
	// accepts the explicitly-typed lambda.
	os.Unsetenv("JDEC_LAMBDA_RAWRECV_CAST_OFF")
	on := rawRecvLambdaDecompile(t)
	if !strings.Contains(on, "(Consumer<Elem>)") {
		t.Errorf("fix ON: expected `(Consumer<Elem>)` functional-interface cast on the lambda arg, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the cast disappears -- the raw receiver then gets an explicitly-typed
	// lambda with no target type, the exact "incompatible parameter types in lambda expression"
	// recompile blocker the fix removes -- proving it is load-bearing.
	t.Setenv("JDEC_LAMBDA_RAWRECV_CAST_OFF", "1")
	off := rawRecvLambdaDecompile(t)
	if strings.Contains(off, "(Consumer<Elem>)") {
		t.Errorf("fix OFF: expected NO `(Consumer<Elem>)` cast, got:\n%s", off)
	}
	if !strings.Contains(off, ".apply((Elem") {
		t.Errorf("fix OFF: expected the bare `.apply((Elem ...) -> ` lambda, got:\n%s", off)
	}
}
