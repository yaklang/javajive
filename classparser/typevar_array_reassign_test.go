package javaclassparser

// 承重测试: `<T> T[] toArray(T[] var1)` 把 Array.newInstance 的 Object[] 写回 T[] 局部,
// 以及 `var1[i] = Object` 元素存储, 均需补回被擦除的 `(T[])` / `(T)` 造型。
// 镜像 commons-collections4 AbstractLinkedList.toArray / AbstractMapBag.toArray。
// kill-switch: JDEC_TYPEVAR_ARRAY_REASSIGN_OFF (整数组重赋值) 与
// JDEC_TYPEVAR_ARRAY_ELEM_STORE_CAST_OFF (局部数组元素存储)。

import (
	"os"
	"strings"
	"testing"
)

func TestTypeVarArrayReassignCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/AbstractLinkedList.class")
	if err != nil {
		t.Fatalf("read AbstractLinkedList seed: %v", err)
	}

	os.Unsetenv("JDEC_TYPEVAR_ARRAY_REASSIGN_OFF")
	os.Unsetenv("JDEC_TYPEVAR_ARRAY_ELEM_STORE_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "var1 = (T[])") && !strings.Contains(on, "var1 = ((T[])") &&
		!strings.Contains(on, "var1 = (T[]) ") {
		t.Errorf("fix ON: expected `var1 = (T[])(Array.newInstance...)`, got:\n%s", on)
	}

	t.Setenv("JDEC_TYPEVAR_ARRAY_REASSIGN_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "var1 = (T[])") || strings.Contains(off, "var1 = ((T[])") ||
		strings.Contains(off, "var1 = (T[]) ") {
		t.Errorf("fix OFF: expected no `(T[])` reassign cast, got:\n%s", off)
	}
}

func TestTypeVarLocalArrayElemStoreCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/AbstractLinkedList.class")
	if err != nil {
		t.Fatalf("read AbstractLinkedList seed: %v", err)
	}

	os.Unsetenv("JDEC_TYPEVAR_ARRAY_ELEM_STORE_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "var1[") || (!strings.Contains(on, "] = (T)") && !strings.Contains(on, "] = (T) ")) {
		t.Errorf("fix ON: expected `var1[i] = (T)(...)` element store, got:\n%s", on)
	}

	t.Setenv("JDEC_TYPEVAR_ARRAY_ELEM_STORE_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "] = (T)") || strings.Contains(off, "] = (T) ") {
		t.Errorf("fix OFF: expected no `(T)` local array element store cast, got:\n%s", off)
	}
}
