package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Handler domains are instruction ranges, not a property of an entire method.
// A later constructor-body handler cannot cover a pre-initialization operand
// or capture store. Keep every original handler wholly beyond the complete
// invokespecial instruction; malformed ranges and backward handler entries
// supply no domain proof. The caller separately binds this instruction to the
// exact original THIS/SUPER initialization and preserves the later body.
func constructorPreludeOutsideHandlers(code *CodeAttribute, initializationPC int, decoder *core.Decompiler, work *workbudget.Budget) bool {
	if code == nil || decoder == nil || initializationPC < 0 || initializationPC > len(code.Code)-3 || code.Code[initializationPC] != core.OP_INVOKESPECIAL {
		return false
	}
	if !nativeProofWork(work, int64(len(code.ExceptionTable))+1) {
		return false
	}
	initialization := decoder.OpcodeByPC(uint16(initializationPC))
	if initialization == nil || initialization.Instr == nil || initialization.Instr.OpCode != core.OP_INVOKESPECIAL || len(initialization.Data) != 2 || initialization.Data[0] != code.Code[initializationPC+1] || initialization.Data[1] != code.Code[initializationPC+2] {
		return false
	}
	end := initializationPC + 3
	for _, handler := range code.ExceptionTable {
		if handler == nil || int(handler.StartPc) < end || handler.StartPc >= handler.EndPc || int(handler.EndPc) > len(code.Code) || int(handler.HandlerPc) < end || int(handler.HandlerPc) >= len(code.Code) {
			return false
		}
		// Interval order is insufficient if a boundary lands inside an
		// instruction operand. Reuse the caller's complete original decoder,
		// so domain admission requires no second parse or unbounded scan.
		if decoder.OpcodeByPC(handler.StartPc) == nil || decoder.OpcodeByPC(handler.HandlerPc) == nil || int(handler.EndPc) != len(code.Code) && decoder.OpcodeByPC(handler.EndPc) == nil {
			return false
		}
	}
	return true
}
