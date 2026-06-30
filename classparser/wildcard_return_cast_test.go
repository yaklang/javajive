package javaclassparser

// 承重测试: 通配符返回造型。一个泛型方法 `<T> R<T> create()` 返回**同类**另一个方法
// `R<?> helper()` 的结果 (`return this.helper();`)。helper 的真实泛型返回是 `R<?>` (通配符),
// 调用点字节码描述符只剩擦除后的裸 `R`, 故 JavaJive 模型看到的是裸 R; 但 javac 用 helper 真实
// 签名 `R<?>`, 把通配符捕获成 CAP#1, 报 `R<CAP#1> cannot be converted to R<T>`。真源码因此带一个
// unchecked `(R<T>)` 造型 (gson JsonAdapterAnnotationTypeAdapterFactory.create -> getTypeAdapter)。
// 实现要点: typeVarReturnCast 对 `this.` 同类实例调用, 用 funcCtx.MethodSignature 复原被调方法的
// 真实泛型返回; 若其为**同擦除的通配符参数化** (`R<?>`), 补 `(R<T>)` 造型。
// kill-switch JDEC_WILDCARD_RET_CAST_OFF 置位后回退到裸 `return this.helper();`, 证明承重。

import (
	"os"
	"regexp"
	"testing"
)

// wildcardRetCastRe matches the recovered unchecked cast on the wildcard-returning sibling call.
var wildcardRetCastRe = regexp.MustCompile(`return \(List<T>\) \(this\.helper\(\)\)`)

// wildcardRetBareRe matches the bare (uncast) return of the sibling call (the OFF / legacy emission).
var wildcardRetBareRe = regexp.MustCompile(`return this\.helper\(\);`)

func TestWildcardReturnCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/WildcardRetSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the wildcard-returning sibling call gets the recovered `(List<T>)` cast.
	os.Unsetenv("JDEC_WILDCARD_RET_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !wildcardRetCastRe.MatchString(on) {
		t.Errorf("fix ON: expected recovered cast `return (List<T>) (this.helper())`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the cast disappears and create() falls back to the bare sibling return,
	// proving this fix (not some unrelated pass) re-synthesizes the cast.
	t.Setenv("JDEC_WILDCARD_RET_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if wildcardRetCastRe.MatchString(off) {
		t.Errorf("fix OFF: expected the recovered cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
	if !wildcardRetBareRe.MatchString(off) {
		t.Errorf("fix OFF: expected the bare `return this.helper();` fallback, got:\n%s", off)
	}
}
