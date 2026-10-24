package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A compiler's bound-reference capture is not necessarily the adjacent LOAD:
// DUP/null-check/POP lies between that LOAD and invokedynamic. Planning may
// identify the original parameter through this exact stack port, without
// moving/removing the check or authorizing any source representation. The final
// source still needs original binding, capture, compiler and consumer receipts.
func nativeBoundAdapterCaptureLoad(object *ClassObject, method, implementation *MemberInfo, code *CodeAttribute, ops []*core.OpCode, factory int, captures []string, work *workbudget.Budget) (int, bool) {
	load, origin, known := nativeBoundAdapterCaptureOrigin(object, method, implementation, code, ops, factory, captures, work)
	return load, known && origin == (ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: core.GetRetrieveIdx(ops[load])})
}

// The eager check and factory must consume the same verified original stack
// identity. Parameter and local binding proofs separately own its source role.
func nativeBoundAdapterCaptureOrigin(object *ClassObject, method, implementation *MemberInfo, code *CodeAttribute, ops []*core.OpCode, factory int, captures []string, work *workbudget.Budget) (int, ssabuild.Origin, bool) {
	var origin ssabuild.Origin
	if object == nil || method == nil || implementation == nil || code == nil || len(captures) != 1 || factory < 4 || factory >= len(ops) || implementation.AccessFlags != 0x100a {
		return 0, origin, false
	}
	reader := NewClassObjectDumper(object)
	reader.Work = work
	shape, known := reader.originalVarargsAdapterShape(implementation)
	if !known || shape.parameters[0] != captures[0] {
		return 0, origin, false
	}
	start := factory - 4
	load, dup, check, pop, site := ops[start], ops[start+1], ops[start+2], ops[start+3], ops[factory]
	if !constructorMotionLoad(load, captures[0]) || dup.Instr.OpCode != core.OP_DUP || len(dup.Data) != 0 || !nativeMemberNullCheck(object, check) || pop.Instr.OpCode != core.OP_POP || len(pop.Data) != 0 || site.Instr.OpCode != core.OP_INVOKEDYNAMIC {
		return 0, origin, false
	}
	slot := core.GetRetrieveIdx(load)
	if slot < 0 {
		return 0, origin, false
	}
	// Equal stack origins do not establish dominance: a branch may supply the
	// same receiver directly to the factory while bypassing the eager check.
	// Require this adjacent packet to be one control/exception region. Entry at
	// the LOAD is safe; entry after it, or a split handler domain, is not.
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return 0, origin, false
	}
	entries, known := nativeMemberLexicalControlEntries(decoder, code, work)
	if !known {
		return 0, origin, false
	}
	for _, pc := range entries {
		if !nativeProofWork(work, 1) || pc > int(load.CurrentOffset) && pc <= int(site.CurrentOffset) {
			return 0, origin, false
		}
	}
	_, frames, known := reader.nativeOriginalMethodSnapshot(method, code)
	if !known {
		return 0, origin, false
	}
	checked, bound := false, false
	for _, record := range frames.Instructions {
		if !nativeProofWork(work, 1) {
			return 0, origin, false
		}
		if record.PC != check.CurrentOffset && record.PC != site.CurrentOffset {
			continue
		}
		if len(record.Uses) != 1 || len(record.Before.Stack) == 0 || record.Before.Stack[len(record.Before.Stack)-1].Kind != frametransfer.Ref {
			return 0, origin, false
		}
		if !checked && !bound {
			origin = record.Uses[0]
		} else if record.Uses[0] != origin {
			return 0, origin, false
		}
		if record.PC == check.CurrentOffset {
			if checked {
				return 0, origin, false
			}
			checked = true
		} else {
			if bound {
				return 0, origin, false
			}
			bound = true
		}
	}
	return start, origin, checked && bound
}
