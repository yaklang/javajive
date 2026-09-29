package core

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// CHECKCAST consumes one reference and produces one reference. If the next
// instruction consumes it as the last method argument, it has no independently
// observable local identity. Keep that cast in the argument tree from the start:
// materializing it as a statement can strand or hoist its definition when a
// surrounding branch becomes a ternary. Earlier operands stay earlier; the
// cast and invocation remain under the same exception handlers.
func (d *Decompiler) canInlineImmediateCheckcastArgument(op *OpCode) bool {
	if d == nil || op == nil || op.Instr == nil || op.Instr.OpCode != OP_CHECKCAST || len(op.Target) != 1 || d.constantPoolGetter == nil {
		return false
	}
	consumer := op.Target[0]
	if consumer == nil || consumer.IsCustom || consumer.Instr == nil || len(consumer.Source) != 1 || consumer.Source[0] != op || !sameHandlerCoverage(d.handlersAt(op), d.handlersAt(consumer)) {
		return false
	}
	switch consumer.Instr.OpCode {
	case OP_INVOKEVIRTUAL, OP_INVOKEINTERFACE, OP_INVOKESTATIC, OP_INVOKESPECIAL:
	default:
		return false
	}
	if len(consumer.Data) < 2 {
		return false
	}
	member, ok := d.constantPoolGetter(int(Convert2bytesToInt(consumer.Data))).(*values.JavaClassMember)
	if !ok || member == nil || member.Member == "<init>" || member.JavaType == nil {
		return false
	}
	method := member.JavaType.FunctionType()
	if method == nil || len(method.ParamTypes) == 0 {
		return false
	}
	last := method.ParamTypes[len(method.ParamTypes)-1]
	if last == nil {
		return false
	}
	_, primitive := last.RawType().(*types.JavaPrimer)
	return !primitive
}

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
