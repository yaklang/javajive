package core

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// canInlineImmediateZeroArgCheckcast keeps a checked value in its use expression when the
// immediately following instruction invokes a zero-argument instance method on that exact type.
// This preserves branch-local evaluation across a CFG merge (for example, a Boolean checkcast in
// one arm of a ternary) without inventing a local whose definition is scoped to only one arm.
func (d *Decompiler) canInlineImmediateZeroArgCheckcast(op *OpCode, castType types.JavaType) bool {
	if d == nil || d.getenv("JDEC_CHECKCAST_IMMEDIATE_INVOKE_OFF") != "" || op == nil || len(op.Target) != 1 {
		return false
	}
	consumer := op.Target[0]
	if consumer == nil || consumer.IsCustom || consumer.Instr == nil {
		return false
	}
	switch consumer.Instr.OpCode {
	case OP_INVOKEVIRTUAL, OP_INVOKEINTERFACE:
	default:
		return false
	}
	if d.constantPoolGetter == nil {
		return false
	}
	member, ok := d.constantPoolGetter(int(Convert2bytesToInt(consumer.Data))).(*values.JavaClassMember)
	if !ok || member == nil || member.JavaType == nil || member.JavaType.FunctionType() == nil || len(member.JavaType.FunctionType().ParamTypes) != 0 {
		return false
	}
	castOwner, ok := types.ClassFQNOf(castType)
	if !ok || castOwner == "" || strings.ReplaceAll(member.Name, "/", ".") != castOwner {
		return false
	}
	return sameIntSlice(d.handlersAt(op), d.handlersAt(consumer))
}

func sameIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
