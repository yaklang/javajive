package javaclassparser

import (
	"bytes"
	"slices"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A super reference's physical helper parameters belong to the selected
// superclass method, not to the boxed SAM. Replacing it with an arrow moves
// unboxing into the helper and changes its descriptor. Prove the complete
// receiver/argument/dispatch/result packet before asking javac to regenerate
// the compiler-owned reference. The implementation name is only a final
// compiler ordinal obligation, never evidence that the body is an adapter.
func (c *ClassObjectDumper) originalSuperReferenceShape(method *MemberInfo) *lambdaVarargsAdapter {
	if c == nil || c.obj == nil || method == nil || method.AccessFlags != 0x1002 || c.obj.MajorVersion != 52 || c.obj.AccessFlags&0x0200 != 0 || len(method.Attributes) > 64 {
		return nil
	}
	owned := 0
	for _, m := range c.obj.Methods {
		if !nativeProofWork(c.Work, 1) {
			return nil
		}
		if m == method {
			owned++
		}
	}
	desc, known := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	parameters, result, err := callbinding.Descriptor(desc)
	if owned != 1 || !known || err != nil || len(parameters) > 254 {
		return nil
	}
	var code *CodeAttribute
	for _, attr := range method.Attributes {
		switch a := attr.(type) {
		case *CodeAttribute:
			if a == nil || code != nil {
				return nil
			}
			code = a
		case *ExceptionsAttribute:
			if a == nil {
				return nil
			}
		default:
			return nil // Parameter/annotation protocols need separate proofs.
		}
	}
	if code == nil || len(code.Code) > 4096 || len(code.ExceptionTable) != 0 || !nativeProofWork(c.Work, int64(len(code.Code))+1) {
		return nil
	}
	if _, known := c.constructorThrowsOriginalExceptions(method); !known {
		return nil
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return nil
	}
	ops := constructorMotionOps(decoder)
	if len(ops) < len(parameters)+3 || !constructorMotionLoad(ops[0], "L"+c.obj.GetClassName()+";") || core.GetRetrieveIdx(ops[0]) != 0 {
		return nil
	}
	slot := 1
	for i, parameter := range parameters {
		if !nativeProofWork(c.Work, 1) || !constructorMotionLoad(ops[i+1], parameter) || core.GetRetrieveIdx(ops[i+1]) != slot {
			return nil
		}
		slot++
		if parameter == "J" || parameter == "D" {
			slot++
		}
	}
	index := len(parameters) + 1
	member := constructorMotionMember(c.obj, ops[index], core.OP_INVOKESPECIAL)
	if member == nil || member.Name != c.obj.GetSupperClassName() || !class_context.PreservesJava8Identifier(member.Member) {
		return nil
	}
	formals, returned, err := callbinding.Descriptor(member.Description)
	if err != nil || !slices.Equal(formals, parameters) {
		return nil
	}
	index, known = c.originalVarargsResultConversion(code, ops, index+1, returned, result)
	if !known || index != len(ops)-1 || ops[index].Instr.OpCode != int(bridgeReturnOpcode("()"+result)) || len(ops[index].Data) != 0 {
		return nil
	}
	if _, _, known := c.nativeOriginalMethodSnapshot(method, code); !known || c.Work.CheckAlloc(int64(len(code.Code))+int64(len(parameters))*32+256) != nil {
		return nil
	}
	return &lambdaVarargsAdapter{object: c.obj, method: method, code: code, codeBytes: bytes.Clone(code.Code), member: *member, opcode: core.OP_INVOKESPECIAL, parameters: parameters, result: result}
}

// This domain is an ordinary instance factory capturing its initialized entry
// receiver. Neither a same-spelled local nor a verifier-compatible object may
// borrow implicit super binding. Recheck both original SSA provenance and the
// writer's pending/completed receiver witness; no textual "this" inference.
func (c *ClassObjectDumper) compilerSuperReceiverPort(f *lambdaSourceFactory, value *values.JavaRef, ctx *class_context.ClassContext, completed bool) bool {
	if c == nil || c.obj == nil || !f.closed() || !f.sealed || f.object != c.obj || f.method.AccessFlags&8 != 0 || isSyntheticMethod(f.method.AccessFlags) || ctx == nil || ctx.IsStatic || strings.ReplaceAll(ctx.ClassName, ".", "/") != c.obj.GetClassName() {
		return false
	}
	name, nk := sourceBridgeUTF8(c.obj, f.method.NameIndex)
	desc, dk := sourceBridgeUTF8(c.obj, f.method.DescriptorIndex)
	if !nk || !dk || name == "<clinit>" || ctx.FunctionName != name || ctx.CurrentMethodDesc != desc {
		return false
	}
	_, frames, known := c.nativeOriginalMethodSnapshot(f.method, f.code)
	if !known {
		return false
	}
	found := false
	for _, record := range frames.Instructions {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if int(record.PC) != f.pc {
			continue
		}
		stack := record.Before.Stack
		if found || record.Before.ThisUninitialized || len(record.Before.Locals) == 0 || record.Before.Locals[0].Kind != frametransfer.Ref || record.Before.Locals[0].Class != c.obj.GetClassName() || len(record.Uses) != 1 || record.Uses[0] != (ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: 0}) || len(stack) == 0 || stack[len(stack)-1].Kind != frametransfer.Ref || stack[len(stack)-1].Class != c.obj.GetClassName() {
			return false
		}
		found = true
	}
	p := &lambdaParameterMetadata{object: c.obj, receiverWords: 1, factory: f, receiver: &nativeEnumSelectorProducer{slot: 0, result: "L" + c.obj.GetClassName() + ";"}}
	return found && (lambdaParameterReceiverSourceClosed(p, value, ctx, c.Work) || !completed && lambdaParameterReceiverPendingClosed(p, value, ctx, c.Work))
}

func (c *ClassObjectDumper) compilerSuperReferenceThrows(site compilerReferenceSite, selected callbinding.Method, declaring string, provider callbinding.Provider) bool {
	resolve := c.nativeAnnotationDeclarationResolver()
	parent, ok := resolve(declaring)
	if !ok || parent == nil || !selected.ExceptionsKnown || !selected.AccessFlagsKnown || selected.AccessFlags&0x140a != 0 || selected.AccessFlags&1 == 0 || !constructorBoundOriginalSignature(parent, selected.Name, selected.Desc, selected.Signature, c.Work) {
		return false
	}
	var target *MemberInfo
	for _, m := range parent.Methods {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		n, _ := sourceBridgeUTF8(parent, m.NameIndex)
		d, _ := sourceBridgeUTF8(parent, m.DescriptorIndex)
		if n == selected.Name && d == selected.Desc {
			if target != nil || m.AccessFlags != selected.AccessFlags {
				return false
			}
			target = m
		}
	}
	if target == nil {
		return false
	}
	parentReader := NewClassObjectDumper(parent)
	parentReader.Work = c.Work
	exceptions, known := parentReader.constructorThrowsOriginalExceptions(target)
	if !known || !slices.Equal(exceptions, selected.Exceptions) {
		return false
	}
	fi, known := provider(site.functional[1 : len(site.functional)-1])
	if !known || !fi.MembersComplete {
		return false
	}
	var sam *callbinding.Method
	for i := range fi.Methods {
		m := &fi.Methods[i]
		if m.Name == site.samName && m.Desc == site.erased && !m.Static && !m.Bridge {
			if sam != nil || !m.ExceptionsKnown || strings.Contains(m.Signature, "^T") {
				return false
			}
			sam = m
		}
	}
	if sam == nil || len(sam.Exceptions) > 128 || len(exceptions) > 128 {
		return false
	}
	functional, known := resolve(fi.Name)
	if !known || functional == nil || !constructorBoundOriginalSignature(functional, sam.Name, sam.Desc, sam.Signature, c.Work) {
		return false
	}
	fiReader := NewClassObjectDumper(functional)
	fiReader.Work = c.Work
	matched := false
	for _, m := range functional.Methods {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		n, _ := sourceBridgeUTF8(functional, m.NameIndex)
		d, _ := sourceBridgeUTF8(functional, m.DescriptorIndex)
		if n == sam.Name && d == sam.Desc {
			declared, valid := fiReader.constructorThrowsOriginalExceptions(m)
			if matched || !valid || !slices.Equal(declared, sam.Exceptions) {
				return false
			}
			matched = true
		}
	}
	if !matched {
		return false
	}
	remaining := 1 << 16
	bounded := func(name string) (callbinding.Class, bool) {
		remaining--
		if remaining < 0 || !nativeProofWork(c.Work, int64(len(name))+1) {
			return callbinding.Class{}, false
		}
		return provider(name)
	}
	for _, exception := range exceptions {
		covered := false
		for _, allowed := range append(slices.Clone(sam.Exceptions), "java/lang/RuntimeException", "java/lang/Error") {
			if !nativeProofWork(c.Work, 1) {
				return false
			}
			covered = covered || callbinding.Assignable("L"+exception+";", "L"+allowed+";", bounded)
		}
		if !covered {
			return false
		}
	}
	return true
}

func (c *ClassObjectDumper) proveCompilerSuperReference(name, descriptor string, captured []values.JavaValue, request methodSourceRequest, ctx *class_context.ClassContext) *lambdaCompilerReference {
	if c == nil || c.obj == nil || ctx == nil || ctx.InvocationMetadata == nil || c.options.SourceCompiler.effective() != ModernJavac && c.options.SourceCompiler.effective() != NativeJavac8 || core.EffectiveTargetSource(c.options.TargetSourceVersion, c.obj.MajorVersion) != 8 || len(captured) != 1 || !request.valid() || request.captureCount != 1 || !c.compilerReferenceConcreteFactoryScope(request.factory) || !nativeMemberTopLevelEvidence(c.obj, c.Work) {
		return nil
	}
	f := request.factory
	site, known := c.originalCompilerReferenceSiteKind(f, 7)
	if !known || site.captureCount != 1 || site.captured != "L"+c.obj.GetClassName()+";" || site.implementation.Member != name || site.implementation.Description != descriptor {
		return nil
	}
	var method *MemberInfo
	for _, m := range c.obj.Methods {
		if !nativeProofWork(c.Work, 1) || m == nil {
			return nil
		}
		n, _ := sourceBridgeUTF8(c.obj, m.NameIndex)
		d, _ := sourceBridgeUTF8(c.obj, m.DescriptorIndex)
		if n == name && d == descriptor {
			if method != nil {
				return nil
			}
			method = m
		}
	}
	shape := c.originalSuperReferenceShape(method)
	if shape == nil {
		return nil
	}
	receiver, ok := values.UnpackSoltValue(captured[0]).(*values.JavaRef)
	if !ok || !c.compilerSuperReceiverPort(f, receiver, ctx, false) {
		return nil
	}
	plan := c.compilerSuperReferenceSelection(shape, site, ctx)
	_, result, err := callbinding.Descriptor(site.actual)
	if !plan.Supported || !slices.Equal(plan.ExpandedParameters, shape.parameters) || err != nil || result != shape.result || !c.compilerSuperReferenceThrows(site, plan.Selected, plan.DeclaringOwner, ctx.InvocationMetadata) {
		return nil
	}
	target := c.compilerReferenceFunctionalTarget(site, ctx)
	component, known := lambdaFactoryNameComponent(f)
	suffix, prefixed := strings.CutPrefix(name, "lambda$"+component+"$")
	ordinal, err := strconv.Atoi(suffix)
	if target == nil || !known || !prefixed || err != nil || ordinal < 0 || ordinal >= len(c.obj.Methods) || suffix != strconv.Itoa(ordinal) {
		return nil
	}
	return &lambdaCompilerReference{superReceiver: true, originalReceiver: receiver, factory: f, shape: shape, receiver: receiver, context: ctx, target: target, erased: site.erased, instantiated: site.actual, template: "super::" + shape.member.Member, maxStack: shape.code.MaxStack, maxLocals: shape.code.MaxLocals, factoryStack: f.code.MaxStack, factoryLocals: f.code.MaxLocals}
}
