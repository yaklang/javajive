package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Elide only a pure copy of an unchanged original parameter or initialized
// implicit receiver. The factory/result
// snapshot remains at the invokedynamic PC: allocation, linkage, other operand
// evaluations, and their exceptions do not move. A captured object's contents
// may change; this proof concerns the parameter's reference/primitive words.
// Eager factory evidence is separate from the deferred body source certificate.
func (d *Decompiler) inlineUnchangedLambdaParameterOperands(op *OpCode, value values.JavaValue) {
	d.inlineUnchangedLambdaOperandCopies(op, value, false)
}

// Local STORE witnesses exist only after their real source declarations have
// been built. Reopen them in statement construction, before declaring the
// factory's operand snapshots. The factory/result itself stays at its PC.
func (d *Decompiler) inlineUnchangedLambdaLocalOperands(op *OpCode, value values.JavaValue) {
	d.inlineUnchangedLambdaOperandCopies(op, value, true)
}

func (d *Decompiler) inlineUnchangedLambdaOperandCopies(op *OpCode, value values.JavaValue, locals bool) {
	lambda, ok := value.(*values.CustomValue)
	if d == nil || op == nil || op.Instr == nil || op.Instr.OpCode != OP_INVOKEDYNAMIC || !ok || lambda == nil || (!lambda.LambdaParameterNamesRequired() && !lambda.OriginalReferenceCaptureRequired()) || d.FunctionContext == nil {
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
		if !locals && proved && snapshot.ExpectedType != nil {
			// A hidden field rendered as its enclosing variable is captured by
			// javac at this factory, not read later by the deferred lambda body.
			// Keep the original factory/result and every other operand snapshot.
			receiver, lexical := lambda.OriginalLambdaLexicalFieldReceiver(pc, position, ref, snapshot.Value, d.FunctionContext)
			if lexical {
				_, initialized := d.unchangedCaptureReceiverSlot(receiver)
				actual, typed := values.SourceTypeErasure(snapshot.Value.Type(), d.FunctionContext)
				expected, known := values.SourceTypeErasure(snapshot.ExpectedType, d.FunctionContext)
				if initialized && typed && known && actual == expected {
					ref.StackVar = snapshot.Value
					ref.MarkOriginalDynamicOperandInline(pc, snapshot.Value)
					continue
				}
			}
		}
		source, parameter := dynamicCaptureSeed(snapshot.Value).(*values.JavaRef)
		slot, stable := d.unchangedCaptureParameterSlot(source)
		originalSlot, sealed := source.OriginalParameterSlot()
		if locals {
			stable = d.unchangedCaptureLocalAt(source, op)
			sealed, slot, originalSlot = stable, 0, 0
		} else if parameter && source != nil && source.IsThis {
			slot, stable = d.unchangedCaptureReceiverSlot(source)
			originalSlot, sealed = source.OriginalReceiverSlot()
			proved = proved && position == 0
		}
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

// A single original STORE must dominate this factory on normal and exceptional
// paths. Inspect every original overlapping write, including an unreachable
// STORE, IINC and the second word of a category-2 value. Matching PC/name/type
// alone cannot establish a source local's stable value.
func (d *Decompiler) unchangedCaptureLocalAt(ref *values.JavaRef, use *OpCode) bool {
	if d == nil || d.FunctionContext == nil || d.FunctionType == nil || d.semanticCFG == nil || ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || use == nil || use.IsCustom {
		return false
	}
	pc, slot, sealed := ref.OriginalLocalDeclaration(ref.Val)
	descriptor, typed := values.SourceTypeErasure(ref.Type(), d.FunctionContext)
	g := d.semanticCFG
	if !sealed || !typed || descriptor == "" || pc >= int(use.CurrentOffset) || len(g.Nodes) > 65536 || d.Work != nil && (d.Work.CheckAlloc(int64(len(g.Nodes))*128) != nil || d.Work.Charge(workbudget.CounterGraphScans, int64(len(g.Nodes)+len(g.Edges)+1)) != nil) {
		return false
	}
	width := 1
	if descriptor == "J" || descriptor == "D" {
		width = 2
	}
	// Recursive body callbacks share mutable source contexts. The original
	// graph froze physical entry words before those callbacks ran.
	for entry, words := range g.entryWidth {
		if entry < slot+width && slot < entry+words {
			return false
		}
	}
	var definition *OpCode
	for _, op := range g.Nodes {
		if op == nil || op.Instr == nil {
			return false
		}
		access := LocalAccessOf(op.Instr.OpCode)
		if !access.Write {
			continue
		}
		written := GetStoreIdx(op)
		if written < 0 || access.Width < 1 || access.Width > 2 {
			return false
		}
		if written < slot+width && slot < written+access.Width {
			if definition != nil || op.IsCustom || int(op.CurrentOffset) != pc || written != slot || access.Read || access.Width != width || !lambdaCaptureStoreDescriptor(op.Instr.OpCode, descriptor) {
				return false
			}
			definition = op
		}
	}
	if definition == nil {
		return false
	}
	stores, entry := g.ReachingDefinitions(use, slot)
	a, b := g.indexOfNode(definition), g.indexOfNode(use)
	return g.Err == nil && !entry && len(stores) == 1 && stores[0] == definition && a >= 0 && b >= 0 && g.GetOrCompute(AnalysisDominators, GraphAnalysisDomain{Roots: []int{0}, IncludeException: true}).Dominates(a, b)
}

func lambdaCaptureStoreDescriptor(opcode int, descriptor string) bool {
	if descriptor == "" {
		return false
	}
	switch descriptor[0] {
	case 'L', '[':
		return opcode == OP_ASTORE || opcode >= OP_ASTORE_0 && opcode <= OP_ASTORE_3
	case 'Z', 'B', 'C', 'S', 'I':
		return dynamicPrimitiveStore(opcode, types.JavaInteger)
	case 'J':
		return dynamicPrimitiveStore(opcode, types.JavaLong)
	case 'F':
		return dynamicPrimitiveStore(opcode, types.JavaFloat)
	case 'D':
		return dynamicPrimitiveStore(opcode, types.JavaDouble)
	}
	return false
}
