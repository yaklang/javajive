package javaclassparser

// 承重测试: MultiKey(K,K) 委派 `this((K[]) new Object[]{k1,k2}, false)`。字节码擦成
// Object[], 反编译若保留 `(Object[])` 则绑不到 `MultiKey(K[], boolean)`。
// kill-switch: JDEC_THIS_CTOR_TYPEVAR_ARRAY_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestMultiKeyArrayCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/MultiKey.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_THIS_CTOR_TYPEVAR_ARRAY_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "this((K[])") && !strings.Contains(on, "this(((K[])") {
		t.Errorf("fix ON: expected `this((K[]) new Object[]{...})`, got:\n%s", on)
	}

	t.Setenv("JDEC_THIS_CTOR_TYPEVAR_ARRAY_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "this((K[])") || strings.Contains(off, "this(((K[])") {
		t.Errorf("fix OFF: expected no `(K[])` this(...) cast, got:\n%s", off)
	}
}
