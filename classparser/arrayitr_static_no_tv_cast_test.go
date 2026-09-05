package javaclassparser

// 承重测试: Iterators$ArrayItr 的静态字段 EMPTY = new ArrayItr(new Object[0], ...)
// 不得带 `(T[])` —— T 在静态上下文不可见。thisCtorTypeVarArrayParamType 只对 this(...) 生效。
// 镜像 guava Iterators$ArrayItr。kill-switch: JDEC_THIS_CTOR_TYPEVAR_ARRAY_OFF 关掉后
// 静态 new 仍不得出现 `(T[])`（该开关只影响 this()）。

import (
	"os"
	"strings"
	"testing"
)

func TestArrayItrStaticEmptyHasNoTypeVarArrayCast(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/Iterators$ArrayItr.class")
	if err != nil {
		t.Fatalf("read ArrayItr: %v", err)
	}

	os.Unsetenv("JDEC_THIS_CTOR_TYPEVAR_ARRAY_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile: %v", err)
	}
	if strings.Contains(on, "static") && strings.Contains(on, "(T[])") {
		t.Errorf("static EMPTY must not mention (T[]):\n%s", on)
	}
	if !strings.Contains(on, "new Object[0]") && !strings.Contains(on, "new Object[]") {
		t.Errorf("expected Object[0] ctor arg, got:\n%s", on)
	}
}
