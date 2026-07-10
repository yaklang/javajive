package javaclassparser

// 承重测试: 试/捕 (try/catch) 两臂分别把 boolean 局部赋 true/false、合流后被读, 不再分裂成
// `Object varN = null` 幽灵量 + int/boolean 多相 (reachingBoolSiblingArmMerge,
// kill-switch JDEC_BOOL_SIBLING_ARM_MERGE_OFF)。
//
// 镜像 gson SqlTypesSupport.<clinit>: `boolean b; try { ...; b = true; } catch (...) { b = false; }
// USE(b);`。javac 把 `b = true/false` 编译成 iconst_1/iconst_0 + istore (int 范畴), 且两臂互不相交
// (try 体 vs catch 处理块), 谁都不到达谁——没有可供 reachingBoolDefaultMerge 锚定的支配性默认初始化。
// DFS 先访问一臂铸出 int 版本, 合流读绑定它, 之后的 boolean 用法 (putstatic ...:Z) 把它重定型为 boolean;
// 第二臂的 int-0/1 存储见到 current=<boolean 版本> 而 val=int, AssignVarGuarded (int 与 boolean 不可转)
// 遂分裂出新 int 变量。第一臂的 boolean 版本在合流处被读、却在第二臂路径上未定义, 兜底 pass 合成
// `Object var1 = null;`, boolean 字段赋值报 "Object cannot be converted to boolean"。
// 修复让第二臂的 int-0/1 存储延续那个 boolean 版本 (字面量重定型为 false/true)。kill-switch 置位后恢复
// 分裂, 证明承重。

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var (
	// Fix ON: both arms assign ONE boolean variable, read in scope and definitely assigned.
	// Tolerate the default-initializer form (`boolean var1 = false;`) added by initProximateSplitSlotDecl.
	boolSiblingArmOnRe = regexp.MustCompile(`boolean var1(?:\s*=\s*false)?;`)
	// Fix OFF: the merge read splits to a phantom `Object var1 = null` -- the recompile blocker.
	boolSiblingArmOffRe = regexp.MustCompile(`Object var1 = null;`)
)

func TestBoolSiblingArmMergeIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/BoolSiblingArmSeed.class")
	if err != nil {
		t.Fatalf("read BoolSiblingArmSeed seed: %v", err)
	}

	// Isolate the overlapping cross-scope orphan-rebind defense throughout BOTH measurements. The
	// later-added replayUnambiguousRebindings (JDEC_ORPHAN_GLOBAL_REBIND) provides defense-in-depth
	// over this exact try/catch slot, so with it ON the kill-switch OFF case no longer reproduces the
	// canonical `Object var1 = null;` phantom (the defect MORPHS into a different int/boolean split).
	// Holding it OFF here pins the sibling-arm merge in isolation, exactly as it was authored. (With
	// everything default ON the real decompile is correct -- both fixes cover this shape.)
	t.Setenv("JDEC_ORPHAN_GLOBAL_REBIND_OFF", "1")

	// Fix ON (default): the two try/catch arms continue ONE boolean slot, so the merge read is in scope.
	os.Unsetenv("JDEC_BOOL_SIBLING_ARM_MERGE_OFF")
	on, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !boolSiblingArmOnRe.MatchString(on) {
		t.Errorf("fix ON: expected unified `boolean var1;`, got:\n%s", on)
	}
	if strings.Contains(on, "Object var1 = null;") {
		t.Errorf("fix ON: must NOT split into `Object var1 = null;`, got:\n%s", on)
	}
	if strings.Contains(on, "var1 = true") == false || strings.Contains(on, "var1 = false") == false {
		t.Errorf("fix ON: expected both arms to assign var1 (true/false), got:\n%s", on)
	}

	// Fix OFF (kill-switch): the second arm splits off an int variable and the merge read binds a phantom
	// `Object var1 = null` that is then assigned to the boolean field -- the exact "Object cannot be
	// converted to boolean" recompile blocker the fix removes -- proving it is load-bearing.
	t.Setenv("JDEC_BOOL_SIBLING_ARM_MERGE_OFF", "1")
	off, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !boolSiblingArmOffRe.MatchString(off) {
		t.Errorf("fix OFF: expected the `Object var1 = null;` split fallback, got:\n%s", off)
	}
}
