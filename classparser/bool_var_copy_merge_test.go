package javaclassparser

// 承重测试: if/else 两臂共用一个 boolean 槽位 —— 一臂复制/三元生成 int-0/1 (boolean default),
// 另一臂存 Z-返回调用的真 boolean 值, 合流后 boolean 用法 (`if (itemRefDetect)`、
// `previous = itemRefDetect`) 不再渲染 `int = boolean` / `boolean != int`
// (reachingBoolVarCopyMerge, kill-switch JDEC_BOOL_VAR_COPY_MERGE_OFF)。
//
// 镜像 fastjson2 FieldWriterList.writeList (`itemRefDetect = previousItemRefDetect`) 与
// ObjectWriterImplList.writeList (`itemRefDetect = (itemClassRefDetect && isRefDetect()) ? 1 : 0`)。
// javac 把复制臂编成 iload/istore 或 iconst/istore (int 范畴), DFS 遂在复制臂铸出 int 版本;
// boolean 臂的 AssignVarGuarded 拒绝 int/boolean 合并、另铸 boolean 变量。合流读 `if (itemRefDetect)`
// 与 `previous = itemRefDetect` 渲染成 `int = boolean` / `boolean != int`, javac 拒
// "boolean cannot be converted to int"。
// 修复把复制臂的 int ref (及其被证为 boolean 的 int-0/1 default) 重定型为 boolean (phi 证同变量)。
// kill-switch 置位后恢复 int/boolean 分裂, 证明承重。

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var (
	// Fix ON: the itemRefDetect slot is ONE boolean variable, no int declaration leaks.
	boolVarCopyOnRe = regexp.MustCompile(`boolean var5;`)
	// Fix OFF: the copy arm splits off an `int var5;` that the merge read cannot accept -- the
	// recompile blocker (`var5 = var2` renders int = boolean).
	boolVarCopyOffRe = regexp.MustCompile(`int var5;`)
)

func TestBoolVarCopyMergeIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/BoolVarCopyMergeSeed.class")
	if err != nil {
		t.Fatalf("read BoolVarCopyMergeSeed seed: %v", err)
	}

	// Fix ON (default): both arms continue ONE boolean slot, the post-merge read is consistently typed.
	os.Unsetenv("JDEC_BOOL_VAR_COPY_MERGE_OFF")
	on, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !boolVarCopyOnRe.MatchString(on) {
		t.Errorf("fix ON: expected unified `boolean var5;`, got:\n%s", on)
	}
	if boolVarCopyOffRe.MatchString(on) {
		t.Errorf("fix ON: must NOT split into `int var5;`, got:\n%s", on)
	}
	if !strings.Contains(on, "var5 = var2") && !strings.Contains(on, "var5 = this.isRefDetect()") {
		t.Errorf("fix ON: expected the copy arm and boolean arm to assign var5, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the copy arm splits off an int variable, leaving `int var5;` whose
	// `var5 = var2` (boolean) assignment javac rejects with "boolean cannot be converted to int" --
	// proving the fix is load-bearing.
	t.Setenv("JDEC_BOOL_VAR_COPY_MERGE_OFF", "1")
	off, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !boolVarCopyOffRe.MatchString(off) {
		t.Errorf("fix OFF: expected the `int var5;` split fallback, got:\n%s", off)
	}
}
