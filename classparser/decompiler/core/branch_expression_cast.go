package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// branchExpressionCastLeaf proves that the complete expression before a
// branch-local CHECKCAST is exactly the branch's private bytecode prefix.
// Field reads and calls are effectful; admitting them requires matching their
// AST operands to every consumed stack value in JVM order. Nothing is skipped,
// repeated or moved outside the selected arm, including volatile reads, class
// initialization, argument evaluation, exceptions and the final cast.
//
// This small stack proof owns only values produced after entry. It never
// follows JavaRef.Val: a materialized local is a snapshot, not its old defining
// expression. Stores, DUP/POP, void calls, constructors, forks and handler
// changes are rejected. Existing single-use/unique-producer checks still apply.
func (d *Decompiler) branchExpressionCastLeaf(ref *values.JavaRef, cast *values.CastExpression, entry, leaf, merge *OpCode, selection ...*OpCode) *OpCode {
	if d == nil || d.getenv("JDEC_BRANCH_EXPRESSION_CAST_OFF") != "" {
		return nil
	}
	check := d.branchCastOpcode(ref, cast, entry, leaf, merge, selection...)
	if check == nil || check.IsCustom || check.IsCatch || check.IsTryCatchParent ||
		len(check.stackConsumed) != 1 || values.UnpackSoltValue(check.stackConsumed[0]) != values.UnpackSoltValue(cast.Value) ||
		values.UnpackSoltValue(d.checkcastInnerArg[check]) != values.UnpackSoltValue(cast.Value) {
		return nil
	}
	stack := []values.JavaValue{}
	seen := map[*OpCode]bool{}
	for cur := entry; cur != check; cur = cur.Target[0] {
		if cur == nil || seen[cur] || len(seen) >= 256 || cur.Instr == nil || cur.IsCustom || cur.IsCatch || cur.IsTryCatchParent ||
			len(cur.Target) != 1 || cur.Target[0] == nil || cur.Target[0].CurrentOffset <= cur.CurrentOffset ||
			len(cur.Target[0].Source) != 1 || cur.Target[0].Source[0] != cur ||
			!sameHandlerCoverage(d.handlersAt(cur), d.handlersAt(check)) {
			return nil
		}
		seen[cur] = true
		if cur.Instr.OpCode == OP_NOP {
			if len(cur.stackConsumed) != 0 || len(cur.stackProduced) != 0 {
				return nil
			}
			continue
		}
		var ok bool
		stack, ok = d.branchExpressionStackStep(cur, stack)
		if !ok {
			return nil
		}
	}
	if len(stack) != 1 || stack[0] != values.UnpackSoltValue(cast.Value) {
		return nil
	}
	return check
}

// branchExpressionStackStep matches one value-producing AST node to the JVM's
// decoded pop order. Path isolation and handler coverage belong to the caller.
// It does not admit DUP, stores, discarded effects or opaque expressions.
func (d *Decompiler) branchExpressionStackStep(cur *OpCode, stack []values.JavaValue) ([]values.JavaValue, bool) {
	if d == nil || cur == nil || cur.Instr == nil {
		return nil, false
	}
	if len(cur.stackProduced) != 1 {
		return nil, false
	}
	produced := values.UnpackSoltValue(cur.stackProduced[0])
	if produced == nil {
		return nil, false
	}
	// Operands are listed in pop order: last argument first, receiver last.
	var operands []values.JavaValue
	op := cur.Instr.OpCode
	switch op {
	case OP_GETFIELD:
		field, ok := produced.(*values.RefMember)
		if !ok || field == nil || field.Object == nil {
			return nil, false
		}
		operands = []values.JavaValue{field.Object}
	case OP_GETSTATIC:
		if field, ok := produced.(*values.JavaClassMember); !ok || field == nil {
			return nil, false
		}
	case OP_AALOAD, OP_IALOAD, OP_BALOAD, OP_CALOAD, OP_SALOAD, OP_LALOAD, OP_FALOAD, OP_DALOAD:
		access, ok := produced.(*values.JavaArrayMember)
		if !ok || access == nil || access.Object == nil || access.Index == nil {
			return nil, false
		}
		operands = []values.JavaValue{access.Index, access.Object}
	case OP_ARRAYLENGTH:
		length, ok := produced.(*values.ArrayLengthExpression)
		if !ok || length == nil || length.Array == nil || !length.HasOriginPC || length.OriginPC != int(cur.CurrentOffset) {
			return nil, false
		}
		operands = []values.JavaValue{length.Array}
	case OP_ANEWARRAY, OP_NEWARRAY, OP_MULTIANEWARRAY:
		array, ok := produced.(*values.NewExpression)
		if !ok || array == nil || !array.IsArray() || !array.HasOriginPC || array.OriginPC != int(cur.CurrentOffset) || len(array.Initializer) != 0 || len(array.Length) == 0 {
			return nil, false
		}
		if op == OP_MULTIANEWARRAY {
			if len(cur.Data) != 3 || int(cur.Data[2]) != len(array.Length) || len(array.Length) > array.Type().ArrayDim() {
				return nil, false
			}
		} else if len(array.Length) != 1 {
			return nil, false
		}
		// A sized allocation is a value-producing operation, with dimensions
		// evaluated before allocation in source order. No DUP/initializer store
		// is consumed here: those require a separate ownership proof.
		for i := len(array.Length) - 1; i >= 0; i-- {
			operands = append(operands, array.Length[i])
		}
	case OP_INVOKEVIRTUAL, OP_INVOKEINTERFACE, OP_INVOKESTATIC:
		call, ok := produced.(*values.FunctionCallExpression)
		if !ok || call == nil || d.invokeFuncCall[cur] != call || call.OriginPC != int(cur.CurrentOffset) || call.Descriptor == "" ||
			call.IsStatic != (op == OP_INVOKESTATIC) {
			return nil, false
		}
		method, err := types.ParseMethodDescriptor(call.Descriptor)
		if err != nil || method.FunctionType() == nil || method.FunctionType().ReturnType == nil ||
			len(method.FunctionType().ParamTypes) != len(call.Arguments) {
			return nil, false
		}
		if p, ok := method.FunctionType().ReturnType.RawType().(*types.JavaPrimer); ok && p.Name == types.JavaVoid {
			return nil, false
		}
		for i := len(call.Arguments) - 1; i >= 0; i-- {
			operands = append(operands, call.Arguments[i])
		}
		if !call.IsStatic {
			if call.Object == nil {
				return nil, false
			}
			operands = append(operands, call.Object)
		}
	case OP_CHECKCAST:
		inner, ok := produced.(*values.CastExpression)
		if !ok || inner == nil || !d.inlineCheckcast[cur] || inner.OriginPC != int(cur.CurrentOffset) ||
			values.UnpackSoltValue(d.checkcastInnerArg[cur]) != values.UnpackSoltValue(inner.Value) {
			return nil, false
		}
		operands = []values.JavaValue{inner.Value}
	default:
		access := LocalAccessOf(op)
		if !(access.Read && !access.Write && op != OP_RET) &&
			op != OP_ACONST_NULL && !(op >= OP_ICONST_M1 && op <= OP_DCONST_1) &&
			op != OP_BIPUSH && op != OP_SIPUSH && op != OP_LDC && op != OP_LDC_W && op != OP_LDC2_W {
			return nil, false
		}
		// A load/constant cannot stand in for a hidden defining expression.
		switch produced.(type) {
		case *values.JavaRef, *values.JavaLiteral, *values.JavaClassValue:
		default:
			return nil, false
		}
	}
	if len(cur.stackConsumed) != len(operands) || len(stack) < len(operands) {
		return nil, false
	}
	for i, operand := range operands {
		value := values.UnpackSoltValue(operand)
		if value == nil || value != values.UnpackSoltValue(cur.stackConsumed[i]) || value != stack[len(stack)-1-i] {
			return nil, false
		}
	}
	return append(stack[:len(stack)-len(operands)], produced), true
}
