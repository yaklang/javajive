package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The physical packet ends at its terminal ATHROW, not at a possibly remote
// continuation. All normal exits must reach the same original disabled/success
// continuation. Internal control remains acyclic; no new entry, side exit, or
// iteration is licensed. Exception priority is checked separately over this
// exact physical packet by nativeAssertionRegionClosed.
func nativeAssertionPacketExitsClosed(ops []*core.OpCode, start, endPC, join, codeSize int, work *workbudget.Budget) bool {
	if len(ops) > 8192 || start < 0 || start+1 >= len(ops) || ops[start] == nil || endPC > codeSize || endPC <= int(ops[start].CurrentOffset) || join < 0 || join >= codeSize {
		return false
	}
	first := int(ops[start].CurrentOffset)
	if join >= first && join < endPC {
		return false
	}
	joinKnown := false
	boundaries := map[int]bool{}
	terminal := false
	if work != nil && work.CheckAlloc(int64(len(ops))*24) != nil {
		return false
	}
	for _, op := range ops {
		if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
			return false
		}
		pc := int(op.CurrentOffset)
		if boundaries[pc] {
			return false
		}
		boundaries[pc] = true
		if pc >= first && pc < endPC && op.Instr.OpCode == core.OP_ATHROW {
			if pc+1 != endPC {
				return false
			}
			terminal = true
		}
		if int(op.CurrentOffset) == join {
			joinKnown = true
		}
	}
	if !joinKnown || !terminal {
		return false
	}
	for i := start + 1; i < len(ops) && int(ops[i].CurrentOffset) < endPC; i++ {
		op := ops[i]
		pc := int(op.CurrentOffset)
		kind := op.Instr.OpCode
		switch kind {
		case core.OP_GOTO, core.OP_GOTO_W, core.OP_IFEQ, core.OP_IFNE, core.OP_IFLT, core.OP_IFGE, core.OP_IFGT, core.OP_IFLE, core.OP_IF_ICMPEQ, core.OP_IF_ICMPNE, core.OP_IF_ICMPLT, core.OP_IF_ICMPGE, core.OP_IF_ICMPGT, core.OP_IF_ICMPLE, core.OP_IF_ACMPEQ, core.OP_IF_ACMPNE, core.OP_IFNULL, core.OP_IFNONNULL:
			width := 2
			if kind == core.OP_GOTO_W {
				width = 4
			}
			target, err := core.BranchTarget(pc, op.Data, width, codeSize)
			if err != nil || !boundaries[target] || target != join && (target <= pc || target >= endPC) {
				return false
			}
		case core.OP_TABLESWITCH, core.OP_LOOKUPSWITCH, core.OP_JSR, core.OP_JSR_W, core.OP_RET, core.OP_RETURN, core.OP_IRETURN, core.OP_LRETURN, core.OP_FRETURN, core.OP_DRETURN, core.OP_ARETURN:
			return false
		}
	}
	return true
}
