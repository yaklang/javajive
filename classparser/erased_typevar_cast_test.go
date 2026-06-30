package javaclassparser

// 承重测试: 擦除型类型变量的多余 upcast 抑制 (calleeParamIsErasedTypeVar)。当被调方法的形参是它自己签名里
// 的类型变量 C (描述符把它擦成边界 Comparable) 时, 给实参补 `(Comparable)` 只是无操作的向上造型, 却会让
// javac 在调用点把 C 推断成 Comparable 而非精确的 Integer, 破坏后续推断 (guava Range.closed /
// ContiguousSet.create 一族)。修复后这类造型被丢弃, 让精确实参类型驱动推断。kill-switch
// JDEC_NO_ERASED_TYPEVAR_NOCAST 置位后造型回归, 证明承重于本修复。

import (
	"os"
	"regexp"
	"testing"
)

func TestErasedTypeVarCastSuppressionIsLoadBearing(t *testing.T) {
	names := []string{"ErasedTV", "ErasedTVUser"}
	bytesByName := map[string][]byte{}
	for _, n := range names {
		data, err := os.ReadFile("testdata/regression/" + n + ".class")
		if err != nil {
			t.Fatalf("read seed %s: %v", n, err)
		}
		bytesByName[n] = data
	}
	resolver := func(internalName string) ([]byte, bool) {
		data, ok := bytesByName[internalName]
		return data, ok
	}
	impl := bytesByName["ErasedTVUser"]
	comparableCast := regexp.MustCompile(`pick\(\(Comparable\)`)
	plainArg := regexp.MustCompile(`pick\(Integer\.valueOf`)

	// Fix ON (default): the no-op `(Comparable)` upcast on a type-variable parameter is dropped.
	os.Unsetenv("JDEC_NO_ERASED_TYPEVAR_NOCAST")
	on, err := DecompileWithResolver(impl, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if comparableCast.MatchString(on) {
		t.Errorf("fix ON: erased-bound cast must be dropped, got:\n%s", on)
	}
	if !plainArg.MatchString(on) {
		t.Errorf("fix ON: expected uncast argument pick(Integer.valueOf...), got:\n%s", on)
	}

	// Fix OFF: the legacy no-op upcast reappears, proving the suppression is load-bearing.
	t.Setenv("JDEC_NO_ERASED_TYPEVAR_NOCAST", "1")
	off, err := DecompileWithResolver(impl, resolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !comparableCast.MatchString(off) {
		t.Errorf("fix OFF: expected legacy `(Comparable)` upcast (kill-switch not load-bearing), got:\n%s", off)
	}
}
