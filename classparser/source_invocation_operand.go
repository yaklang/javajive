package javaclassparser

import (
	"encoding/binary"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Invocation operands are not interchangeable with ldc/field operands.
// invokeinterface carries a two-byte pool index, a receiver-inclusive slot
// count, and a reserved zero byte. Validate the complete instruction before
// using its reference in a closed-consumer proof.
func sourceInvocationCPIndex(object *ClassObject, op *core.OpCode, work *workbudget.Budget) (uint16, bool) {
	if op == nil || op.Instr == nil || op.IsWide {
		return 0, false
	}
	opcode := op.Instr.OpCode
	switch opcode {
	case core.OP_INVOKESTATIC, core.OP_INVOKESPECIAL, core.OP_INVOKEVIRTUAL, core.OP_INVOKEINTERFACE:
	default:
		return 0, false
	}
	member := constructorMotionMember(object, op, opcode)
	if member == nil || !nativeProofWork(work, int64(len(member.Description))+1) || member.Member == "<clinit>" || member.Member == "<init>" && opcode != core.OP_INVOKESPECIAL {
		return 0, false
	}
	if len(member.Description) > 65535 || work.CheckAlloc(int64(len(member.Description))*16) != nil {
		return 0, false
	}
	args, result, err := callbinding.Descriptor(member.Description)
	if err != nil || member.Member == "<init>" && result != "V" {
		return 0, false
	}
	slots := 0
	if opcode != core.OP_INVOKESTATIC {
		slots++
	}
	for _, arg := range args {
		slots++
		if arg == "J" || arg == "D" {
			slots++
		}
	}
	if slots > 255 || opcode == core.OP_INVOKEINTERFACE && int(op.Data[2]) != slots {
		return 0, false
	}
	return binary.BigEndian.Uint16(op.Data[:2]), true
}
