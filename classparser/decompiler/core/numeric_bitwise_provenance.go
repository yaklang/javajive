package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"reflect"
)

// IAND/IOR/IXOR alone do not distinguish an integer expression from a boolean
// expression. This only recognizes the original operation packet. The caller
// must prove every operand's reaching definitions and a numeric root before
// using its result as an integer-domain witness.
func originalIntBitwiseResult(op *OpCode) *values.JavaExpression {
	if op == nil || op.Instr == nil || op.IsCustom || op.IsWide || len(op.Data) != 0 || len(op.stackConsumed) != 2 || len(op.stackProduced) != 1 {
		return nil
	}
	operator := ""
	switch op.Instr.OpCode {
	case OP_IAND:
		operator = values.AND
	case OP_IOR:
		operator = values.OR
	case OP_IXOR:
		operator = values.XOR
	default:
		return nil
	}
	x, ok := safeSeedOperand(op.stackProduced[0]).(*values.JavaExpression)
	if !ok || x == nil || x.Op != operator || len(x.Values) != 2 {
		return nil
	}
	for i, v := range x.Values {
		a, b := values.OriginalStackLifetimeUse(v), values.OriginalStackLifetimeUse(op.stackConsumed[1-i])
		if a == nil || b == nil || a != b || reflect.ValueOf(a).Kind() == reflect.Ptr && reflect.ValueOf(a).IsNil() {
			return nil
		}
	}
	return x
}
