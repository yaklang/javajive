package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A bound reference can consume a typed local definition, not only an entry
// parameter. Reuse the exception-aware last-writer frontier and source capture
// proof. The original eager null check remains part of the required stack port;
// no producer is substituted or evaluated again at the reference occurrence.
func (c *ClassObjectDumper) compilerReferenceLocalPort(f *lambdaSourceFactory, shape *lambdaVarargsAdapter, receiver *values.JavaRef, descriptor string, ctx *class_context.ClassContext) *nativeEnumSelectorProducer {
	if c == nil || c.obj == nil || !f.closed() || !f.sealed || f.object != c.obj || shape == nil || receiver == nil || receiver.Id == nil || receiver.CustomValue != nil || ctx == nil || ctx.InvocationMetadata == nil || strings.ReplaceAll(ctx.ClassName, ".", "/") != c.obj.GetClassName() {
		return nil
	}
	name, nk := sourceBridgeUTF8(c.obj, f.method.NameIndex)
	desc, dk := sourceBridgeUTF8(c.obj, f.method.DescriptorIndex)
	if !nk || !dk || ctx.FunctionName != name || ctx.CurrentMethodDesc != desc || ctx.IsStatic != (f.method.AccessFlags&StaticFlag != 0) || constructorLoweringCode(f.method) != f.code {
		return nil
	}
	owned := 0
	for _, method := range c.obj.Methods {
		if !nativeProofWork(c.Work, 1) {
			return nil
		}
		if method == f.method {
			owned++
		}
	}
	if owned != 1 {
		return nil
	}
	pc, index, sealed := receiver.OriginalPendingDynamicOperand(receiver.Val)
	if !sealed {
		pc, index, sealed = receiver.OriginalDynamicOperandWitness(receiver.Val)
	}
	if !sealed {
		pc, index, sealed = receiver.OriginalInlinedDynamicOperandWitness(receiver.Val)
	}
	typ, typed := values.SourceTypeErasure(receiver.Type(), ctx)
	if !sealed || pc != f.pc || index != 0 || !typed || typ != descriptor {
		return nil
	}
	decoder := core.NewDecompiler(f.code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return nil
	}
	ops := constructorMotionOps(decoder)
	load := -1
	for i, op := range ops {
		if !nativeProofWork(c.Work, 1) {
			return nil
		}
		if int(op.CurrentOffset) == f.pc {
			at, _, known := nativeBoundAdapterCaptureOrigin(c.obj, f.method, shape.method, f.code, ops, i, []string{descriptor}, c.Work)
			if !known || load != -1 {
				return nil
			}
			load = at
		}
	}
	if load < 0 {
		return nil
	}
	parameters, _, err := callbinding.Descriptor(desc)
	if err != nil {
		return nil
	}
	params, words := map[int]string{}, 0
	if f.method.AccessFlags&StaticFlag == 0 {
		params[0] = "L" + c.obj.GetClassName() + ";"
		words++
	}
	for _, parameter := range parameters {
		params[words] = parameter
		words += constructorEffectType(parameter).width()
	}
	op := ops[load]
	slot := core.GetRetrieveIdx(op)
	if slot < words {
		return nil
	}
	flow := nativeEnumParameterOriginalFlow(decoder, f.code, c.Work)
	if flow == nil {
		return nil
	}
	reads, known := c.nativeTypedLocalReads(f.method, f.code, flow, params, true, nativeLocalReferenceDomain{descriptors: map[int]string{int(op.CurrentOffset): descriptor}, metadata: ctx.InvocationMetadata})
	read := reads[int(op.CurrentOffset)]
	if !known || read == nil || read.slot != slot || read.descriptor != descriptor || !nativeLambdaLocalDefinitionsClosed(ops, read, c.Work) {
		return nil
	}
	return &nativeEnumSelectorProducer{pc: int(op.CurrentOffset), opcode: op.Instr.OpCode, slot: slot, result: descriptor, local: read}
}

func (c *ClassObjectDumper) compilerReferenceLocalCaptureSource(p *lambdaCompilerReference) *nativeLambdaLocalCaptureSource {
	if p == nil || p.local == nil {
		return nil
	}
	site := &nativeLambdaLocalCaptureSite{method: p.factory.method, code: p.factory.code, pc: p.factory.pc, operands: []*nativeEnumSelectorProducer{p.local}}
	source := c.lambdaRetainedCaptureSource(site, []values.JavaValue{p.receiver}, c.nativeMemberBody)
	if !nativeLambdaLocalCaptureSourceClosed(site, source, c.Work) {
		return nil
	}
	return source
}
