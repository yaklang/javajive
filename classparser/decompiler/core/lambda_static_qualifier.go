package core

import (
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// javac evaluates an expression qualifying a static class invocation and
// discards its value before evaluating arguments. In a lambda, even a pure
// captured qualifier contributes a physical implementation argument. Preserve
// that actual LOAD/POP as the call's qualifier, rather than inventing an unused
// read or silently dropping a capture. This narrow domain moves no effects:
// the qualifier and intervening arguments are original adjacent local LOADs.
func (d *Decompiler) retainLambdaStaticQualifier(op *OpCode, call *values.FunctionCallExpression) {
	if d == nil || len(d.LambdaCaptureOrderSlots) == 0 && !d.LambdaReceiverCapture || d.FunctionContext == nil || d.FunctionContext.InvocationMetadata == nil || op == nil || op.Instr == nil || op.Instr.OpCode != OP_INVOKESTATIC || call == nil || !call.IsStatic || call.Kind != values.InvokeStatic || !call.HasOriginPC || call.OriginPC != int(op.CurrentOffset) {
		return
	}
	if len(call.Descriptor) > 65535 || d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(call.Descriptor))+1) != nil || d.Work.CheckAlloc(int64(len(call.Descriptor))*32+256) != nil) {
		return
	}
	parameters, _, err := callbinding.Descriptor(call.Descriptor)
	if err != nil || len(parameters) > 255 || len(parameters) != len(call.Arguments) {
		return
	}
	previous := func(current *OpCode) *OpCode {
		if current == nil || len(current.Source) != 1 || d.Work != nil && d.Work.Charge(workbudget.CounterGraphEdges, 1) != nil {
			return nil
		}
		prior := current.Source[0]
		if prior == nil || prior.Instr == nil || len(prior.Target) != 1 || prior.Target[0] != current || int(prior.CurrentOffset)+1+len(prior.Data) != int(current.CurrentOffset) || prior.IsWide || current.IsCatch || current.IsTryCatchParent {
			return nil
		}
		for _, handler := range d.ExceptionTable {
			if handler == nil || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil || handler.HandlerPc == current.CurrentOffset {
				return nil
			}
		}
		return prior
	}
	cursor := op
	for i := len(parameters) - 1; i >= 0; i-- {
		cursor = previous(cursor)
		if cursor == nil {
			return
		}
		access := LocalAccessOf(cursor.Instr.OpCode)
		width := 1
		if parameters[i] == "J" || parameters[i] == "D" {
			width = 2
		}
		if !access.Read || access.Write || access.Width != width || GetRetrieveIdx(cursor) < 0 {
			return
		}
	}
	pop := previous(cursor)
	if pop == nil || pop.Instr.OpCode != OP_POP || len(pop.stackConsumed) != 1 {
		return
	}
	load := previous(pop)
	if load == nil || load.Instr.OpCode != OP_ALOAD && (load.Instr.OpCode < OP_ALOAD_0 || load.Instr.OpCode > OP_ALOAD_3) {
		return
	}
	ref, known := values.UnpackSoltValue(pop.stackConsumed[0]).(*values.JavaRef)
	if !known || ref == nil {
		return
	}
	slot, original := ref.OriginalParameterSlot()
	captured := slices.Contains(d.LambdaCaptureOrderSlots, slot)
	if ref.IsThis {
		slot, original = ref.OriginalReceiverSlot()
		captured = d.LambdaReceiverCapture && slot == 0 && !d.FunctionContext.IsStatic
	}
	owner := strings.ReplaceAll(call.ClassName, ".", "/")
	erasure, typed := values.SourceTypeErasure(ref.Type(), d.FunctionContext)
	if !original || slot != GetRetrieveIdx(load) || !captured || !typed || erasure != "L"+owner+";" {
		return
	}
	metadata, known := d.FunctionContext.InvocationMetadata(owner)
	if !known || metadata.Name != owner || metadata.IsInterface || !metadata.MembersComplete || !metadata.ParentsComplete || len(metadata.Methods) > 4096 {
		return
	}
	targets := 0
	for _, method := range metadata.Methods {
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return
		}
		if method.Name == call.FunctionName && method.Desc == call.Descriptor && method.Static && !method.Bridge {
			targets++
		}
	}
	if targets == 1 {
		call.Object = pop.stackConsumed[0]
	}
}
