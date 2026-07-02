package javaclassparser

// 承重测试: `List<T>.set(int, E)` / `add(int, E)` 的元素实参(第二参数)必须补回被擦除的 `(T)` 造型。
// 描述符把元素形参擦成 Object, 故 `Object v = list.get(i); list.set(j, v);` 里 v 是 Object, 若不造型
// javac 按 set(int, T) 复解析报 `Object cannot be converted to T`(guava
// Iterables.removeIfFromRandomAccessList `var0.set(var3, var4)`, var0=`List<T>`; spring 同形态各 -1)。
// 治法(kill-switch JDEC_LIST_SET_PARAM_OFF): jdkMethodParamTypeArgIndex 增补 List 子族的
// set/add(int, E) —— 第二参数解析回接收者的元素类型实参, 既有 arg-cast 逻辑遂补出 `(T)`。
// 种子 = ListSetElemSeed.shift: 源码写显式 `(T) v`, 字节码擦除为无 checkcast, 反编译需重建。

import (
	"os"
	"regexp"
	"testing"
)

// listSetCastRe matches the re-emitted element cast on the set() call, e.g. `set(var3,(T)(var4))`.
var listSetCastRe = regexp.MustCompile(`\.set\(var\d+,\(T\)\(var\d+\)\)`)

func TestListSetParamCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ListSetElemSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the erased `(T)` element cast is re-emitted on `list.set(int, E)`.
	os.Unsetenv("JDEC_LIST_SET_PARAM_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !listSetCastRe.MatchString(on) {
		t.Errorf("fix ON: expected `set(...,(T)(...))` element cast, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the cast disappears (element arg rendered bare), proving this fix
	// is what re-emits it rather than some unrelated pass.
	t.Setenv("JDEC_LIST_SET_PARAM_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if listSetCastRe.MatchString(off) {
		t.Errorf("fix OFF: expected the element cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
