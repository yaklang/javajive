package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// For an already initialized exact callee, a catch-all receives any throwable
// from the first covering table row. The exception edge starts with the INPUT
// locals and a singleton receiver-free Throwable stack. Typed selection remains
// outside this domain. Constructor handler boundaries retain their own proof.
// JVMS 2.10, 4.9.2 and 6.5.athrow. This graph only adds proof obligations; source
// instructions, exception table order, failure identity and effects do not move.
func constructorReceiverCatchAllHandlerTargets(code *CodeAttribute, ops []*core.OpCode, remaining *int, work *workbudget.Budget) (map[int]int, bool) {
	targets := map[int]int{}
	if code == nil || remaining == nil {
		return nil, false
	}
	if len(code.ExceptionTable) == 0 {
		return targets, true
	}
	*remaining -= len(ops)
	if *remaining < 0 || !nativeProofWork(work, int64(len(ops))) || work != nil && work.CheckAlloc(int64(len(ops))*32) != nil {
		return nil, false
	}
	offsets := map[int]int{}
	for i, op := range ops {
		if op == nil || op.Instr == nil {
			return nil, false
		}
		pc := int(op.CurrentOffset)
		if pc < 0 || pc >= len(code.Code) || int(code.Code[pc]) != op.Instr.OpCode {
			return nil, false
		}
		if _, seen := offsets[pc]; seen {
			return nil, false
		}
		offsets[pc] = i
	}
	for _, h := range code.ExceptionTable {
		*remaining--
		if *remaining < 0 || !nativeProofWork(work, 1) || h == nil || h.CatchType != 0 || h.StartPc >= h.EndPc || int(h.EndPc) > len(code.Code) {
			return nil, false
		}
		_, start := offsets[int(h.StartPc)]
		_, end := offsets[int(h.EndPc)]
		_, handler := offsets[int(h.HandlerPc)]
		if !start || !end && int(h.EndPc) != len(code.Code) || !handler {
			return nil, false
		}
	}
	for i, op := range ops {
		if !core.MayThrowOpcode(op.Instr.OpCode) {
			continue
		}
		for _, h := range code.ExceptionTable {
			*remaining--
			if *remaining < 0 || !nativeProofWork(work, 1) {
				return nil, false
			}
			if op.CurrentOffset >= h.StartPc && op.CurrentOffset < h.EndPc {
				if work != nil && work.CheckAlloc(int64(len(targets)+1)*32) != nil {
					return nil, false
				}
				targets[i] = offsets[int(h.HandlerPc)]
				break
			}
		}
	}
	return targets, true
}

// A throwing allocation initializer makes precisely its uninitialized local
// aliases unusable. Keep all other incoming values, including unrelated NEW
// tokens and the already initialized THIS. The caller bounds the copy before
// invoking this transfer; handler operands always start as one Throwable.
func constructorEffectExceptionLocals(locals []constructorEffectValue, failedAllocation int) []constructorEffectValue {
	incoming := append([]constructorEffectValue(nil), locals...)
	if failedAllocation != 0 {
		for i, v := range incoming {
			if v.allocation == failedAllocation {
				incoming[i] = constructorEffectValue{}
			}
		}
	}
	return incoming
}
