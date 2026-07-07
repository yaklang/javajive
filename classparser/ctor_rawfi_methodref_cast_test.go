package javaclassparser

// 承重测试: 构造器 RAW 函数式接口形参位的方法引用造型 (kill-switch JDEC_CTOR_RAWFI_METHODREF_CAST_OFF)。
//
// 构造器(或静态方法)的某形参是 RAW 函数式接口(如 raw `BiConsumer`, SAM 为 accept(Object,Object)),
// 传入的 UNBOUND 实例方法引用 `Throwable::setStackTrace`(实现元数 (Throwable, StackTraceElement[]))
// 绑不到 raw (Object,Object) SAM, javac 报「invalid method reference」。源码原带
// `(BiConsumer<Throwable,StackTraceElement[]>) Throwable::setStackTrace` 造型; 从 invokedynamic
// instantiatedMethodType 恢复。镜像 fastjson2 ObjectReaderCreator
// `new FieldReaderStackTrace(..., Throwable::setStackTrace)`(CFR/Vineflower 亦丢此造型, 三方同败)。

import (
	"os"
	"strings"
	"testing"
)

func TestCtorRawFIMethodRefCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/MethodRefRawFICtorSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the raw-BiConsumer constructor argument carries the recovered
	// `(BiConsumer<Throwable, StackTraceElement[]>)` cast on the method reference.
	os.Unsetenv("JDEC_CTOR_RAWFI_METHODREF_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "(BiConsumer<Throwable, StackTraceElement[]>)(Throwable::setStackTrace)") {
		t.Errorf("fix ON: expected the cast `(BiConsumer<Throwable, StackTraceElement[]>)(Throwable::setStackTrace)`, got:\n%s", on)
	}

	// Fix OFF: the bare method reference reappears, proving the cast is load-bearing -- javac would
	// reject it as "invalid method reference" against the raw BiConsumer.accept(Object,Object) SAM.
	t.Setenv("JDEC_CTOR_RAWFI_METHODREF_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "(BiConsumer<Throwable, StackTraceElement[]>)") {
		t.Errorf("fix OFF: expected NO `(BiConsumer<Throwable, StackTraceElement[]>)` cast, got:\n%s", off)
	}
	if !strings.Contains(off, "Throwable::setStackTrace") {
		t.Errorf("fix OFF: expected the bare `Throwable::setStackTrace` method reference, got:\n%s", off)
	}
}
