package javaclassparser

// 承重测试: 泛型类构造器 + 方法引用实参的两个耦合修复
//   1. classLiteralArgToClassParam (kill-switch JDEC_CLASSLIT_ARG_NOCAST_OFF):
//      类字面量 `Integer.class` 是 `Class<Integer>`, 但其值类型报告为被表示类 `Integer`, 实参造型逻辑
//      误加 `(Class)(Integer.class)` 原始造型, 擦除泛型并压垮构造器推断。
//   2. genericCtorDiamond (kill-switch JDEC_CTOR_DIAMOND_OFF):
//      泛型类的 RAW `new CtorDiamondBox(...)` 把 `Function<String,T>` 形参擦成原始 `Function`,
//      javac 拒收方法引用("invalid method reference")。补回菱形 `<>` 恢复推断。
//
// 镜像 fastjson2 ObjectReaderImplFromString `new ...(Duration.class, Duration::parse)` 及其
// URI/Charset/Pattern/ZoneId/... 同族。两个 kill-switch 各自独立置位, 分别证明承重。

import (
	"os"
	"strings"
	"testing"
)

func ctorDiamondDecompile(t *testing.T) string {
	t.Helper()
	seed, err := os.ReadFile("testdata/regression/CtorDiamondSeed.class")
	if err != nil {
		t.Fatalf("read CtorDiamondSeed seed: %v", err)
	}
	// Resolver exposes the sibling generic class CtorDiamondBox so SiblingClassSig can confirm it is
	// generic (the diamond gate) and recover its constructor parameter types.
	resolver := func(internalName string) ([]byte, bool) {
		base := internalName[strings.LastIndexByte(internalName, '/')+1:]
		b, e := os.ReadFile("testdata/regression/" + base + ".class")
		if e != nil {
			return nil, false
		}
		return b, true
	}
	out, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	return out
}

func TestGenericCtorDiamondIsLoadBearing(t *testing.T) {
	// Hold the class-literal fix ON so we observe ONLY the diamond toggle.
	t.Setenv("JDEC_CLASSLIT_ARG_NOCAST_OFF", "")
	os.Unsetenv("JDEC_CLASSLIT_ARG_NOCAST_OFF")

	// Fix ON (default): the generic constructor with a method-reference argument carries the diamond.
	os.Unsetenv("JDEC_CTOR_DIAMOND_OFF")
	on := ctorDiamondDecompile(t)
	if !strings.Contains(on, "new CtorDiamondBox<>(") {
		t.Errorf("fix ON: expected diamond `new CtorDiamondBox<>(`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the constructor falls back to a RAW `new CtorDiamondBox(` -- the exact
	// "invalid method reference" recompile blocker the diamond removes -- proving it is load-bearing.
	t.Setenv("JDEC_CTOR_DIAMOND_OFF", "1")
	off := ctorDiamondDecompile(t)
	if strings.Contains(off, "new CtorDiamondBox<>(") || !strings.Contains(off, "new CtorDiamondBox(") {
		t.Errorf("fix OFF: expected raw `new CtorDiamondBox(` (no diamond), got:\n%s", off)
	}
}

func TestClassLiteralArgNoCastIsLoadBearing(t *testing.T) {
	// Hold the diamond fix ON so we observe ONLY the class-literal toggle.
	os.Unsetenv("JDEC_CTOR_DIAMOND_OFF")

	// Fix ON (default): the class literal flows in plainly, no `(Class)` upcast.
	os.Unsetenv("JDEC_CLASSLIT_ARG_NOCAST_OFF")
	on := ctorDiamondDecompile(t)
	if strings.Contains(on, "(Class)(Integer.class)") {
		t.Errorf("fix ON: must NOT wrap the class literal as `(Class)(Integer.class)`, got:\n%s", on)
	}
	if !strings.Contains(on, "Integer.class,") {
		t.Errorf("fix ON: expected the bare class literal `Integer.class,`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the spurious `(Class)` raw cast returns -- it erases the generic and is the
	// inference-collapsing blocker the fix removes -- proving it is load-bearing.
	t.Setenv("JDEC_CLASSLIT_ARG_NOCAST_OFF", "1")
	off := ctorDiamondDecompile(t)
	if !strings.Contains(off, "(Class)(Integer.class)") {
		t.Errorf("fix OFF: expected the `(Class)(Integer.class)` cast fallback, got:\n%s", off)
	}
}
