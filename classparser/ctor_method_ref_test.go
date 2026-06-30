package javaclassparser

// 承重测试: 构造器方法引用渲染。`Type::new` 的 invokedynamic impl 句柄是 REF_newInvokeSpecial -> <init>,
// 渲染器须输出关键字 `new`, 不能过 SafeIdentifier(它把关键字 `new` 改写成 `new_`, 导致 `Type::new_`
// —— javac 视为「invalid method reference」, 去找一个名为 new_ 的方法)。kill-switch
// JDEC_CTOR_METHODREF_FIX_OFF 置位则回退到旧的损坏形态, 证明造型承重于本修复。

import (
	"os"
	"strings"
	"testing"
)

func TestCtorMethodRefIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/CtorMethodRefSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): constructor refs render as `Type::new`, never `Type::new_`.
	os.Unsetenv("JDEC_CTOR_METHODREF_FIX_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "ArrayList::new") || !strings.Contains(on, "AtomicInteger::new") {
		t.Errorf("fix ON: expected `Type::new` constructor refs, got:\n%s", on)
	}
	if strings.Contains(on, "::new_") {
		t.Errorf("fix ON: must not emit the mangled `::new_`, got:\n%s", on)
	}

	// Fix OFF: legacy sanitized form `::new_` reappears, proving the fix is load-bearing.
	t.Setenv("JDEC_CTOR_METHODREF_FIX_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "::new_") {
		t.Errorf("fix OFF: expected legacy mangled `::new_` (kill-switch not load-bearing), got:\n%s", off)
	}
}
