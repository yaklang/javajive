package javaclassparser

// 承重测试: 链式字段赋值中被 dup 复制的 `null` 字面量不再折叠成 `Object varN` 临时量
// (checkAndConvertRef 的 null 短路, kill-switch JDEC_NULL_DUP_FOLD_OFF)。
//
// `n.left = n.right = n.parent = null` 是链式字段赋值: javac 发 `aconst_null` 后用 `dup_x1` 把同一个 null
// 沿栈复制并存入三个字段。JavaJive 的 dup 机制会把被复制的值折成共享临时量, 而 null 的静态类型是
// `java.lang.Object`, 于是得到 `Object var = null` 并把它存进 `Node` 类型字段 -> javac 判
// "Object cannot be converted to Node"。因为 null 是免费、无副作用、不可变的常量, 修复让它留在栈上,
// 每个存储点直接重新物化 `null`(可无造型赋给任意引用类型)。镜像 gson LinkedHashTreeMap$AvlBuilder。
// kill-switch 置位后恢复 `Object var2 = null` 临时量折叠, 证明承重。

import (
	"os"
	"regexp"
	"testing"
)

var (
	nullDupOnRe  = regexp.MustCompile(`var1\.parent = null;`)
	nullDupOffRe = regexp.MustCompile(`Object var2 = null;`)
)

func TestNullDupFoldSuppressionIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/NullDupSeed.class")
	if err != nil {
		t.Fatalf("read NullDupSeed seed: %v", err)
	}

	// Fix ON (default): the chained null store re-materializes `null` directly at each field, with no
	// `Object varN = null` temp -- so nothing of type Object is stored into the Node fields.
	os.Unsetenv("JDEC_NULL_DUP_FOLD_OFF")
	on, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !nullDupOnRe.MatchString(on) {
		t.Errorf("fix ON: expected `var1.parent = null;`, got:\n%s", on)
	}
	if nullDupOffRe.MatchString(on) {
		t.Errorf("fix ON: must NOT emit `Object var2 = null;` temp, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the dup'd null folds back into an `Object var2 = null` temp that is then stored
	// into the Node fields -- the exact "Object cannot be converted to Node" recompile blocker the fix
	// removes -- proving it is load-bearing.
	t.Setenv("JDEC_NULL_DUP_FOLD_OFF", "1")
	off, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !nullDupOffRe.MatchString(off) {
		t.Errorf("fix OFF: expected `Object var2 = null;` temp fallback, got:\n%s", off)
	}
}
