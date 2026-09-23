package core

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// EvaluationSnapshot materializes one already-evaluated bytecode operand. The
// original SlotValue remains on the RHS for phase-two reaching-def rebinding;
// the reconstructed concat/lambda sees only the dedicated immutable temporary.
type EvaluationSnapshot struct {
	Ref      *values.JavaRef
	Value    values.JavaValue
	OriginPC int
}

func (d *Decompiler) snapshotDynamicOperands(op *OpCode, sim StackSimulation, args []values.JavaValue, parameters []types.JavaType) ([]values.JavaValue, error) {
	if err := d.chargeNodeCopies(len(args) + 1); err != nil {
		return nil, err
	}
	if d.evaluationSnapshots == nil {
		d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{}
	}
	result := make([]values.JavaValue, len(args))
	// JVM stack-pop order is reversed; source evaluation order must be restored.
	for i := len(args) - 1; i >= 0; i-- {
		ref := sim.NewVar(args[i])
		ref.ResetVarType(ref.Type().Copy())
		if len(parameters) == len(args) {
			want := parameters[len(args)-1-i]
			if _, ok := want.RawType().(*types.JavaPrimer); ok {
				ref.ResetVarType(want.Copy())
			}
		}
		d.disFoldRef = append(d.disFoldRef, ref)
		d.evaluationSnapshots[op] = append(d.evaluationSnapshots[op], EvaluationSnapshot{Ref: ref, Value: args[i], OriginPC: int(op.CurrentOffset)})
		result[i] = ref
	}
	return result, nil
}
func (d *Decompiler) snapshotDynamicResult(op *OpCode, sim StackSimulation, value values.JavaValue) values.JavaValue {
	if d.canInlineImmediateMethodRef(op, value) {
		return value
	}
	ref := sim.NewVar(value)
	ref.ResetVarType(ref.Type().Copy())
	d.disFoldRef = append(d.disFoldRef, ref)
	d.evaluationSnapshots[op] = append(d.evaluationSnapshots[op], EvaluationSnapshot{Ref: ref, Value: value, OriginPC: int(op.CurrentOffset)})
	return ref
}

// canInlineImmediateMethodRef keeps a method reference as a poly expression
// only when the next instruction consumes it as the final argument of a
// same-class invocation whose functional-interface parameter exactly matches
// the invokedynamic result type. The declaration being rebuilt in the same
// compilation unit preserves its generic target signature; external calls can
// cross raw receiver/erasure boundaries, so their typed temporary stays.
func (d *Decompiler) canInlineImmediateMethodRef(op *OpCode, value values.JavaValue) bool {
	ref, ok := value.(*values.CustomValue)
	if !ok || ref == nil || !ref.IsMethodRef || d == nil || d.constantPoolGetter == nil ||
		d.FunctionContext == nil || d.FunctionContext.ClassName == "" ||
		op == nil || op.Instr == nil || op.Instr.OpCode != OP_INVOKEDYNAMIC {
		return false
	}
	if len(op.Target) != 1 {
		return false
	}
	next := op.Target[0]
	if next == nil || next.Instr == nil || len(next.Source) != 1 || next.Source[0] != op ||
		int(next.CurrentOffset) != int(op.CurrentOffset)+1+len(op.Data) {
		return false
	}
	if !sameIntSlice(d.handlersAt(op), d.handlersAt(next)) || len(next.Data) < 2 {
		return false
	}
	switch next.Instr.OpCode {
	case OP_INVOKESTATIC, OP_INVOKESPECIAL, OP_INVOKEVIRTUAL, OP_INVOKEINTERFACE:
	default:
		return false
	}
	member, ok := d.constantPoolGetter(int(Convert2bytesToInt(next.Data))).(*values.JavaClassMember)
	if !ok || member == nil || member.JavaType == nil || member.JavaType.FunctionType() == nil {
		return false
	}
	params := member.JavaType.FunctionType().ParamTypes
	if len(params) == 0 || params[len(params)-1] == nil || ref.Type() == nil {
		return false
	}
	ownerType, ownerOK := params[len(params)-1].RawType().(*types.JavaClass)
	refType, refOK := ref.Type().RawType().(*types.JavaClass)
	if !ownerOK || ownerType == nil || !refOK || refType == nil ||
		normalizeJavaClassName(member.Name) != normalizeJavaClassName(d.FunctionContext.ClassName) ||
		normalizeJavaClassName(ownerType.Name) != normalizeJavaClassName(refType.Name) {
		return false
	}
	return true
}

func normalizeJavaClassName(name string) string {
	return strings.ReplaceAll(name, ".", "/")
}
