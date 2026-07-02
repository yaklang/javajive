package javaclassparser

// 承重测试: 同类字段声明为 X<C> (以类型变量参数化), 与同 raw 擦除的泛型静态工厂调用做 == / != 比较时,
// 裸比较无目标类型使 javac 独立把工厂的自由返回类型变量推到上界 (X<Comparable>), 与字段 X<C> 不可比。治法
// (JDEC_CMP_GENERIC_FIELD_RAW_CAST_OFF): 对调用侧补一处被字节码擦除的 raw (X) 造型, 同擦除下 raw 与参数化
// 永远互相可比, 故该治本只会修好、绝不会新增编译错误。镜像 guava TreeRangeSet$ComplementRangesByLowerBound$1/$2
// 的 `nextComplementRangeLowerBound != Cut.aboveAll()`。

import (
	"os"
	"strings"
	"testing"
)

func TestCmpGenericFieldRawCastIsLoadBearing(t *testing.T) {
	defer os.Unsetenv("JDEC_CMP_GENERIC_FIELD_RAW_CAST_OFF")

	seed, err := os.ReadFile("testdata/regression/CmpGenericFieldRawCastSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_CMP_GENERIC_FIELD_RAW_CAST_OFF")
	on, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (cast ON): %v", err)
	}
	if !strings.Contains(on, "(CmpBound)(CmpBound.top())") {
		t.Errorf("cast ON: expected raw cast `(CmpBound)(CmpBound.top())`, got:\n%s", on)
	}

	os.Setenv("JDEC_CMP_GENERIC_FIELD_RAW_CAST_OFF", "1")
	off, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (cast OFF): %v", err)
	}
	if strings.Contains(off, "(CmpBound)(CmpBound.top())") {
		t.Errorf("cast OFF: expected the raw cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
	if !strings.Contains(off, "CmpBound.top()") {
		t.Errorf("cast OFF: expected the bare `CmpBound.top()` call, got:\n%s", off)
	}
}
