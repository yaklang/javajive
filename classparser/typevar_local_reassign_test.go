package javaclassparser

// 承重测试: 裸类型变量局部/参数重赋值 `T var1 = rawCall()` 需补回被擦除的 `(T)` 造型。
// ChainedTransformer.transform 在 raw `Transformer[]` 上调用 transform, 返回 Object 写回参数 T,
// javac 拒 "Object cannot be converted to T"。kill-switch: JDEC_TYPEVAR_LOCAL_REASSIGN_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestTypeVarLocalReassignCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ChainedTransformer.class")
	if err != nil {
		t.Fatalf("read ChainedTransformer seed: %v", err)
	}

	os.Unsetenv("JDEC_TYPEVAR_LOCAL_REASSIGN_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "var1 = (T)") && !strings.Contains(on, "var1 = (T) ") {
		t.Errorf("fix ON: expected `var1 = (T)(...transform...)`, got:\n%s", on)
	}

	t.Setenv("JDEC_TYPEVAR_LOCAL_REASSIGN_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "var1 = (T)") || strings.Contains(off, "var1 = (T) ") {
		t.Errorf("fix OFF: expected no `(T)` reassign cast, got:\n%s", off)
	}
}
