package javaclassparser

// 承重测试: 三元(条件)表达式存入通配符类型变量字段时的存储造型 (wildcardFieldStoreCast 的三元分支,
// kill-switch JDEC_WILDCARD_FIELD_CAST_OFF)。
//
// `this.comparator = c != null ? c : NATURAL_ORDER` 把一个条件表达式存入声明为 `Comparator<? super K>`
// 的字段(通配符且提及类型变量 K)。一个分支(c)正是该类型, 另一分支(NATURAL_ORDER)是
// `Comparator<Comparable>`。三元是 poly 表达式, JavaJive 把其类型算成两臂的 merge(TernaryExpression.Type
// -> MergeTypes), 未解析的 merge 会沉默地保留第一臂类型, 于是值报告成精确的 `Comparator<? super K>`,
// 掩盖了 NATURAL_ORDER 臂不兼容。裸存储被 javac 判 "bad type in conditional expression"。显式
// `(Comparator<? super K>)(...)` 把整个条件重定为 poly 目标, 两臂都按字段类型(unchecked)转换, 即可重编译。
// 镜像 gson LinkedTreeMap / LinkedHashTreeMap 的 NATURAL_ORDER。
// kill-switch 置位后造型消失(回退到裸三元), 证明承重。

import (
	"os"
	"regexp"
	"testing"
)

var (
	ternaryFieldCastOnRe  = regexp.MustCompile(`this\.comparator = \(Comparator<\? super K>\) \(`)
	ternaryFieldCastOffRe = regexp.MustCompile(`this\.comparator = \(\(var1\) != \(null\)\) \? `)
)

func TestTernaryWildcardFieldStoreCastIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/TernaryFieldSeed.class")
	if err != nil {
		t.Fatalf("read TernaryFieldSeed seed: %v", err)
	}

	// Fix ON (default): the conditional store is wrapped in `(Comparator<? super K>)(...)`.
	os.Unsetenv("JDEC_WILDCARD_FIELD_CAST_OFF")
	on, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !ternaryFieldCastOnRe.MatchString(on) {
		t.Errorf("fix ON: expected `this.comparator = (Comparator<? super K>) (...)`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the cast disappears, leaving the bare ternary store -- the exact
	// "bad type in conditional expression" recompile blocker the fix removes -- proving it is load-bearing.
	t.Setenv("JDEC_WILDCARD_FIELD_CAST_OFF", "1")
	off, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if ternaryFieldCastOnRe.MatchString(off) {
		t.Errorf("fix OFF: cast must NOT appear (kill-switch not load-bearing), got:\n%s", off)
	}
	if !ternaryFieldCastOffRe.MatchString(off) {
		t.Errorf("fix OFF: expected bare ternary store fallback, got:\n%s", off)
	}
}
