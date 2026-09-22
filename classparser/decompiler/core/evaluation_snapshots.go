package core

import (
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
	ref := sim.NewVar(value)
	ref.ResetVarType(ref.Type().Copy())
	d.disFoldRef = append(d.disFoldRef, ref)
	d.evaluationSnapshots[op] = append(d.evaluationSnapshots[op], EvaluationSnapshot{Ref: ref, Value: value, OriginPC: int(op.CurrentOffset)})
	return ref
}
