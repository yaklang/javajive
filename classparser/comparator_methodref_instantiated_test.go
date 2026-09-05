package javaclassparser

// 承重测试: Comparator 方法引用的实例化类型上行 (kill-switch JDEC_METHODREF_INSTANTIATED_TYPE_OFF)
//
// `String::compareTo` 的目标函数式接口是 `Comparator<String>`。字节码 invokedynamic 的
// instantiatedMethodType 记录 `(String,String)I`, 但 Comparator 原先不在
// inferLambdaTypeFromInstantiated 的 switch 里, 方法引用值类型停在 RAW `Comparator`。
// 存进 `Comparator<String>` 字段时 wildcardObjectAssignRawBridge 补 raw `(Comparator)` 造型,
// javac 拒收 `(Comparator)(String::compareTo)` ("invalid method reference")。
// 镜像 okhttp3.internal.Util.NATURAL_ORDER。

import (
	"os"
	"strings"
	"testing"
)

func TestComparatorMethodRefInstantiatedTypeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/NaturalOrderSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_METHODREF_INSTANTIATED_TYPE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "String::compareTo") {
		t.Errorf("fix ON: expected method reference `String::compareTo`, got:\n%s", on)
	}
	if strings.Contains(on, "(Comparator) (String::compareTo)") ||
		strings.Contains(on, "(Comparator)(String::compareTo)") {
		t.Errorf("fix ON: raw `(Comparator)` wrap must not wrap the method reference, got:\n%s", on)
	}

	t.Setenv("JDEC_METHODREF_INSTANTIATED_TYPE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "(Comparator) (String::compareTo)") &&
		!strings.Contains(off, "(Comparator)(String::compareTo)") {
		t.Errorf("fix OFF: expected raw `(Comparator)` wrap around `String::compareTo`, got:\n%s", off)
	}
}
