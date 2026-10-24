package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Swapping the two complete prefix packets preserves their total byte length.
// Every suffix edge and protected interval must stay beyond that empty-stack
// cut. No exception handler can observe the intermediate uninitialized receiver,
// and no back edge can repeat capture stores or SUPER. Suffix Code stays exact.
func constructorLoweringSuffixClosed(object *ClassObject, code *CodeAttribute, ops []*core.OpCode, start int, work *workbudget.Budget) bool {
	if object == nil || code == nil || !nativeProofWork(work, int64(len(ops))+int64(len(code.ExceptionTable))*8+1) || work.CheckAlloc(int64(len(ops)+1)*64) != nil {
		return false
	}
	withoutHandlers := *code
	withoutHandlers.ExceptionTable = nil
	if !enumArraySuffixClosed(&withoutHandlers, ops, start) {
		return false
	}
	positions := map[int]bool{len(code.Code): true}
	for _, op := range ops {
		positions[int(op.CurrentOffset)] = true
	}
	for _, handler := range code.ExceptionTable {
		if handler == nil || int(handler.StartPc) < start || handler.StartPc >= handler.EndPc || !positions[int(handler.StartPc)] || !positions[int(handler.EndPc)] || int(handler.HandlerPc) < start || int(handler.HandlerPc) >= len(code.Code) || !positions[int(handler.HandlerPc)] {
			return false
		}
		if handler.CatchType != 0 {
			if _, ok := sourceBridgeClassName(object, handler.CatchType); !ok {
				return false
			}
		}
	}
	return true
}
