package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// An allocation certificate binds every original hidden constructor argument
// to an immutable declaring-method parameter slot. Equal nominal types or
// source names do not identify captured values. This initial profile refuses
// producer/phi captures until their source declarations and dominance close.
// Neither this proof nor the metadata/default-constructor proofs license a
// source local class or private access without a complete source transaction.
type nativeMethodLocalAllocation struct {
	newPC, invokePC int
	slots           []int
}

func (c *ClassObjectDumper) nativeMethodLocalParameterAllocations(local *ClassObject, owner *nativeMethodLocalOwner, constructor *nativeMethodLocalConstructor) (map[int]nativeMethodLocalAllocation, bool) {
	if c == nil || c.obj == nil || local == nil || owner == nil || constructor == nil || owner.owner != c.obj.GetClassName() || owner.declaration == nil {
		return nil, false
	}
	// Revalidate physical ownership at the boundary, rather than trusting a
	// cached pointer whose attributes or constructor packet may have changed.
	physical, known := originalMethodLocalOwner(local, c.obj, c.Work)
	if !known || physical.method != owner.method || physical.descriptor != owner.descriptor || physical.name != owner.name || physical.ordinal != owner.ordinal || physical.declaration != owner.declaration {
		return nil, false
	}
	actual, known := originalMethodLocalDefaultConstructor(local, c.obj, c.Work)
	if !known || actual.descriptor != constructor.descriptor || actual.delegatePC != constructor.delegatePC || actual.delegateOwner != constructor.delegateOwner || actual.enclosingField != constructor.enclosingField || len(actual.captures) != len(constructor.captures) {
		return nil, false
	}
	for field, index := range actual.captures {
		cachedIndex, cached := constructor.captures[field]
		cachedPC, cachedSite := constructor.capturePCs[field]
		if !nativeProofWork(c.Work, 1) || !cached || !cachedSite || cachedIndex != index || cachedPC != actual.capturePCs[field] {
			return nil, false
		}
	}
	var code *CodeAttribute
	for _, attribute := range owner.declaration.Attributes {
		if body, ok := attribute.(*CodeAttribute); ok {
			if body == nil || code != nil {
				return nil, false
			}
			code = body
		}
	}
	if code == nil {
		return nil, false
	}
	ir, fn, known := c.nativeOriginalMethodSnapshot(owner.declaration, code)
	if !known {
		return nil, false
	}
	methodParams, _, err := callbinding.Descriptor(owner.descriptor)
	captureParams, ret, capErr := callbinding.Descriptor(constructor.descriptor)
	if err != nil || capErr != nil || ret != "V" {
		return nil, false
	}
	parameterTypes := map[int]string{}
	slot := 0
	if owner.declaration.AccessFlags&8 == 0 {
		parameterTypes[0] = "L" + owner.owner + ";"
		slot = 1
	}
	for _, descriptor := range methodParams {
		parameterTypes[slot] = descriptor
		slot++
		if descriptor == "J" || descriptor == "D" {
			slot++
		}
	}
	result := map[int]nativeMethodLocalAllocation{}
	var sharedSlots []int
	for _, record := range fn.Instructions {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		instruction, exists := ir.InstrByID(methodir.InstrID(record.PC))
		if !exists || instruction.Opcode != core.OP_INVOKESPECIAL || instruction.Class != local.GetClassName() || instruction.Member != "<init>" {
			continue
		}
		if instruction.Desc != constructor.descriptor || len(record.BeforeOrigins) != len(record.Before.Locals)+len(record.Before.Stack) {
			return nil, false
		}
		receiverIndex := len(record.Before.Stack) - nativeMemberParameterWidth(captureParams) - 1
		if receiverIndex < 0 {
			return nil, false
		}
		receiver := record.Before.Stack[receiverIndex]
		origin := record.BeforeOrigins[len(record.Before.Locals)+receiverIndex]
		allocation, exists := ir.InstrByID(methodir.InstrID(receiver.NewPC))
		if receiver.Kind != frametransfer.UninitNew || receiver.Class != local.GetClassName() || origin.Kind != ssabuild.OriginInstr || origin.PC != receiver.NewPC || !exists || allocation.Opcode != core.OP_NEW || allocation.Class != local.GetClassName() {
			return nil, false
		}
		if _, duplicate := result[int(receiver.NewPC)]; duplicate {
			return nil, false
		}
		if c.Work != nil && c.Work.CheckAlloc(int64(len(result)+1)*int64(len(captureParams)+1)*16) != nil {
			return nil, false
		}
		proof := nativeMethodLocalAllocation{newPC: int(receiver.NewPC), invokePC: int(record.PC), slots: make([]int, len(captureParams))}
		at := receiverIndex + 1
		for i, descriptor := range captureParams {
			if !nativeProofWork(c.Work, 1) || at >= len(record.Before.Stack) {
				return nil, false
			}
			argument := record.BeforeOrigins[len(record.Before.Locals)+at]
			if argument.Kind != ssabuild.OriginParam || parameterTypes[argument.Slot] != descriptor {
				return nil, false
			}
			if i == constructor.captures[constructor.enclosingField] && constructor.enclosingField != "" && argument.Slot != 0 {
				return nil, false
			}
			proof.slots[i] = argument.Slot
			at++
			if descriptor == "J" || descriptor == "D" {
				at++
			}
		}
		if sharedSlots != nil {
			for i, slot := range proof.slots {
				if slot != sharedSlots[i] {
					return nil, false
				}
			}
		} else {
			sharedSlots = append([]int(nil), proof.slots...)
		}
		result[proof.newPC] = proof
	}
	return result, len(result) > 0
}

// The source operand must name the same descriptor-seeded parameter. Mutable
// replacement values, equal-typed swapped parameters and arbitrary this-like
// text cannot satisfy a physical parameter certificate. Source stability and
// membership in the declaring method's actual parameter list are additional
// obligations for the later placement transaction.
func nativeMethodLocalParameterOperand(value values.JavaValue, slot int, descriptor string, ctx *class_context.ClassContext, work *workbudget.Budget) bool {
	if ctx == nil || slot < 0 {
		return false
	}
	for depth := 0; depth < 32; depth++ {
		if sourceProofNil(value) || !nativeProofWork(work, 1) {
			return false
		}
		if wrapped, ok := value.(*values.SlotValue); ok {
			value = wrapped.GetValue()
			continue
		}
		ref, ok := value.(*values.JavaRef)
		if !ok || ref == nil || ref.CustomValue != nil || ref.StackVar != nil {
			return false
		}
		erasure, known := values.SourceTypeErasure(ref.Type(), ctx)
		if !known || erasure != descriptor {
			return false
		}
		if slot == 0 && !ctx.IsStatic {
			return ref.IsThis && descriptor == "L"+strings.ReplaceAll(ctx.ClassName, ".", "/")+";"
		}
		actual, known := ref.OriginalParameterSlot()
		return known && actual == slot
	}
	return false
}
