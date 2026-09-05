package javaclassparser

// 承重测试: `<T> T[] toArray(T[])` 里局部槽被 Object[] 合并后仍需 `(T[])` Array.newInstance
// 与 `(T)` 元素存储。镜像 commons-collections4 AbstractInputCheckedMapDecorator$EntrySet。
// kill-switch: JDEC_TYPEVAR_ARRAY_REASSIGN_OFF / JDEC_TYPEVAR_ARRAY_ELEM_STORE_CAST_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestEntrySetToArrayCastsAreLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/AbstractInputCheckedMapDecorator$EntrySet.class")
	if err != nil {
		t.Fatalf("read EntrySet: %v", err)
	}

	os.Unsetenv("JDEC_TYPEVAR_ARRAY_REASSIGN_OFF")
	os.Unsetenv("JDEC_TYPEVAR_ARRAY_ELEM_STORE_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "(T[])") && !strings.Contains(on, "(T[]) ") {
		t.Errorf("fix ON: expected `(T[])` Array.newInstance reassign, got:\n%s", on)
	}

	t.Setenv("JDEC_TYPEVAR_ARRAY_REASSIGN_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "var2 = (T[])") || strings.Contains(off, "var2 = ((T[])") {
		t.Errorf("fix OFF: expected no `(T[])` reassign, got:\n%s", off)
	}
}
