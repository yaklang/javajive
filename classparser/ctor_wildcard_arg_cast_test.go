package javaclassparser

// 承重测试: 同类 `this(...)` 构造器自调用的通配符实参造型 (ctorWildcardArgCast, kill-switch
// JDEC_CTOR_WILDCARD_CAST_OFF)。
//
// 无参构造器委派 `this((Comparator<? super K>) NATURAL_ORDER)`。NATURAL_ORDER 是 `Comparator<Comparable>`,
// 目标构造器形参是 `Comparator<? super K>`。泛型把两者都擦除成裸 `Comparator` 且不发 checkcast, 源码造型从
// 字节码消失, 故反编译器必须在自调用实参上重新补 `(Comparator<? super K>)`, 否则 javac 报
// "Comparator<Comparable> cannot be converted to Comparator<? super K>"。`this(...)` 自调用恒在实例构造器内,
// K 在作用域内, 造型可写。镜像 gson LinkedTreeMap / LinkedHashTreeMap 的 `this(NATURAL_ORDER)`。
// 安全边界: 只对 receiver 为 `this` 的自调用造型, 绝不对 `new 当前类(...)`(可能在静态工厂里 K 越界)造型;
// 内部类构造器签名省略合成 this$0 形参导致下标偏移, 故只在签名形参数==实参数时记录(offset-safe)。
// kill-switch 置位后造型消失(裸 this(NATURAL_ORDER)), 证明承重。

import (
	"os"
	"regexp"
	"testing"
)

var (
	ctorWildcardOnRe  = regexp.MustCompile(`this\(\(Comparator<\? super K>\)\(NATURAL_ORDER\)\)`)
	ctorWildcardOffRe = regexp.MustCompile(`this\(NATURAL_ORDER\)`)
)

func TestCtorWildcardArgCastIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/CtorWildcardSeed.class")
	if err != nil {
		t.Fatalf("read CtorWildcardSeed seed: %v", err)
	}

	// Fix ON (default): the `this(...)` self-call argument is cast `(Comparator<? super K>)`.
	os.Unsetenv("JDEC_CTOR_WILDCARD_CAST_OFF")
	on, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !ctorWildcardOnRe.MatchString(on) {
		t.Errorf("fix ON: expected `this((Comparator<? super K>)(NATURAL_ORDER))`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the cast disappears, leaving the bare `this(NATURAL_ORDER)` self-call -- the
	// exact "Comparator<Comparable> cannot be converted to Comparator<? super K>" recompile blocker the fix
	// removes -- proving it is load-bearing.
	t.Setenv("JDEC_CTOR_WILDCARD_CAST_OFF", "1")
	off, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if ctorWildcardOnRe.MatchString(off) {
		t.Errorf("fix OFF: cast must NOT appear (kill-switch not load-bearing), got:\n%s", off)
	}
	if !ctorWildcardOffRe.MatchString(off) {
		t.Errorf("fix OFF: expected bare `this(NATURAL_ORDER)` fallback, got:\n%s", off)
	}
}
