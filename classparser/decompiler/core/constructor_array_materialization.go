package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

type delegationArrayMaterialization struct {
	node  *Node
	array *values.NewExpression
	ref   *values.JavaRef
}

// A completed later initializer remains an argument in its original position.
// Its source definition must be the exact original DUP materialization, with
// sequential stores, one invocation use, no publication or other aliases.
func (d *Decompiler) privateDelegationCompletedArray(c delegationArrayMaterialization, call *values.FunctionCallExpression, invoke *OpCode, argument int, origins map[int]*OpCode) bool {
	array, ref := c.array, c.ref
	if array == nil || ref == nil || c.node == nil || call == nil || call.FuncType == nil || invoke == nil || argument < 0 || argument >= len(call.Arguments) || argument >= len(call.FuncType.ParamTypes) || !sameExactArrayType(array.Type(), call.FuncType.ParamTypes[argument]) {
		return false
	}
	method, err := types.ParseMethodDescriptor(call.Descriptor)
	if err != nil || method.FunctionType() == nil || argument >= len(method.FunctionType().ParamTypes) || !sameExactArrayType(array.Type(), method.FunctionType().ParamTypes[argument]) {
		return false
	}
	return d.privateCompletedArrayAtUse(array, ref, origins[c.node.Id], invoke, len(call.Arguments)-1-argument, map[*values.NewExpression]bool{}, 0)
}

func (d *Decompiler) privateCompletedArrayAtUse(array *values.NewExpression, ref *values.JavaRef, dup, sink *OpCode, useIndex int, active map[*values.NewExpression]bool, depth int) bool {
	if array == nil || ref == nil || ref.IsParam || ref.IsThis || sink == nil || active[array] || depth >= 16 || !array.HasOriginPC || !array.HasEvaluationEndPC || len(array.Length) != 1 || len(array.Initializer) == 0 || len(array.Initializer) > 128 || array.EvaluationEndPC >= int(sink.CurrentOffset) || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
		return false
	}
	active[array] = true
	defer delete(active, array)
	length, known := values.UnpackSoltValue(array.Length[0]).(*values.JavaLiteral)
	if !known || length.Data != len(array.Initializer) {
		return false
	}
	allocation := d.opcodeAtOffset(array.OriginPC)
	if allocation == nil || allocation.Instr == nil || allocation.Instr.OpCode != OP_ANEWARRAY && allocation.Instr.OpCode != OP_NEWARRAY || len(allocation.Target) != 1 || allocation.Target[0] != dup || dup == nil || dup.Instr == nil || dup.Instr.OpCode != OP_DUP || len(dup.stackConsumed) != 1 || values.UnpackSoltValue(dup.stackConsumed[0]) != array || len(dup.stackProduced) != 2 || !delegationArraySameRef(dup.stackProduced[0], ref) || !delegationArraySameRef(dup.stackProduced[1], ref) {
		return false
	}
	stores, uses := 0, 0
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		for index, value := range op.stackConsumed {
			if !delegationArraySameRef(value, ref) {
				continue
			}
			if op == sink && index == useIndex {
				uses++
				continue
			}
			if int(op.CurrentOffset) <= array.OriginPC || int(op.CurrentOffset) > array.EvaluationEndPC {
				return false
			}
			if op.Instr.OpCode == OP_DUP {
				continue
			}
			if !isArrayElementStore(op.Instr.OpCode) || index != 2 || len(op.stackConsumed) != 3 || stores >= len(array.Initializer) {
				return false
			}
			original, kept := values.UnpackSoltValue(op.stackConsumed[0]), values.UnpackSoltValue(array.Initializer[stores])
			if original != kept {
				// Nested initializer folding replaces one original child DUP
				// reference with its completed array. Re-prove that child's
				// one-use stack ownership at this exact parent element store.
				child, childKnown := kept.(*values.NewExpression)
				childRef, refKnown := original.(*values.JavaRef)
				if !childKnown || !refKnown || !child.IsArray() || !child.HasOriginPC {
					return false
				}
				create := d.opcodeAtOffset(child.OriginPC)
				if create == nil || len(create.Target) != 1 || !d.privateCompletedArrayAtUse(child, childRef, create.Target[0], op, 0, active, depth+1) {
					return false
				}
			}
			position, known := values.UnpackSoltValue(op.stackConsumed[1]).(*values.JavaLiteral)
			if !known || position.Data != stores {
				return false
			}
			stores++
			if stores == len(array.Initializer) && int(op.CurrentOffset) != array.EvaluationEndPC {
				return false
			}
		}
	}
	return uses == 1 && stores == len(array.Initializer)
}
