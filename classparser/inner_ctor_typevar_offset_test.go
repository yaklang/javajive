package javaclassparser

// 承重测试: 非静态内部类构造器 Signature 省略 this$0, this(...) 第二实参对应 Signature 第 0
// 形参 K, 需补 `(K)`。镜像 commons-collections4 AbstractPatriciaTrie$PrefixRangeMap 合成构造器。
// kill-switch: JDEC_THIS_CTOR_TYPEVAR_ARG_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestInnerCtorTypeVarOffsetCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/AbstractPatriciaTrie$PrefixRangeMap.class")
	if err != nil {
		t.Fatalf("read PrefixRangeMap: %v", err)
	}

	os.Unsetenv("JDEC_THIS_CTOR_TYPEVAR_ARG_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "this(var1,(K)") && !strings.Contains(on, "this(var1, (K)") {
		t.Errorf("fix ON: expected `this(var1,(K)(var2),...)`, got:\n%s", on)
	}

	t.Setenv("JDEC_THIS_CTOR_TYPEVAR_ARG_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "this(var1,(K)") || strings.Contains(off, "this(var1, (K)") {
		t.Errorf("fix OFF: expected no `(K)` this(...) cast, got:\n%s", off)
	}
}
