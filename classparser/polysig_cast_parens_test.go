package javaclassparser

// 承重测试: 多态签名 (MethodHandle.invoke/invokeExact) 返回造型需自括号, 以便作为后续调用接收者时精度正确。
// 源码 `(Boolean) mh.invoke(...)` 使 invoke 调用带非 Object 描述符返回 (Boolean), 反编译重新发出造型
// `(Boolean)(mh.invoke(...))`; 当该造型是后续调用的接收者 (`.booleanValue()`) 时, 单层外括号会误解析——
// `(Boolean)(x).booleanValue()` 绑定为 `(Boolean)((x).booleanValue())`(造型优先级低于调用), javac 遂对
// Object 型的 invoke 结果调 booleanValue() 报 `cannot find symbol: method booleanValue()`(fastjson2
// JSONReader:3130 `METHOD_HANDLE_HAS_NEGATIVE.invoke(...)`, tree 40->38)。治法(kill-switch
// JDEC_POLYSIG_CAST_PARENS_OFF): 与 OP_CHECKCAST 一致地自括号 `((Boolean)(mh.invoke(...)))`, 额外括号在任意
// 位置皆为合法 Java, 故不损害赋值/实参用法。种子 = PolySigCastRecvSeed.check。

import (
	"os"
	"regexp"
	"testing"
)

// polySigCastParensRe matches the self-parenthesized polymorphic-signature cast: `((Boolean)(`.
var polySigCastParensRe = regexp.MustCompile(`\(\(Boolean\)\(`)

func TestPolySigCastParensIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/PolySigCastRecvSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the polymorphic-signature return cast is self-parenthesized so it is safe as a
	// call receiver.
	os.Unsetenv("JDEC_POLYSIG_CAST_PARENS_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !polySigCastParensRe.MatchString(on) {
		t.Errorf("fix ON: expected self-parenthesized `((Boolean)(` cast, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the single-paren `(Boolean)(...)` form returns, which misparses as a
	// receiver -- proving this fix is what re-adds the outer parens.
	t.Setenv("JDEC_POLYSIG_CAST_PARENS_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if polySigCastParensRe.MatchString(off) {
		t.Errorf("fix OFF: expected the outer parens to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
