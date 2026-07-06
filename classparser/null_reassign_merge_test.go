package javaclassparser

// 承重测试: 已定型的局部先存入具体引用值 (`Character c = Character.valueOf(chars[i])`), 又在其中一条分支
// 被重赋 null (`if (c.charValue() == 0) c = null;`), 随后在分支合流后被读 (`sink(c)`)。定型 def 与
// `= null` def 都到达合流读, 故是同一个变量 (null 可赋给任意引用类型)。
// (reachingRefSlotNullReassignMerge, kill-switch JDEC_REF_SLOT_NULL_REASSIGN_MERGE_OFF)。
//
// 镜像 snakeyaml Resolver.addImplicitResolver 与 commons-lang3 LocaleUtils.countriesByLanguage:
// 不修时 AssignVarGuarded 见 null 值不携带具体类型、无法匹配 current 的 Character 引用, 遂为 null 存储
// 另铸一个 Object 变量; 定型 def 只剩 `c.charValue()` 一处使用被单用折叠吃进条件、其存储被丢弃, 合流读
// 绑到那个只在 null 分支赋值的新变量 -> javac "variable c might not have been initialized"。
// 修复让 null 存储延续那个定型变量 (成为普通 `c = null` 重赋值)。kill-switch 置位后恢复分裂+折叠, 证明承重。

import (
	"os"
	"strings"
	"testing"
)

func TestNullReassignMergeIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/NullReassignSeed.class")
	if err != nil {
		t.Fatalf("read NullReassignSeed seed: %v", err)
	}

	// Fix ON (default): the typed def and the `= null` store continue ONE Character variable, so the
	// post-branch read is in scope and the declaration carries the concrete type.
	os.Unsetenv("JDEC_REF_SLOT_NULL_REASSIGN_MERGE_OFF")
	on, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "Character var3 = Character.valueOf(") {
		t.Errorf("fix ON: expected a single `Character var3 = Character.valueOf(...)` declaration, got:\n%s", on)
	}
	if !strings.Contains(on, "var3 = null;") || !strings.Contains(on, "sink(var3)") {
		t.Errorf("fix ON: expected `var3 = null;` reassignment and `sink(var3)` read of one variable, got:\n%s", on)
	}
	if strings.Contains(on, "Object var3;") {
		t.Errorf("fix ON: must NOT split into a bare `Object var3;`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the null store splits off a fresh Object variable, the typed store is
	// single-use-folded into the condition and dropped, and the post-branch read binds the Object var
	// that is assigned only on the null branch -- the definite-assignment blocker this merge removes.
	t.Setenv("JDEC_REF_SLOT_NULL_REASSIGN_MERGE_OFF", "1")
	off, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "Object var3;") {
		t.Errorf("fix OFF: expected the split `Object var3;` fallback (kill-switch load-bearing), got:\n%s", off)
	}
	if strings.Contains(off, "Character var3 = Character.valueOf(") {
		t.Errorf("fix OFF: expected the typed store to be dropped, got:\n%s", off)
	}
}
