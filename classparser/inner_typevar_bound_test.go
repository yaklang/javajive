package javaclassparser

// 承重测试: 非静态内部类被扁平成顶层 `Outer$Inner` 单元后, 其继承自外层类的类型变量必须连同**bound**
// 一起重建。否则注入的裸 `<C>` (即 `C extends Object`) 用在需要 bound 的位置 (`ITVBox<C>`, ITVBox
// 要求 `C extends Comparable`) 会被 javac 拒为 "type argument C is not within bounds of type-variable C"
// (guava TreeRangeSet / TreeRangeMap / ImmutableRange* 家族的 `not within bounds` 阻断, ~90 行)。
// 实现要点: enclosingTypeParamBounds 沿二进制名 `$` 链用 foldSiblingResolver 加载外层类字节,
// 解析其形式类型参数的 bound, 重建成 `<C extends Comparable<?>>`。
// 关键: 该治本依赖跨类 resolver (取外层类 bound), 故必须用 DecompileWithResolver, 单类 Decompile 不触发。
// kill-switch JDEC_INNER_TYPEVAR_BOUND_OFF 置位后回退到裸名, 证明承重。

import (
	"os"
	"regexp"
	"testing"
)

// innerTypeVarBoundRe matches the flattened inner class declaration WITH the recovered bound.
var innerTypeVarBoundRe = regexp.MustCompile(`class ITVBoundSeed\$Ranges<C extends Comparable`)

// innerTypeVarBareRe matches the flattened inner class declaration with the bare (unbounded) variable.
var innerTypeVarBareRe = regexp.MustCompile(`class ITVBoundSeed\$Ranges<C>`)

func TestInnerTypeVarBoundIsLoadBearing(t *testing.T) {
	innerBytes, err := os.ReadFile("testdata/regression/ITVBoundSeed$Ranges.class")
	if err != nil {
		t.Fatalf("read inner seed: %v", err)
	}
	outerBytes, err := os.ReadFile("testdata/regression/ITVBoundSeed.class")
	if err != nil {
		t.Fatalf("read outer seed: %v", err)
	}
	// Resolver feeds the enclosing class's bytes by binary internal name (default package -> bare name),
	// so the bound recovery can read ITVBoundSeed's `<C extends Comparable<?>>` declaration.
	resolver := func(internalName string) ([]byte, bool) {
		if internalName == "ITVBoundSeed" {
			return outerBytes, true
		}
		return nil, false
	}

	// Fix ON (default): the enclosing class's bound is recovered, so the flattened inner class declares
	// `<C extends Comparable<?>>` instead of the bare `<C>`.
	os.Unsetenv("JDEC_INNER_TYPEVAR_BOUND_OFF")
	on, err := DecompileWithResolver(innerBytes, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !innerTypeVarBoundRe.MatchString(on) {
		t.Errorf("fix ON: expected recovered bound `<C extends Comparable...>` on the flattened inner class, got:\n%s", on)
	}

	// Fix OFF: the bound recovery is disabled, so the declaration falls back to the bare `<C>`, proving
	// the enclosing-class walk (not some unrelated pass) is what re-synthesizes the bound.
	t.Setenv("JDEC_INNER_TYPEVAR_BOUND_OFF", "1")
	off, err := DecompileWithResolver(innerBytes, resolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if innerTypeVarBoundRe.MatchString(off) {
		t.Errorf("fix OFF: expected the recovered bound to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
	if !innerTypeVarBareRe.MatchString(off) {
		t.Errorf("fix OFF: expected the bare `<C>` fallback declaration, got:\n%s", off)
	}
}
