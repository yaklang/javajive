package javaclassparser

import (
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The callback's early context precedes final identity/name allocation. Freeze
// the context that actually emits this factory, while its owning method frame
// is active. Analysis previews cannot certify the final local spelling.
func (c *ClassObjectDumper) bindLambdaLocalCaptureEmission(body *dumpedMethods, frame *lambdaMethodEmission) bool {
	if body == nil || body.lambdaParameterBinding == nil {
		return true
	}
	b := body.lambdaParameterBinding
	if b.metadata == nil || b.metadata.factory == nil {
		return false
	}
	local := false
	for _, operand := range b.metadata.captures {
		local = local || operand != nil && operand.local != nil
	}
	if !local {
		return true
	}
	factory := b.metadata.factory
	if c == nil || c.FuncCtx == nil || c.CurrentMethod != factory.method || frame == nil || frame.ended || frame.body == nil || frame.body.member != factory.method || c.lambdaMethodRendering != frame || b.captureEmission != nil {
		return false
	}
	name, named := sourceBridgeUTF8(c.obj, factory.method.NameIndex)
	descriptor, typed := sourceBridgeUTF8(c.obj, factory.method.DescriptorIndex)
	if !named || !typed || c.FuncCtx.FunctionName != name || c.FuncCtx.CurrentMethodDesc != descriptor {
		return false
	}
	context, known := c.FuncCtx.SnapshotForSourceProof(c.Work)
	if !known || !b.capturedNamesInContextClosed(context, c.Work) || c.Work != nil && c.Work.CheckAlloc(int64(len(b.captured))*16) != nil {
		return false
	}
	identities := make([]*coreutils.VariableId, len(b.captured))
	sources := make([]*coreutils.VariableId, len(b.captured))
	for i, capture := range b.captured {
		v, known := nativeMemberEnclosingUnpack(capture, c.Work)
		ref, typed := v.(*values.JavaRef)
		if !known || !typed || ref == nil || ref.Id == nil {
			return false
		}
		identities[i] = ref.Id
		source, known := lambdaCaptureSourceReference(ref, factory.pc, i+b.metadata.receiverWords, c.Work)
		if !known {
			return false
		}
		sources[i] = source.Id
	}
	b.captureContext, b.captureEmission, b.captureDeclarationIDs = context, frame, identities
	b.captureSourceIDs = sources
	return true
}

// Retain eager operand identity and final source declaration identity separately.
// A sealed inline copy does not have its own source declaration, and arbitrary
// StackVar/ref.Val histories cannot replace the actual binder.
func lambdaCaptureSourceReference(ref *values.JavaRef, pc, index int, work *workbudget.Budget) (*values.JavaRef, bool) {
	if ref == nil || ref.Id == nil {
		return nil, false
	}
	if ref.StackVar == nil {
		return ref, true
	}
	location, ordinal, sealed := ref.OriginalInlinedDynamicOperandWitness(ref.Val)
	if !sealed || location != pc || ordinal != index {
		return nil, false
	}
	v, known := nativeMemberEnclosingUnpack(ref.StackVar, work)
	source, typed := v.(*values.JavaRef)
	return source, known && typed && source != nil && source.Id != nil && source.StackVar == nil && source.CustomValue == nil
}

// Metadata constrains the existing factory snapshot's source spelling. The
// retained statement pass still proves the original typed local STORE, seed,
// snapshot declaration and visibility before applying any name. This record
// creates neither a source declaration nor a replacement capture value.
func (c *ClassObjectDumper) recordLambdaLocalParameterNameDemands(p *lambdaParameterMetadata, captures []values.JavaValue) bool {
	if c == nil || p == nil || p.factory == nil || !p.factory.closed() || len(captures) != len(p.captures) || len(captures) > 64 || len(p.names) < len(captures) || len(c.nativeLambdaCaptureParameterNames) > 4096 || !nativeProofWork(c.Work, int64(len(captures))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(captures))*256+256) != nil {
		return false
	}
	operands := p.captures
	if p.receiverWords != 0 {
		if p.receiver == nil {
			return false
		}
		operands = append([]*nativeEnumSelectorProducer{p.receiver}, p.captures...)
	}
	site := &nativeLambdaLocalCaptureSite{method: p.factory.method, code: p.factory.code, pc: p.factory.pc, operands: operands}
	var pending []nativeLambdaCaptureNameDemand
	for i, operand := range p.captures {
		if operand == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		if operand.local == nil {
			continue
		}
		found := false
		for _, prior := range c.nativeLambdaCaptureParameterNames[site.method] {
			if !nativeProofWork(c.Work, 1) || prior.site == nil {
				return false
			}
			if prior.site.method == site.method && prior.site.code == site.code && prior.site.pc == site.pc && prior.index == i+p.receiverWords {
				if prior.name != p.names[i] || prior.value != captures[i] {
					return false
				}
				found = true
			}
		}
		if !found {
			pending = append(pending, nativeLambdaCaptureNameDemand{site: site, index: i + p.receiverWords, name: p.names[i], value: captures[i]})
		}
	}
	if len(pending) == 0 {
		return true
	}
	if len(c.nativeLambdaCaptureParameterNames[site.method])+len(pending) > 64 || c.Work != nil && c.Work.CheckAlloc(int64(len(pending))*256+256) != nil {
		return false
	}
	if c.nativeLambdaCaptureParameterNames == nil {
		c.nativeLambdaCaptureParameterNames = map[*MemberInfo][]nativeLambdaCaptureNameDemand{}
	}
	c.nativeLambdaCaptureParameterNames[site.method] = append(c.nativeLambdaCaptureParameterNames[site.method], pending...)
	return true
}
