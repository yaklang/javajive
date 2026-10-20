package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A compiler's bound-reference capture is not necessarily the adjacent LOAD:
// DUP/null-check/POP lies between that LOAD and invokedynamic. Planning may
// identify the original parameter through this exact stack port, without
// moving/removing the check or authorizing any source representation. The final
// source still needs original binding, capture, compiler and consumer receipts.
func nativeBoundAdapterCaptureLoad(object *ClassObject, method, implementation *MemberInfo, code *CodeAttribute, ops []*core.OpCode, factory int, captures []string, work *workbudget.Budget) (int, bool) {
	if object == nil || method == nil || implementation == nil || code == nil || len(captures) != 1 || factory < 4 || factory >= len(ops) || implementation.AccessFlags != 0x100a {
		return 0, false
	}
	reader := NewClassObjectDumper(object)
	reader.Work = work
	shape, known := reader.originalVarargsAdapterShape(implementation)
	if !known || shape.parameters[0] != captures[0] {
		return 0, false
	}
	start := factory - 4
	load, dup, check, pop, site := ops[start], ops[start+1], ops[start+2], ops[start+3], ops[factory]
	if !constructorMotionLoad(load, captures[0]) || dup.Instr.OpCode != core.OP_DUP || len(dup.Data) != 0 || !nativeMemberNullCheck(object, check) || pop.Instr.OpCode != core.OP_POP || len(pop.Data) != 0 || site.Instr.OpCode != core.OP_INVOKEDYNAMIC {
		return 0, false
	}
	slot := core.GetRetrieveIdx(load)
	if slot < 0 {
		return 0, false
	}
	_, frames, known := reader.nativeOriginalMethodSnapshot(method, code)
	if !known {
		return 0, false
	}
	origin := ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: slot}
	checked, bound := false, false
	for _, record := range frames.Instructions {
		if !nativeProofWork(work, 1) {
			return 0, false
		}
		if record.PC != check.CurrentOffset && record.PC != site.CurrentOffset {
			continue
		}
		if len(record.Uses) != 1 || record.Uses[0] != origin || len(record.Before.Stack) == 0 || record.Before.Stack[len(record.Before.Stack)-1].Kind != frametransfer.Ref {
			return 0, false
		}
		if record.PC == check.CurrentOffset {
			if checked {
				return 0, false
			}
			checked = true
		} else {
			if bound {
				return 0, false
			}
			bound = true
		}
	}
	return start, checked && bound
}
