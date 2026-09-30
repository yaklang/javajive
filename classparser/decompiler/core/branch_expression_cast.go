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
func (d *Decompiler) branchExpressionCastLeaf(ref *values.JavaRef, cast *values.CastExpression, entry, leaf, merge *OpCode) *OpCode {
	if d == nil || d.getenv("JDEC_BRANCH_EXPRESSION_CAST_OFF") != "" {
		return nil
	}
	check := d.branchCastOpcode(ref, cast, entry, leaf, merge)
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
		if len(cur.stackProduced) != 1 {
			return nil
		}
		produced := values.UnpackSoltValue(cur.stackProduced[0])
		if produced == nil {
			return nil
		}
		// Operands are listed in pop order: last argument first, receiver last.
		var operands []values.JavaValue
		op := cur.Instr.OpCode
		switch op {
		case OP_GETFIELD:
			field, ok := produced.(*values.RefMember)
			if !ok || field == nil || field.Object == nil {
				return nil
			}
			operands = []values.JavaValue{field.Object}
		case OP_GETSTATIC:
			if field, ok := produced.(*values.JavaClassMember); !ok || field == nil {
				return nil
			}
		case OP_INVOKEVIRTUAL, OP_INVOKEINTERFACE, OP_INVOKESTATIC:
			call, ok := produced.(*values.FunctionCallExpression)
			if !ok || call == nil || d.invokeFuncCall[cur] != call || call.OriginPC != int(cur.CurrentOffset) || call.Descriptor == "" ||
				call.IsStatic != (op == OP_INVOKESTATIC) {
				return nil
			}
			method, err := types.ParseMethodDescriptor(call.Descriptor)
			if err != nil || method.FunctionType() == nil || method.FunctionType().ReturnType == nil ||
				len(method.FunctionType().ParamTypes) != len(call.Arguments) {
				return nil
			}
			if p, ok := method.FunctionType().ReturnType.RawType().(*types.JavaPrimer); ok && p.Name == types.JavaVoid {
				return nil
			}
			for i := len(call.Arguments) - 1; i >= 0; i-- {
				operands = append(operands, call.Arguments[i])
			}
			if !call.IsStatic {
				if call.Object == nil {
					return nil
				}
				operands = append(operands, call.Object)
			}
		case OP_CHECKCAST:
			inner, ok := produced.(*values.CastExpression)
			if !ok || inner == nil || !d.inlineCheckcast[cur] || inner.OriginPC != int(cur.CurrentOffset) ||
				values.UnpackSoltValue(d.checkcastInnerArg[cur]) != values.UnpackSoltValue(inner.Value) {
				return nil
			}
			operands = []values.JavaValue{inner.Value}
		default:
			access := LocalAccessOf(op)
			if !(access.Read && !access.Write && op != OP_RET) &&
				op != OP_ACONST_NULL && !(op >= OP_ICONST_M1 && op <= OP_DCONST_1) &&
				op != OP_BIPUSH && op != OP_SIPUSH && op != OP_LDC && op != OP_LDC_W && op != OP_LDC2_W {
				return nil
			}
			// A load/constant cannot stand in for a hidden defining expression.
			switch produced.(type) {
			case *values.JavaRef, *values.JavaLiteral, *values.JavaClassValue:
			default:
				return nil
			}
		}
		if len(cur.stackConsumed) != len(operands) || len(stack) < len(operands) {
			return nil
		}
		for i, operand := range operands {
			value := values.UnpackSoltValue(operand)
			if value == nil || value != values.UnpackSoltValue(cur.stackConsumed[i]) || value != stack[len(stack)-1-i] {
				return nil
			}
		}
		stack = append(stack[:len(stack)-len(operands)], produced)
	}
	if len(stack) != 1 || stack[0] != values.UnpackSoltValue(cast.Value) {
		return nil
	}
	return check
}
