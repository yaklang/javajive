package javaclassparser

// 承重测试: 扁平内部类的外层形参元数对齐 (enclosing-arity reconciliation)。
//
// 非静态内部类被扁平成顶层 `Outer$Inner` 后, 其引用点的泛型签名编码了**完整**外层实参集
// (`LOuter<TK;TV;>.Inner;`), parseSigClassType 把这一整套 `<K, V>` 带到扁平名上; 但注入用的用量扫描
// 只能复原内部类体里**实际提及**的子集:
//
//   - `Inner` 既不提 K 也不提 V -> 用量扫描为空 -> 旧逻辑声明裸 `class Outer$Inner`, 与引用 `<K, V>`
//     不一致 (gson TreeTypeAdapter$GsonContextImpl -> "does not take parameters")。
//   - `KeyView` 只经 `extends AbstractSet<K>` 提及 K -> 用量扫描 {K} -> 旧逻辑声明 `class Outer$KeyView<K>`,
//     与引用 `<K, V>` 不一致 (gson LinkedTreeMap$KeySet -> "wrong number of type arguments; required 1")。
//
// 实现要点: 当用量子集 ⊆ 最近泛型外层类的形参集时, enclosingFormalTypeParamsForArity 用 foldSiblingResolver
// 取该外层类的**完整有序**形参, 整体回填到扁平内部类声明, 使声明/引用元数与顺序一致。
// 关键: 该治本依赖跨类 resolver (取外层类形参), 故必须用 DecompileWithResolver, 单类 Decompile 不触发。
// kill-switch JDEC_INNER_ENCLOSING_ARITY_OFF 置位后回退到用量子集 (空 / 部分) 声明, 证明承重。

import (
	"os"
	"regexp"
	"testing"
)

// Empty-usage case (GsonContextImpl-like): bare reference vs `<K, V>` reference.
var (
	innerEmptyFullRe = regexp.MustCompile(`class EnclosingAritySeed\$Inner<K, V>`)
	innerEmptyBareRe = regexp.MustCompile(`class EnclosingAritySeed\$Inner[ {]`)
)

// Partial-usage case (KeySet-like): `<K>` (used subset) vs `<K, V>` (full enclosing set).
var (
	innerPartialFullRe = regexp.MustCompile(`class EnclosingAritySeed\$KeyView<K, V>`)
	innerPartialSubRe  = regexp.MustCompile(`class EnclosingAritySeed\$KeyView<K>`)
)

func TestInnerEnclosingArityIsLoadBearing(t *testing.T) {
	innerBytes, err := os.ReadFile("testdata/regression/EnclosingAritySeed$Inner.class")
	if err != nil {
		t.Fatalf("read Inner seed: %v", err)
	}
	keyViewBytes, err := os.ReadFile("testdata/regression/EnclosingAritySeed$KeyView.class")
	if err != nil {
		t.Fatalf("read KeyView seed: %v", err)
	}
	outerBytes, err := os.ReadFile("testdata/regression/EnclosingAritySeed.class")
	if err != nil {
		t.Fatalf("read outer seed: %v", err)
	}
	// Resolver feeds the enclosing class's bytes by binary internal name (default package -> bare name),
	// so enclosingFormalTypeParamsForArity can read EnclosingAritySeed's `<K, V>` formal parameters.
	resolver := func(internalName string) ([]byte, bool) {
		if internalName == "EnclosingAritySeed" {
			return outerBytes, true
		}
		return nil, false
	}

	// Fix ON (default): both flattened inner classes adopt the full ordered enclosing set `<K, V>`,
	// matching the `EnclosingAritySeed$Inner<K, V>` / `$KeyView<K, V>` reference sites.
	os.Unsetenv("JDEC_INNER_ENCLOSING_ARITY_OFF")
	onInner, err := DecompileWithResolver(innerBytes, resolver)
	if err != nil {
		t.Fatalf("decompile Inner (fix ON) failed: %v", err)
	}
	if !innerEmptyFullRe.MatchString(onInner) {
		t.Errorf("fix ON (empty-usage): expected `class EnclosingAritySeed$Inner<K, V>`, got:\n%s", onInner)
	}
	onKeyView, err := DecompileWithResolver(keyViewBytes, resolver)
	if err != nil {
		t.Fatalf("decompile KeyView (fix ON) failed: %v", err)
	}
	if !innerPartialFullRe.MatchString(onKeyView) {
		t.Errorf("fix ON (partial-usage): expected `class EnclosingAritySeed$KeyView<K, V>`, got:\n%s", onKeyView)
	}

	// Fix OFF (kill-switch): the reconciliation is disabled, so the declarations fall back to the
	// usage-based subset (empty -> bare; {K} -> `<K>`), proving the enclosing-class walk re-synthesizes
	// the full arity.
	t.Setenv("JDEC_INNER_ENCLOSING_ARITY_OFF", "1")
	offInner, err := DecompileWithResolver(innerBytes, resolver)
	if err != nil {
		t.Fatalf("decompile Inner (fix OFF) failed: %v", err)
	}
	if innerEmptyFullRe.MatchString(offInner) {
		t.Errorf("fix OFF (empty-usage): expected `<K, V>` to disappear (kill-switch not load-bearing), got:\n%s", offInner)
	}
	if !innerEmptyBareRe.MatchString(offInner) {
		t.Errorf("fix OFF (empty-usage): expected bare `class EnclosingAritySeed$Inner` fallback, got:\n%s", offInner)
	}
	offKeyView, err := DecompileWithResolver(keyViewBytes, resolver)
	if err != nil {
		t.Fatalf("decompile KeyView (fix OFF) failed: %v", err)
	}
	if innerPartialFullRe.MatchString(offKeyView) {
		t.Errorf("fix OFF (partial-usage): expected `<K, V>` to disappear (kill-switch not load-bearing), got:\n%s", offKeyView)
	}
	if !innerPartialSubRe.MatchString(offKeyView) {
		t.Errorf("fix OFF (partial-usage): expected `<K>` subset fallback, got:\n%s", offKeyView)
	}
}
