package core

import (
	"bytes"
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Materializing a nested factory may precede the source expression consuming
// an earlier JVM stack operand. Retain that actual operand, rather than making
// the outer compiler discover a later capture first. Only unchanged original
// capture parameters qualify: copying their words here moves no effects or
// failure, and the factory/allocation remains at the original invokedynamic.
func (d *Decompiler) preserveEarlierLambdaCaptureOperands(op *OpCode, simulation StackSimulation, value values.JavaValue) {
	lambda, ok := value.(*values.CustomValue)
	if d == nil || op == nil || op.Instr == nil || op.Instr.OpCode != OP_INVOKEDYNAMIC || !ok || lambda == nil || lambda.IsMethodRef || len(d.LambdaCaptureOrderSlots) == 0 || len(d.LambdaCaptureOrderSlots) > 255 || len(d.opCodes) > 65536 || d.FunctionType == nil || len(d.FunctionType.ParamTypes) > 255 || simulation == nil {
		return
	}
	if d.FunctionContext == nil || !d.lambdaEarlierOriginalInstruction(op) {
		return
	}
	startWord := 0
	if !d.FunctionContext.IsStatic {
		startWord = 1
	}
	for i, slot := range d.LambdaCaptureOrderSlots {
		if slot < 0 || slot > 254 || i == 0 && slot != startWord || i > 0 && slot <= d.LambdaCaptureOrderSlots[i-1] {
			return
		}
	}
	pc, captures, known := lambda.OriginalLambdaFactoryEvaluation()
	if !known || pc != int(op.CurrentOffset) || len(captures) == 0 {
		return
	}
	var sim *StackSimulationImpl
	switch stack := simulation.(type) {
	case *StackSimulationImpl:
		sim = stack
	case *StackSimulationProxy:
		if stack != nil {
			sim = stack.StackSimulationImpl
		}
	}
	if sim == nil || sim.Size() == 0 || sim.Size() > 256 || d.chargeNodeCopies(sim.Size()+1) != nil || d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(sim.Size())*int64(len(d.LambdaCaptureOrderSlots)+len(d.Params)+len(d.opCodes)+1)) != nil || d.Work.CheckAlloc(int64(sim.Size())*128+64) != nil) {
		return
	}
	items := make([]values.JavaValue, 0, sim.Size())
	for entry := sim.stackEntry; entry != nil && entry.parent != nil; entry = entry.parent {
		items = append(items, entry.value)
	}
	type operand struct {
		index int
		value values.JavaValue
	}
	plan := []operand{}
	for index := len(items) - 1; index >= 0; index-- {
		original := items[index]
		ref, typed := dynamicCaptureSeed(original).(*values.JavaRef)
		if !typed || ref == nil || ref.IsThis || ref.Type() == nil {
			continue
		}
		slot, stable := d.unchangedCaptureParameterSlot(ref)
		sealedSlot, sealed := ref.OriginalParameterSlot()
		if !stable || !sealed || sealedSlot != slot || !slices.Contains(d.LambdaCaptureOrderSlots, slot) {
			continue
		}
		producer := d.stackValueProducers[original]
		if producer == nil || !d.lambdaEarlierOriginalInstruction(producer) || producer.CurrentOffset >= op.CurrentOffset || !LocalAccessOf(producer.Instr.OpCode).Read || GetRetrieveIdx(producer) != slot || !d.stackLifetimeProducerPath(producer, op) {
			continue
		}
		if !d.lambdaEarlierParameterLoad(ref, producer, slot) {
			continue
		}
		plan = append(plan, operand{index, original})
	}
	if len(plan) == 0 || d.Work != nil && d.Work.Err() != nil {
		return
	}
	// Validate all source/producer identities before publishing any copies.
	// Prepend them to the factory's own capture snapshots, preserving the
	// original stack order. Each copy is used by the actual later consumer.
	prefix := make([]EvaluationSnapshot, 0, len(plan))
	for _, operand := range plan {
		saved := sim.NewVar(operand.value)
		saved.ResetVarType(operand.value.Type().Copy())
		d.disFoldRef = append(d.disFoldRef, saved)
		prefix = append(prefix, EvaluationSnapshot{Ref: saved, Value: operand.value, OriginPC: pc})
		items[operand.index] = saved
	}
	d.evaluationSnapshots[op] = append(prefix, d.evaluationSnapshots[op]...)
	// Stack entries are shared with predecessor simulations. Rebuild private
	// entries without reporting additional decoded pushes or pops.
	sim.stackEntry = NewEmptyStackEntry()
	for index := len(items) - 1; index >= 0; index-- {
		sim.Push(items[index])
	}
}

// Reopen the actual narrow LOAD/factory packet. A changed IR opcode, operand,
// PC, WIDE prefix or synthetic node cannot manufacture an original stack read.
func (d *Decompiler) lambdaEarlierOriginalInstruction(op *OpCode) bool {
	if d == nil || op == nil || op.IsCustom || op.IsWide || op.Instr == nil {
		return false
	}
	length := -1
	switch kind := op.Instr.OpCode; {
	case kind == OP_INVOKEDYNAMIC:
		length = 4
	case kind >= OP_ILOAD && kind <= OP_ALOAD:
		length = 1
	case kind >= OP_ILOAD_0 && kind <= OP_ALOAD_3:
		length = 0
	}
	if length < 0 || len(op.Data) != length {
		return false
	}
	pc := int(op.CurrentOffset)
	return pc >= 0 && pc+1+len(op.Data) <= len(d.bytecodes) && int(d.bytecodes[pc]) == op.Instr.OpCode && bytes.Equal(d.bytecodes[pc+1:pc+1+len(op.Data)], op.Data)
}

func (d *Decompiler) lambdaEarlierParameterLoad(ref *values.JavaRef, op *OpCode, slot int) bool {
	actual, typed := values.SourceTypeErasure(ref.Type(), d.FunctionContext)
	if !typed || len(actual) == 0 {
		return false
	}
	// CalcOpcodeStackInfo builds the physical entry layout by prepending
	// the receiver to FunctionType.ParamTypes for instance implementations.
	// These are already JVM local words: counting a second receiver shifts
	// every subsequent declaration and can even walk beyond the last capture.
	word := 0
	matched := false
	for _, param := range d.FunctionType.ParamTypes {
		descriptor, known := values.SourceTypeErasure(param, d.FunctionContext)
		if !known || descriptor == "" {
			return false
		}
		if word == slot {
			matched = descriptor == actual
			break
		}
		word++
		if descriptor == "J" || descriptor == "D" {
			word++
		}
	}
	if !matched {
		return false
	}
	kind := actual[0]
	if kind == '[' {
		kind = 'L'
	}
	if kind == 'B' || kind == 'C' || kind == 'S' || kind == 'Z' {
		kind = 'I'
	}
	category := strings.IndexByte("IJFDL", kind)
	opcode := op.Instr.OpCode
	return category >= 0 && (opcode == OP_ILOAD+category || opcode >= OP_ILOAD_0+category*4 && opcode <= OP_ILOAD_3+category*4)
}
