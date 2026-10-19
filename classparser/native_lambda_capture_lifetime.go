package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// This query consumes an original typed STORE/LOAD result, not a descriptor
// inferred from source text. A later physical slot lifetime need not belong to
// the captured source declaration. The complete exception-aware last-writer
// frontier must still select exactly this STORE. Source publication separately
// reopens the retained declaration and the factory's eager capture snapshot.
//
// Earlier overlapping writes remain outside this projection. In particular,
// this does not turn arbitrary mutable/merged locals into source final binders.
func nativeLambdaLocalCaptureAtFactory(ops []*core.OpCode, flow *nativeEnumParameterFlow, read *nativeEnumLocalRead, factoryPC int, work *workbudget.Budget) bool {
	if flow == nil || flow.code == nil || read == nil || read.storePC < 0 || len(read.storePCs) != 0 || len(ops) == 0 || len(ops) > 8192 || len(ops) != len(flow.byPC) || read.pc >= factoryPC || !nativeProofWork(work, 1) {
		return false
	}
	factory := flow.byPC[factoryPC]
	if factory == nil || factory.Instr == nil || factory.Instr.OpCode != core.OP_INVOKEDYNAMIC {
		return false
	}
	operand := &nativeEnumSelectorProducer{pc: read.pc, opcode: read.opcode, slot: read.slot, result: read.descriptor}
	store, known := flow.reachingStore(operand, work)
	if !known || store != read.storePC {
		return false
	}
	width := 1
	if read.descriptor == "J" || read.descriptor == "D" {
		width = 2
	}
	selected := false
	for _, op := range ops {
		if op == nil || op.Instr == nil || flow.byPC[int(op.CurrentOffset)] != op || !nativeProofWork(work, 1) {
			return false
		}
		access := core.LocalAccessOf(op.Instr.OpCode)
		if !access.Write {
			continue
		}
		slot := core.GetStoreIdx(op)
		if slot < 0 || access.Width < 1 || access.Width > 2 {
			return false
		}
		if slot >= read.slot+width || read.slot >= slot+access.Width {
			continue
		}
		pc := int(op.CurrentOffset)
		if pc == read.storePC {
			if selected || slot != read.slot || access.Width != width || access.Read {
				return false
			}
			selected = true
		} else if pc <= factoryPC {
			return false
		}
	}
	return selected
}
