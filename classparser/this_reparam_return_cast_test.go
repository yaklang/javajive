package javaclassparser

// 承重测试: `cast()` 重参数化惯用法。非静态泛型方法的声明返回类型是**同一个类**但用方法自己的类型变量
// 参数化 (`<N1 extends N> C<N1>`), 方法体 `return this;`。`this` 是 `C<N>` (类自身参数化) 而非 `C<N1>`,
// 故真源码带一个擦除后无字节码的 unchecked `(C<N1>) this` 造型。历史实现把它丢了, 发裸 `return this;`,
// javac 报 "C<N> cannot be converted to C<N1>" (guava GraphBuilder / NetworkBuilder / ValueGraphBuilder /
// ElementOrder.cast() 四个整类, guava tree 365->360 / 171->167 blocker units)。
// 实现要点: typeVarReturnCast 在同擦除的 `return this` 上, 若返回类型的类型实参 != 类自身类型参数
// (returnArgsAreClassParams) 即补 `(C<N1>) this` 造型。
// kill-switch JDEC_THIS_REPARAM_CAST_OFF 置位后回退到裸 `return this;`, 证明承重。
// 关键: 该治本是同类的 (造型目标取自本方法/本类的类型参数), 故单类 Decompile 即可触发, 无需 resolver。

import (
	"os"
	"regexp"
	"testing"
)

// thisReparamCastRe matches the recovered unchecked cast on the cast() reparameterization method.
var thisReparamCastRe = regexp.MustCompile(`return \(ThisReparamSeed<N1>\) \(this\)`)

// thisReparamBareRe matches the bare (uncast) `return this;` body (the OFF / legacy emission of cast()).
var thisReparamBareRe = regexp.MustCompile(`ThisReparamSeed<N1> cast\(\) \{\s*return this;`)

// thisReparamSelfRe matches the IDENTITY self() method, whose return type IS the class's own
// parameterization (`ThisReparamSeed<N>`); it must NEVER be cast (the over-cast guard).
var thisReparamSelfRe = regexp.MustCompile(`ThisReparamSeed<N> self\(\) \{\s*return this;`)

func TestThisReparamReturnCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ThisReparamSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the reparameterizing cast() recovers the unchecked `(ThisReparamSeed<N1>)`
	// cast, while the identity self() stays uncast.
	os.Unsetenv("JDEC_THIS_REPARAM_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !thisReparamCastRe.MatchString(on) {
		t.Errorf("fix ON: expected recovered cast `return (ThisReparamSeed<N1>) (this)` on cast(), got:\n%s", on)
	}
	if !thisReparamSelfRe.MatchString(on) {
		t.Errorf("fix ON: identity self() must stay uncast `return this;`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the cast disappears and cast() falls back to the bare `return this;`,
	// proving this fix (not some unrelated pass) is what re-synthesizes the cast.
	t.Setenv("JDEC_THIS_REPARAM_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if thisReparamCastRe.MatchString(off) {
		t.Errorf("fix OFF: expected the recovered cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
	if !thisReparamBareRe.MatchString(off) {
		t.Errorf("fix OFF: expected the bare `return this;` fallback on cast(), got:\n%s", off)
	}
}
