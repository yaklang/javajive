package javaclassparser

// 承重测试: dup/dup2 一条指令物化多个临时变量的复合数组自增惯用法 (`this.f[expr]++`)。
// 当数组引用 (this.pathIndices) 与下标 (this.stackSize - 1) 都需要物化成临时变量时,
// 单条 dup2 opcode 连发两条 AssignStatement, 两节点共享 node.Id == opcode.Id;
// idToNode 只保留最后一个 (数组), 图连边只能经 predecessor -> idToNode[opcode.Id] 到达数组节点,
// 较早的下标赋值节点成为孤儿被丢弃 -> 其 JavaRef 永不被 RewriteVar 改名, 与数组临时名相撞,
// 产出 `int[] var1 = this.pathIndices; var1[var1] = var1[var1] + 1;` (int[] 当下标, javac 报
// "int[] cannot be converted to int")。这是 gson JsonReader pathIndices[stackSize-1]++ 的根因。
// 实现要点: 图连边后做 dup-family 多临时拼接 (code_analyser.go), 把同 id 的孤儿赋值节点按发射顺序
// 重新拼回主节点之前: preds -> 下标 -> 数组 -> store。
// kill-switch JDEC_DUP_MULTI_TEMP_SPLICE_OFF 置位后回退到丢弃孤儿 (复现 var1[var1]), 证明承重。
// 关键: 该治本是纯结构 (单类即触发), 无需 resolver。

import (
	"os"
	"regexp"
	"testing"
)

// dupSpliceIdxRe matches the recovered index temp assignment (`int varX = this.stackSize - 1`).
var dupSpliceIdxRe = regexp.MustCompile(`int var\d+ = \(this\.stackSize\) - \(1\);`)

// dupSpliceArrIdxRe captures the array/index var numbers of an array access `varA[varB]`. RE2 has no
// backreferences, so equality (the self-index bug) is checked in Go below.
var dupSpliceArrIdxRe = regexp.MustCompile(`var(\d+)\[var(\d+)\]`)

// hasSelfIndexedArray reports whether the source contains the legacy `varN[varN]` self-index bug.
func hasSelfIndexedArray(src string) bool {
	for _, m := range dupSpliceArrIdxRe.FindAllStringSubmatch(src, -1) {
		if m[1] == m[2] {
			return true
		}
	}
	return false
}

func TestDupMultiTempSpliceIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ArrCompoundSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the index materialization is spliced back in, so the compound store reads
	// `arrTmp[idxTmp]` with two distinct temps (no `var[var]` self-index).
	os.Unsetenv("JDEC_DUP_MULTI_TEMP_SPLICE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !dupSpliceIdxRe.MatchString(on) {
		t.Errorf("fix ON: expected recovered index temp `int varX = (this.stackSize) - (1);`, got:\n%s", on)
	}
	if hasSelfIndexedArray(on) {
		t.Errorf("fix ON: array temp must not be used as its own index (varN[varN]), got:\n%s", on)
	}

	// Fix OFF (kill-switch): the index assignment node is dropped again and the array temp collides
	// with the index name, reproducing `var1[var1]`. Proves THIS splice is what fixes it.
	t.Setenv("JDEC_DUP_MULTI_TEMP_SPLICE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !hasSelfIndexedArray(off) {
		t.Errorf("fix OFF: expected the legacy `varN[varN]` self-index bug to reappear (kill-switch not load-bearing), got:\n%s", off)
	}
}
