package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The actual factory snapshots THIS separately from all physical parameters.
// Neither an IsThis flag, equal source text nor a matching erased type can
// replace the descriptor-seeded receiver and the original dynamic operand.
func lambdaParameterReceiverPendingClosed(p *lambdaParameterMetadata, value values.JavaValue, context *class_context.ClassContext, work *workbudget.Budget) bool {
	if p == nil || p.receiverWords != 1 || p.receiver == nil || !p.factory.closed() || context == nil || context.IsStatic {
		return false
	}
	v, known := nativeMemberEnclosingUnpack(value, work)
	ref, typed := v.(*values.JavaRef)
	if !known || !typed || ref == nil || ref.Id == nil || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil {
		return false
	}
	pc, index, original := ref.OriginalPendingDynamicOperand(ref.Val)
	seed, known := nativeMemberEnclosingUnpack(ref.Val, work)
	receiver, typed := seed.(*values.JavaRef)
	erasure, erased := values.SourceTypeErasure(ref.Type(), context)
	if !original || pc != p.factory.pc || index != 0 || !known || !typed || receiver == nil || !erased || erasure != p.receiver.result {
		return false
	}
	slot, original := receiver.OriginalReceiverSlot()
	receiverType, typed := values.SourceTypeErasure(receiver.Type(), context)
	return original && slot == 0 && typed && receiverType == p.receiver.result
}

// The completed writer must still emit the original implicit receiver. A
// materialized alias would change the compiler's capture roles and helper
// descriptor even if the alias happened to point to the same object.
func lambdaParameterReceiverSourceClosed(p *lambdaParameterMetadata, value values.JavaValue, context *class_context.ClassContext, work *workbudget.Budget) bool {
	if p == nil || p.receiverWords != 1 || p.receiver == nil || !p.factory.closed() || context == nil || context.IsStatic {
		return false
	}
	v, known := nativeMemberEnclosingUnpack(value, work)
	ref, typed := v.(*values.JavaRef)
	if !known || !typed || ref == nil || ref.Id == nil {
		return false
	}
	pc, index, original := ref.OriginalInlinedDynamicOperandWitness(ref.Val)
	return original && pc == p.factory.pc && index == 0 && nativeLambdaOriginalReceiverSource(value, pc, index, work) && nativeLambdaLocalCaptureValue(p.receiver, value, pc, index, context, work) && ref.String(context) == "this"
}
