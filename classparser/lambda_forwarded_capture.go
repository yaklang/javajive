package javaclassparser

import (
	"fmt"
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A parent's physical capture prefix is not a source SAM parameter. Follow
// the exact original factory edge and word boundary; descriptor/name equality
// alone cannot transfer another same-typed captured value.
func lambdaForwardedCaptureIndex(p *lambdaParameterMetadata, index int) (int, bool) {
	if p == nil || p.parent == nil || p.factory == nil || !p.factory.closed() || p.parent.factory == nil || !p.parent.factory.closed() || p.object != p.parent.object || p.factory.method != p.parent.method || index < 0 || index >= len(p.captures) || index >= len(p.captureTypes) || index >= len(p.names) || index >= len(p.flags) || len(p.parent.captureTypes) != len(p.parent.captures) || len(p.parent.captureTypes) > len(p.parent.names) || len(p.parent.captureTypes) > len(p.parent.flags) {
		return 0, false
	}
	operand := p.captures[index]
	if operand == nil || operand.local != nil || operand.owner != "" || operand.member != "" || operand.storage != nil || len(operand.operands) != 0 || operand.result != p.captureTypes[index] || p.flags[index] != 0x1010 {
		return 0, false
	}
	slot := p.parent.receiverWords
	for i, descriptor := range p.parent.captureTypes {
		if slot == operand.slot {
			return i, descriptor == operand.result && p.parent.names[i] == p.names[index] && p.parent.flags[i] == 0x1010
		}
		slot++
		if descriptor == "J" || descriptor == "D" {
			slot++
		}
	}
	return 0, false
}

func (b *lambdaParameterBinding) hasForwardedCaptures() bool {
	if b == nil || b.metadata == nil {
		return false
	}
	for i := range b.metadata.captures {
		if _, known := lambdaForwardedCaptureIndex(b.metadata, i); known {
			return true
		}
	}
	return false
}

func (b *lambdaParameterBinding) bindCaptureEntries(parameters []values.JavaValue, ctx *class_context.ClassContext, work *workbudget.Budget) bool {
	if b == nil || b.metadata == nil || len(b.metadata.captureTypes) != len(b.metadata.captures) || len(parameters) < len(b.metadata.captures) || len(parameters) > 255 || len(b.captureEntries) != 0 || len(b.captureEntryIDs) != 0 || !nativeProofWork(work, int64(len(parameters))+1) || work != nil && work.CheckAlloc(int64(len(parameters))*32) != nil {
		return false
	}
	slot := b.metadata.receiverWords
	seen := map[*coreutils.VariableId]bool{}
	for i, descriptor := range b.metadata.captureTypes {
		ref, known := parameters[i].(*values.JavaRef)
		if !known || ref == nil || ref.Id == nil || seen[ref.Id] || !nativeMethodLocalParameterOperand(ref, slot, descriptor, ctx, work) {
			return false
		}
		seen[ref.Id] = true
		b.captureEntries = append(b.captureEntries, ref)
		b.captureEntryIDs = append(b.captureEntryIDs, ref.Id)
		slot++
		if descriptor == "J" || descriptor == "D" {
			slot++
		}
	}
	return true
}

func (b *lambdaParameterBinding) captureEntriesClosed(ctx *class_context.ClassContext, work *workbudget.Budget) bool {
	if b == nil || b.metadata == nil || ctx == nil || len(b.metadata.captureTypes) != len(b.metadata.captures) || len(b.captureEntries) != len(b.metadata.captures) || len(b.captureEntryIDs) != len(b.captureEntries) {
		return false
	}
	slot := b.metadata.receiverWords
	seen := map[*coreutils.VariableId]bool{}
	for i, descriptor := range b.metadata.captureTypes {
		ref := b.captureEntries[i]
		if ref == nil || ref.Id == nil || ref.Id != b.captureEntryIDs[i] || seen[ref.Id] || !nativeMethodLocalParameterOperand(ref, slot, descriptor, ctx, work) || ref.String(ctx) != fmt.Sprintf("\x00LCAP%d\x00", i+b.metadata.receiverWords) {
			return false
		}
		seen[ref.Id] = true
		slot++
		if descriptor == "J" || descriptor == "D" {
			slot++
		}
	}
	return true
}

func (b *lambdaParameterBinding) forwardingParentMatches(parent *lambdaParameterBinding, frame *lambdaMethodEmission) bool {
	if b == nil || b.metadata == nil || b.metadata.factory == nil || b.metadata.parent == nil || parent == nil || parent.metadata == nil || frame == nil || frame.body == nil || frame.body.lambdaParameterBinding != parent || frame.body.member != b.metadata.factory.method || frame.request != frame.body.sourceRequest || parent.request != frame.request || b.metadata.object != parent.metadata.object || b.metadata.parent.method != parent.metadata.method || !b.metadata.parent.factory.same(parent.metadata.factory) || !slices.Equal(b.metadata.parent.parameters, parent.metadata.parameters) || !slices.Equal(b.metadata.parent.names, parent.metadata.names) || !slices.Equal(b.metadata.parent.flags, parent.metadata.flags) || !slices.Equal(b.metadata.parent.captureTypes, parent.metadata.captureTypes) {
		return false
	}
	return true
}

func (b *lambdaParameterBinding) forwardedCaptureEntryClosed(index int, captured *values.JavaRef, ctx *class_context.ClassContext, work *workbudget.Budget) bool {
	if b == nil || b.metadata == nil {
		return false
	}
	parentIndex, forwarded := lambdaForwardedCaptureIndex(b.metadata, index)
	if !forwarded || b.forwardParent == nil || !b.forwardingParentMatches(b.forwardParent, b.forwardEmission) || !b.forwardParent.captureEntriesClosed(ctx, work) {
		return false
	}
	source, known := lambdaCaptureSourceReference(captured, b.metadata.factory.pc, index+b.metadata.receiverWords, work)
	if !known || source.Id != b.forwardParent.captureEntryIDs[parentIndex] || source.Id != b.forwardParent.captureEntries[parentIndex].Id || source.String(ctx) != fmt.Sprintf("\x00LCAP%d\x00", parentIndex+b.forwardParent.metadata.receiverWords) {
		return false
	}
	slot, original := source.OriginalParameterSlot()
	return original && slot == b.metadata.captures[index].slot
}

func (b *lambdaParameterBinding) forwardingEmissionClosed(work *workbudget.Budget) bool {
	if b == nil || b.forwardParent == nil || b.forwardEmission == nil || !b.forwardEmission.closed() || !b.forwardingParentMatches(b.forwardParent, b.forwardEmission) || b.captureEmission != b.forwardEmission || !b.forwardParent.captureEntriesClosed(b.captureContext, work) || !b.forwardParent.parametersClosed(b.forwardParent.context, work) || b.forwardParent.arrow == "" || b.forwardEmission.body.code != b.forwardParent.arrow {
		return false
	}
	return true
}
