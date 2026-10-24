package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// javac derives a lambda implementation's capture parameters from the source
// binders' declared types. A solved local web may have a narrower producer type
// than the original factory descriptor. Preserve that descriptor on the actual
// eager copy, without changing the source local, operand or evaluation point.
// Bound method references have an additional lookup domain and do not acquire
// this lambda-body declaration rule.
func (d *Decompiler) markLambdaCaptureDescriptorSnapshots(op *OpCode, value values.JavaValue) {
	lambda, ok := value.(*values.CustomValue)
	if d == nil || op == nil || op.Instr == nil || op.Instr.OpCode != OP_INVOKEDYNAMIC || !ok || lambda == nil || lambda.IsMethodRef {
		return
	}
	pc, captures, known := lambda.OriginalLambdaFactoryEvaluation()
	if !known || pc != int(op.CurrentOffset) || len(d.evaluationSnapshots[op]) > 64 || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, int64(len(captures)+1)) != nil {
		return
	}
	for i := range d.evaluationSnapshots[op] {
		snapshot := &d.evaluationSnapshots[op][i]
		if !snapshot.Operand || snapshot.Ref == nil || snapshot.ExpectedType == nil || snapshot.OriginPC != pc {
			continue
		}
		location, position, original := snapshot.Ref.OriginalPendingDynamicOperand(snapshot.Value)
		if original && location == pc && position >= 0 && position < len(captures) && captures[position] == snapshot.Ref {
			snapshot.lambdaCapture = true
		}
	}
}

func (d *Decompiler) lambdaCaptureSnapshotType(snapshot EvaluationSnapshot, source types.JavaType) (types.JavaType, bool) {
	if !snapshot.lambdaCapture || snapshot.ExpectedType == nil || source == nil || d == nil || d.FunctionContext == nil {
		return nil, false
	}
	actual, typed := values.SourceTypeErasure(source, d.FunctionContext)
	expected, known := values.SourceTypeErasure(snapshot.ExpectedType, d.FunctionContext)
	if !typed || !known || !callbinding.Reference(actual) || !callbinding.Reference(expected) || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, int64(len(actual)+len(expected))+1) != nil {
		return nil, false
	}
	if actual == expected {
		// Preserve a solved generic declaration with the same physical erasure.
		return source, true
	}
	if !callbinding.Assignable(actual, expected, d.FunctionContext.InvocationMetadata) {
		return nil, false
	}
	return snapshot.ExpectedType, true
}
