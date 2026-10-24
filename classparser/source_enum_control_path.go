package javaclassparser

import (
	"encoding/binary"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A compiler/instrumenter may route class initialization through a finite chain
// of unconditional jumps before executing its declarations. Prove that the
// original has one complete, nonrepeating effect trace: every instruction is
// reached exactly once, only unconditional edges are elided, and the surviving
// instructions retain their physical order. A skipped effect, cycle, handler,
// conditional choice or reverse-ordered body needs a different control proof.
//
// This is a read-only view of ORIGINAL instructions, used to recognize the enum
// array protocol. Original PCs and verifier identities remain attached. Fresh
// executable editing still uses its complete unmodified instruction inventory;
// no original Code or control edge is copied into the output.
func enumArrayOriginalEntryPath(code *CodeAttribute, ops []*core.OpCode, work *workbudget.Budget) ([]*core.OpCode, bool) {
	if code == nil || len(code.ExceptionTable) != 0 || len(ops) == 0 || len(ops) > 65535 || !enumArrayInstructionSpans(code.Code, ops) || !nativeProofWork(work, int64(len(ops))+1) || work.CheckAlloc(int64(len(ops))*96) != nil {
		return nil, false
	}
	positions := make(map[int]int, len(ops))
	for i, op := range ops {
		positions[int(op.CurrentOffset)] = i
	}
	visited := make([]bool, len(ops))
	path := make([]*core.OpCode, 0, len(ops))
	at, count, last := 0, 0, -1
	for at >= 0 && at < len(ops) {
		if visited[at] || !nativeProofWork(work, 1) {
			return nil, false
		}
		visited[at] = true
		count++
		op := ops[at]
		pc, opcode := int(op.CurrentOffset), op.Instr.OpCode
		if opcode == core.OP_GOTO || opcode == core.OP_GOTO_W {
			var delta int64
			if opcode == core.OP_GOTO && len(op.Data) == 2 {
				delta = int64(int16(binary.BigEndian.Uint16(op.Data)))
			} else if opcode == core.OP_GOTO_W && len(op.Data) == 4 {
				delta = int64(int32(binary.BigEndian.Uint32(op.Data)))
			} else {
				return nil, false
			}
			target := int64(pc) + delta
			if target < 0 || target >= int64(len(code.Code)) {
				return nil, false
			}
			var known bool
			at, known = positions[int(target)]
			if !known || op.IsWide {
				return nil, false
			}
			continue
		}
		if constructorMotionPacketBranch(opcode) || opcode == core.OP_TABLESWITCH || opcode == core.OP_LOOKUPSWITCH || opcode == core.OP_RET || opcode == core.OP_ATHROW || opcode >= core.OP_IRETURN && opcode < core.OP_RETURN || pc <= last {
			return nil, false
		}
		path = append(path, op)
		last = pc
		if opcode == core.OP_RETURN {
			return path, count == len(ops)
		}
		at++
	}
	return nil, false
}
