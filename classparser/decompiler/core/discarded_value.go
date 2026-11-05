package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// materializeDiscardedValue preserves the evaluation of a value that the JVM
// discards with POP/POP2 but that Java cannot express as a bare expression
// statement. A discarded static field read may trigger class initialization, a
// discarded array/cast/arithmetic expression may throw, and neither
// `Owner.FIELD;` nor `a[i];` is a legal statement (JLS 14.8). Evaluate it in a
// boolean sink (`expr == null` / `expr == 0`): the comparison is always a legal
// initializer, the original instruction remains at its edge, and class
// initialization and exceptional exits are retained.
//
// Function calls and class-instance creation are already legal statement
// expressions and are left to the existing emission path. Pure locals,
// literals, class literals and this-rooted field reads remain droppable via
// keepDiscardedStackValue.
func (d *Decompiler) materializeDiscardedValue(opcode *OpCode, value values.JavaValue) statements.Statement {
	if d == nil || opcode == nil || value == nil {
		return nil
	}
	switch values.UnpackSoltValue(value).(type) {
	case *values.FunctionCallExpression, *values.NewExpression:
		return nil
	}
	if isSideEffectFreeDiscard(value) {
		return nil
	}
	typ := value.Type()
	if typ == nil {
		return nil
	}
	var zero values.JavaValue = values.JavaNull
	if primer, ok := typ.RawType().(*types.JavaPrimer); ok {
		switch primer.Name {
		case types.JavaString:
			zero = values.JavaNull
		case types.JavaBoolean:
			zero = values.NewJavaLiteral(false, typ.Copy())
		case types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger:
			zero = values.NewJavaLiteral(0, typ.Copy())
		case types.JavaLong:
			zero = values.NewJavaLiteral(int64(0), typ.Copy())
		case types.JavaFloat:
			zero = values.NewJavaLiteral(float32(0), typ.Copy())
		case types.JavaDouble:
			zero = values.NewJavaLiteral(float64(0), typ.Copy())
		default:
			// void and other non-value categories cannot reach POP.
			return nil
		}
	}
	sim := d.opcodeToSimulateStack[opcode]
	if sim == nil {
		return nil
	}
	boolean := types.NewJavaPrimer(types.JavaBoolean)
	sink := sim.NewVar(values.NewJavaLiteral(false, boolean))
	sink.ResetVarType(boolean)
	comparison := values.NewBinaryExpression(value, zero, values.EQ, boolean)
	return statements.NewAssignStatement(sink, comparison, true)
}
