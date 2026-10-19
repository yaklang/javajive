package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Elide only a pure copy of an unchanged original parameter. The factory/result
// snapshot remains at the invokedynamic PC: allocation, linkage, other operand
// evaluations, and their exceptions do not move. A captured object's contents
// may change; this proof concerns the parameter's reference/primitive words.
// Eager factory evidence is separate from the deferred body source certificate.
func (d *Decompiler) inlineUnchangedLambdaParameterOperands(op *OpCode, value values.JavaValue) {
	lambda, ok := value.(*values.CustomValue)
	if d == nil || op == nil || op.Instr == nil || op.Instr.OpCode != OP_INVOKEDYNAMIC || !ok || lambda == nil || !lambda.LambdaParameterNamesRequired() || d.FunctionContext == nil {
		return
	}
	pc, captures, known := lambda.OriginalLambdaFactoryEvaluation()
	if !known || pc != int(op.CurrentOffset) {
		return
	}
	snapshots := d.evaluationSnapshots[op]
	if len(snapshots) > 64 || len(d.Params) > 256 || len(d.opCodes) > 65536 || d.chargeNodeCopies(len(snapshots)+1) != nil || d.Work != nil && (d.Work.CheckAlloc(int64(len(snapshots))*128+64) != nil || d.Work.Charge(workbudget.CounterGraphScans, int64(len(snapshots))*int64(len(d.Params)+len(d.opCodes)+1)) != nil) {
		return
	}
	keep := make([]EvaluationSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		ref := snapshot.Ref
		proved := snapshot.Operand && ref != nil && ref.StackVar == nil && ref.CustomValue == nil
		location, position, original := ref.OriginalPendingDynamicOperand(snapshot.Value)
		proved = proved && original && location == pc && position >= 0 && position < len(captures) && captures[position] == ref
		source, parameter := dynamicCaptureSeed(snapshot.Value).(*values.JavaRef)
		slot, stable := d.unchangedCaptureParameterSlot(source)
		originalSlot, sealed := source.OriginalParameterSlot()
		if !proved || !parameter || !stable || !sealed || slot != originalSlot || snapshot.ExpectedType == nil {
			keep = append(keep, snapshot)
			continue
		}
		actual, typed := values.SourceTypeErasure(source.Type(), d.FunctionContext)
		expected, known := values.SourceTypeErasure(snapshot.ExpectedType, d.FunctionContext)
		if !typed || !known || actual != expected {
			keep = append(keep, snapshot)
			continue
		}
		ref.StackVar = snapshot.Value
		ref.MarkOriginalDynamicOperandInline(pc, snapshot.Value)
	}
	d.evaluationSnapshots[op] = keep
}
