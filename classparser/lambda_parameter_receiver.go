package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A constructor's slot zero begins as uninitialized THIS. Ask the immutable
// original verifier/origin snapshot at both its actual capture LOAD and the
// actual factory instruction. A textual super() occurrence or an earlier NEW
// of another object cannot initialize that receiver. The snapshot's joined
// frames cover every original incoming path, including exception edges.
func lambdaParameterReceiverInitialized(p *lambdaParameterMetadata, work *workbudget.Budget) bool {
	if p == nil || p.object == nil || p.receiverWords != 1 || p.receiver == nil || p.receiver.slot != 0 || p.receiver.result != "L"+p.object.GetClassName()+";" || !p.factory.closed() || !p.factory.sealed || p.factory.method.AccessFlags&8 != 0 || p.receiver.pc < 0 || p.receiver.pc >= p.factory.pc || !nativeProofWork(work, 1) {
		return false
	}
	name, known := sourceBridgeUTF8(p.object, p.factory.method.NameIndex)
	if !known {
		return false
	}
	if name != "<init>" {
		return true // ordinary instance entry is already initialized
	}
	reader := ClassObjectDumper{obj: p.object, Work: work}
	ir, frames, known := reader.nativeOriginalMethodSnapshot(p.factory.method, p.factory.code)
	if !known {
		return false
	}
	words := p.receiverWords
	for _, descriptor := range p.captureTypes {
		words++
		if descriptor == "J" || descriptor == "D" {
			words++
		}
	}
	loads, factories := 0, 0
	for _, record := range frames.Instructions {
		if !nativeProofWork(work, 1) {
			return false
		}
		if int(record.PC) != p.receiver.pc && int(record.PC) != p.factory.pc {
			continue
		}
		instruction, exists := ir.InstrByID(methodir.InstrID(record.PC))
		frame := record.Before
		if !exists || frame.ThisUninitialized || len(frame.Locals) == 0 || len(record.BeforeOrigins) != len(frame.Locals)+len(frame.Stack) || frame.Locals[0].Kind != frametransfer.Ref || frame.Locals[0].Class != p.object.GetClassName() || record.BeforeOrigins[0] != (ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: 0}) {
			return false
		}
		if int(record.PC) == p.receiver.pc {
			if instruction.Opcode != p.receiver.opcode {
				return false
			}
			loads++
		} else {
			base := len(frame.Stack) - words
			if instruction.Opcode != core.OP_INVOKEDYNAMIC || base < 0 || frame.Stack[base].Kind != frametransfer.Ref || frame.Stack[base].Class != p.object.GetClassName() || record.BeforeOrigins[len(frame.Locals)+base] != (ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: 0}) {
				return false
			}
			factories++
		}
	}
	return loads == 1 && factories == 1
}

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
