package javaclassparser

// 承重测试: 传给 java.security.AccessController.doPrivileged 的 lambda / 方法引用实参必须补回
// (PrivilegedAction) 函数式接口造型。doPrivileged 在 PrivilegedAction<T> 与 PrivilegedExceptionAction<T>
// 上重载, 裸 lambda/方法引用(poly 表达式)对两者都适用, javac 拒为 "reference to doPrivileged is
// ambiguous"。invokedynamic 结果类型记录了确切的函数式接口, 修复据此补回源码里的造型。kill-switch
// JDEC_DOPRIVILEGED_LAMBDA_CAST_OFF 置位则回退裸实参, 复现歧义。真实命中: fastjson2
// DynamicClassLoader / JSONFactory。

import (
	"os"
	"strings"
	"testing"
)

func TestDoPrivilegedLambdaCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/DoPrivilegedLambdaSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): both call sites carry the (PrivilegedAction) functional-interface cast.
	os.Unsetenv("JDEC_DOPRIVILEGED_LAMBDA_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if n := strings.Count(on, "doPrivileged((PrivilegedAction)"); n < 2 {
		t.Errorf("fix ON: expected >=2 `doPrivileged((PrivilegedAction)` casts, got %d:\n%s", n, on)
	}

	// Fix OFF: the bare (ambiguous) argument reappears, proving the cast is load-bearing.
	t.Setenv("JDEC_DOPRIVILEGED_LAMBDA_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "doPrivileged((PrivilegedAction)") {
		t.Errorf("fix OFF: expected bare doPrivileged argument (kill-switch not load-bearing), got:\n%s", off)
	}
	if !strings.Contains(off, "doPrivileged(") {
		t.Errorf("fix OFF: expected a doPrivileged call, got:\n%s", off)
	}
}
