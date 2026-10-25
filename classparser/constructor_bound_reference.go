package javaclassparser

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A bound reference checks its receiver when the reference is created, before
// later constructor arguments run. javac makes that check explicit in bytecode.
// Recreating it as a separate statement needs a constructor-prefix helper and
// changes the declared member family. This source-only projection consumes the
// exact compiler check together with its original metafactory site. It does not
// move an arbitrary prefix, trust a source callback, or change the typed IR.
func (c *ClassObjectDumper) constructorBoundReferenceSource(code *CodeAttribute, body []statements.Statement, params []values.JavaValue, method *MemberInfo) []statements.Statement {
	if projected, known := c.constructorBoundReferencePortListSource(code, body, params, method); known {
		return projected
	}
	if c == nil || c.obj == nil || c.FuncCtx == nil || method == nil || code == nil || c.isGenuineEnum() {
		return body
	}
	name, nk := sourceBridgeUTF8(c.obj, method.NameIndex)
	descriptor, dk := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	if !nk || !dk || name != "<init>" || method.AccessFlags&8 != 0 {
		return body
	}
	physical, _, err := callbinding.Descriptor(descriptor)
	if err != nil || len(params) != len(physical)+1 {
		return body
	}
	this, ok := params[0].(*values.JavaRef)
	if !ok {
		return body
	}
	if _, known := this.OriginalReceiverSlot(); !known {
		return body
	}
	indices := []int{}
	for i, st := range body {
		if middle, ok := st.(*statements.MiddleStatement); ok && middle != nil && middle.Data == nil {
			continue
		}
		indices = append(indices, i)
		if len(indices) == 2 {
			break
		}
	}
	if len(indices) != 2 {
		return body
	}
	checkStatement, ck := body[indices[0]].(*statements.ExpressionStatement)
	delegateStatement, dd := body[indices[1]].(*statements.ExpressionStatement)
	if !ck || !dd || checkStatement == nil || delegateStatement == nil {
		return body
	}
	check, ck := values.UnpackSoltValue(checkStatement.Expression).(*values.FunctionCallExpression)
	delegate, dd := values.UnpackSoltValue(delegateStatement.Expression).(*values.FunctionCallExpression)
	if !ck || !dd || check == nil || delegate == nil || delegate.FunctionName != "<init>" || delegate.Kind != values.InvokeSpecial || values.UnpackSoltValue(delegate.Object) != this || len(delegate.Arguments) == 0 {
		return body
	}
	owner := strings.ReplaceAll(delegate.ClassName, ".", "/")
	if owner != c.obj.GetClassName() && owner != c.obj.GetSupperClassName() {
		return body
	}
	factory, ok := values.UnpackSoltValue(delegate.Arguments[0]).(*values.CustomValue)
	if !ok || factory == nil || !factory.IsMethodRef {
		return body
	}
	pc, captures, known := factory.OriginalLambdaFactoryEvaluation()
	if !known || len(captures) != 1 {
		return body
	}
	receiver, ok := values.UnpackSoltValue(captures[0]).(*values.JavaRef)
	if !ok || receiver == nil || receiver.Id == nil {
		return body
	}
	slot, known := receiver.OriginalParameterSlot()
	parameterIndex, present := constructorParameterSlots(physical)[slot]
	if !known || !present || params[parameterIndex+1] != receiver || !strings.HasPrefix(physical[parameterIndex], "L") {
		return body
	}
	if erased, known := values.SourceTypeErasure(receiver.Type(), c.FuncCtx); !known || erased != physical[parameterIndex] {
		return body
	}
	decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(c.ConstantPool, index) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil || !constructorPreludeOutsideHandlers(code, delegate.OriginPC, decoder, c.Work) || !constructorOriginalInvoke(decoder, delegate, func(index int) values.JavaValue { return GetValueFromCP(c.ConstantPool, index) }) || !constructorOriginalInvoke(decoder, check, func(index int) values.JavaValue { return GetValueFromCP(c.ConstantPool, index) }) {
		return body
	}
	if !c.constructorReferenceFactoryPort(method, code, pc, delegate, slot) {
		return body
	}
	ops := constructorMotionOps(decoder)
	if len(ops) < 7 || core.GetRetrieveIdx(ops[0]) != 0 || !constructorMotionLoad(ops[0], "Ljava/lang/Object;") || core.GetRetrieveIdx(ops[1]) != slot || !constructorMotionLoad(ops[1], physical[parameterIndex]) || ops[2].Instr.OpCode != core.OP_DUP || !nativeMemberNullCheck(c.obj, ops[3]) || ops[4].Instr.OpCode != core.OP_POP || ops[5].Instr.OpCode != core.OP_INVOKEDYNAMIC || int(ops[3].CurrentOffset) != check.OriginPC || int(ops[5].CurrentOffset) != pc || pc >= delegate.OriginPC {
		return body
	}
	// JLS 15.13.3 requires this eager receiver NPE for a bound reference,
	// independently of the compiler selected for the new source. The supported
	// JDK compilers spell it as either final Object.getClass or the one-argument
	// platform Objects.requireNonNull intrinsic. Neither calls user code; the
	// discarded result cannot escape the exact DUP/check/POP/factory packet.
	// Match the ORIGINAL opcode and receiver, never the destination profile.
	if c.options.SourceCompiler != NativeJavac8 && c.options.SourceCompiler != ModernJavac {
		return body
	}
	if ops[3].Instr.OpCode == core.OP_INVOKEVIRTUAL {
		if check.Kind != values.InvokeVirtual || len(check.Arguments) != 0 || values.UnpackSoltValue(check.Object) != receiver {
			return body
		}
	} else if ops[3].Instr.OpCode == core.OP_INVOKESTATIC {
		if check.Kind != values.InvokeStatic || len(check.Arguments) != 1 || values.UnpackSoltValue(check.Arguments[0]) != receiver {
			return body
		}
	} else {
		return body
	}
	target, member, instantiated, proved := c.constructorBoundReferenceTarget(code, ops[5], owner, delegate.Descriptor, physical[parameterIndex])
	var compilerBody *dumpedMethods
	if !proved {
		target, member, instantiated, compilerBody, proved = c.constructorBoundCompilerReferenceTarget(code, pc, owner, delegate.Descriptor, receiver)
	}
	if !proved {
		return body
	}
	value := c.constructorBoundReferenceValue(receiver, target, member, instantiated, pc, compilerBody)
	copyCall := *delegate
	copyCall.Arguments = append([]values.JavaValue(nil), delegate.Arguments...)
	copyCall.Arguments[0] = value
	copyStatement := *delegateStatement
	copyStatement.Expression = &copyCall
	result := append([]statements.Statement(nil), body...)
	result[indices[1]] = &copyStatement
	return append(result[:indices[0]], result[indices[0]+1:]...)
}

// Only a direct bound virtual/interface reference with an exact instantiated
// SAM/member descriptor is admitted. Varargs, generic methods, markers, bridges,
// overloaded source families, receiver adaptation and unknown declarations need
// their own binding proofs. All scans share the bounded original-work budget.
func (c *ClassObjectDumper) constructorBoundReferenceTarget(code *CodeAttribute, op *core.OpCode, delegateOwner, delegateDescriptor, receiverDescriptor string, argumentPort ...int) (types.JavaType, string, string, bool) {
	fail := func() (types.JavaType, string, string, bool) { return nil, "", "", false }
	port := 0
	if len(argumentPort) > 1 || len(argumentPort) == 1 && argumentPort[0] < 0 {
		return fail()
	}
	if len(argumentPort) == 1 {
		port = argumentPort[0]
	}
	if c == nil || c.FuncCtx == nil || c.FuncCtx.InvocationMetadata == nil {
		return fail()
	}
	remaining := 4096
	siteDescriptor, valid := c.constructorReceiverDynamicDescriptor(c.obj, code, op, &remaining)
	if !valid {
		return fail()
	}
	captured, functional, err := callbinding.Descriptor(siteDescriptor)
	if err != nil || len(captured) != 1 || captured[0] != receiverDescriptor || !strings.HasPrefix(functional, "L") {
		return fail()
	}
	site := c.obj.ConstantPool[int(core.Convert2bytesToInt(op.Data[:2]))-1].(*ConstantInvokeDynamicInfo)
	nt := c.obj.ConstantPool[site.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
	samName, valid := sourceBridgeUTF8(c.obj, nt.NameIndex)
	if !valid {
		return fail()
	}
	var bootstrap *BootstrapMethod
	for _, attr := range c.obj.Attributes {
		if table, ok := attr.(*BootstrapMethodsAttribute); ok {
			bootstrap = table.BootstrapMethods[site.BootstrapMethodAttrIndex]
		}
	}
	if bootstrap == nil || len(bootstrap.BootstrapArguments) != 3 {
		return fail()
	}
	handle := c.obj.ConstantPool[bootstrap.BootstrapMethodRef-1].(*ConstantMethodHandleInfo)
	bootMember := constructorMotionMember(c.obj, &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_INVOKESTATIC}, Data: []byte{byte(handle.ReferenceIndex >> 8), byte(handle.ReferenceIndex)}}, core.OP_INVOKESTATIC)
	if handle.ReferenceKind != 6 || bootMember == nil || bootMember.Name != "java/lang/invoke/LambdaMetafactory" || bootMember.Member != "metafactory" || bootMember.Description != "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodHandle;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;" {
		return fail()
	}
	methodType := func(index uint16) (string, bool) {
		entry, ok := c.obj.ConstantPool[index-1].(*ConstantMethodTypeInfo)
		if !ok || entry == nil {
			return "", false
		}
		return sourceBridgeUTF8(c.obj, entry.DescriptorIndex)
	}
	erased, ek := methodType(bootstrap.BootstrapArguments[0])
	actual, ak := methodType(bootstrap.BootstrapArguments[2])
	impl, ik := c.obj.ConstantPool[bootstrap.BootstrapArguments[1]-1].(*ConstantMethodHandleInfo)
	if !ek || !ak || !ik || impl == nil || impl.ReferenceKind != 5 && impl.ReferenceKind != 9 {
		return fail()
	}
	opcode := core.OP_INVOKEVIRTUAL
	if impl.ReferenceKind == 9 {
		opcode = core.OP_INVOKEINTERFACE
	}
	data := []byte{byte(impl.ReferenceIndex >> 8), byte(impl.ReferenceIndex)}
	if opcode == core.OP_INVOKEINTERFACE {
		data = append(data, 1, 0)
	}
	member := constructorMotionMember(c.obj, &core.OpCode{Instr: &core.Instruction{OpCode: opcode}, Data: data}, opcode)
	if member == nil || member.Description != actual || receiverDescriptor != "L"+member.Name+";" || class_context.SafeIdentifier(member.Member) != member.Member || strings.ContainsAny(member.Member, "<>\x00") {
		return fail()
	}
	provider := c.FuncCtx.InvocationMetadata
	declarations := func(name string) (callbinding.Class, bool) {
		remaining--
		if remaining < 0 || !nativeProofWork(c.Work, 1) {
			return callbinding.Class{}, false
		}
		declaration, known := provider(name)
		if !known || len(declaration.Methods) > 1024 || len(declaration.Parents) > 64 || !nativeProofWork(c.Work, int64(len(declaration.Methods)+len(declaration.Parents))) {
			return callbinding.Class{}, false
		}
		remaining -= len(declaration.Methods) + len(declaration.Parents)
		return declaration, remaining >= 0
	}
	memberOwner, known := declarations(member.Name)
	if !known || memberOwner.Name != member.Name || memberOwner.IsInterface != (impl.ReferenceKind == 9) || len(types.ClassFormalTypeParamNames(memberOwner.Signature)) != 0 {
		return fail()
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{Owner: member.Name, Name: member.Member, Desc: actual}, declarations)
	if err != nil || !family.Complete || family.Proof != callbinding.Unique || family.Target == nil || len(family.Methods) > 64 {
		return fail()
	}
	for _, declaration := range family.Methods {
		if declaration.Desc != actual || declaration.Generic || declaration.Varargs || declaration.Bridge || declaration.Static || !nativeProofWork(c.Work, 1) {
			return fail()
		}
	}
	if !family.Target.Public {
		original, ok := c.resolveBridgeClass(member.Name)
		if !ok || nativeBinaryPackage(member.Name) != nativeBinaryPackage(c.obj.GetClassName()) {
			return fail()
		}
		found := false
		for _, method := range original.Methods {
			if !nativeProofWork(c.Work, 1) || method == nil {
				return fail()
			}
			n, nk := sourceBridgeUTF8(original, method.NameIndex)
			d, dk := sourceBridgeUTF8(original, method.DescriptorIndex)
			if nk && dk && n == member.Member && d == actual && method.AccessFlags&0x000a == 0 {
				found = true
			}
		}
		if !found {
			return fail()
		}
	}
	fiName := functional[1 : len(functional)-1]
	fi, ok := declarations(fiName)
	if !ok || fi.Name != fiName || !fi.IsInterface || !fi.MembersComplete || !fi.ParentsComplete || len(fi.Methods) > 256 {
		return fail()
	}
	samSignature, samFound := "", false
	for _, method := range fi.Methods {
		if !nativeProofWork(c.Work, 1) {
			return fail()
		}
		// Generic here also marks a method that merely uses its interface's
		// class formals. The descriptor/Signature projection below binds those
		// variables; an unrelated method formal remains unsupported there.
		if method.Name == samName && method.Desc == erased && !method.Static && !method.Varargs && !method.Bridge {
			if samFound {
				return fail()
			}
			samSignature, samFound = method.Signature, true
		}
	}
	if !samFound {
		return fail()
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	if !nativeLambdaFunctionalTarget(functional, samName, erased, resolve, c.Work) {
		return fail()
	}
	fiOriginal, originalKnown := resolve(fiName)
	if !originalKnown {
		return fail()
	}
	classSignature, signatureKnown := nativeMethodLocalOriginalSignature(fiOriginal, fiOriginal.Attributes, c.Work)
	if !signatureKnown || classSignature != fi.Signature || !constructorBoundOriginalSignature(fiOriginal, samName, erased, samSignature, c.Work) {
		return fail()
	}
	target := core.ProjectOriginalSamTarget(fiName, fi.Signature, samSignature, erased, actual, declarations)
	if target == nil || !constructorStaticSourceType(target, c.FuncCtx) {
		return fail()
	}
	// The cast fixes the same instantiated poly target as the original selected
	// constructor's Signature. A raw/foreign parameter cannot borrow this proof.
	owner, ok := declarations(delegateOwner)
	if !ok || owner.Name != delegateOwner || !owner.MembersComplete || len(owner.Methods) > 4096 {
		return fail()
	}
	matched := false
	for _, method := range owner.Methods {
		if !nativeProofWork(c.Work, 1) {
			return fail()
		}
		if method.Name != "<init>" || method.Desc != delegateDescriptor {
			continue
		}
		// Absence of Signature on a monomorphic declaration is an identity
		// projection of its descriptor, not missing type evidence. Validate the
		// actual original absence before using that descriptor as a source view.
		sourceSignature := method.Signature
		if sourceSignature == "" {
			sourceSignature = method.Desc
		}
		original, originalKnown := resolve(delegateOwner)
		if !originalKnown {
			return fail()
		}
		ownerSignature, ownerKnown := nativeMethodLocalOriginalSignature(original, original.Attributes, c.Work)
		if !ownerKnown || ownerSignature != owner.Signature || ownerSignature != "" && !nativeOriginalClassSignatureSupersClosed(original, ownerSignature, c.Work) {
			return fail()
		}
		physical, _, exact := types.EraseLexicalMethodSignatureWithThrows(ownerSignature, sourceSignature)
		if !exact || physical != delegateDescriptor || !originalKnown || !constructorBoundOriginalSignature(original, "<init>", delegateDescriptor, method.Signature, c.Work) {
			return fail()
		}
		formals, parameters, result := types.ParseMethodSignatureFull(sourceSignature, c.FuncCtx)
		if matched || formals != "" || result == nil || port >= len(parameters) || parameters[port] == nil || !reflect.DeepEqual(parameters[port].RawType(), target.RawType()) {
			return fail()
		}
		matched = true
	}
	if !matched {
		return fail()
	}
	return target, member.Member, actual, true
}

func constructorBoundOriginalSignature(obj *ClassObject, name, descriptor, signature string, work *workbudget.Budget) bool {
	if obj == nil || len(obj.Methods) > 4096 {
		return false
	}
	found := false
	for _, method := range obj.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return false
		}
		n, nk := sourceBridgeUTF8(obj, method.NameIndex)
		d, dk := sourceBridgeUTF8(obj, method.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		if n != name || d != descriptor {
			continue
		}
		s, valid := nativeMethodLocalOriginalSignature(obj, method.Attributes, work)
		if found || !valid || s != signature {
			return false
		}
		found = true
	}
	return found
}

func (c *ClassObjectDumper) constructorBoundReferenceValue(receiver *values.JavaRef, target types.JavaType, member, instantiated string, pc int, compilerBody *dumpedMethods) *values.CustomValue {
	// Build the source from original declarations and the checked receiver. The
	// old CustomValue supplies eager identity only; its renderer never runs here.
	value := values.NewStreamingCustomValue(func(ctx *class_context.ClassContext, out *workbudget.Writer) error {
		start := out.Len()
		receiverText := receiver.String(ctx)
		if err := out.WriteString("(" + target.String(ctx) + ") (" + receiverText + "::" + class_context.SafeIdentifier(member) + ")"); err != nil {
			return err
		}
		if compilerBody != nil {
			packet := out.String()[start:]
			c.recordCompilerReferenceCapture(compilerBody, core.LambdaSourceRender{Packet: packet, Captures: []core.LambdaCaptureRender{{Index: 0, Text: receiverText}}, Complete: true})
			c.recordNativeLambdaRenderedPacket(compilerBody, compilerBody.code, packet)
			c.recordLambdaMethodEmission(compilerBody, compilerBody.code, packet)
		}
		return nil
	}, func() types.JavaType { return target })
	value.Flag, value.IsMethodRef, value.NoOuterCapture, value.CapturesKnown = "lambda", true, true, true
	value.OriginPC, value.HasOriginPC, value.InstantiatedMtdDesc = pc, true, instantiated
	value.Captures = []values.JavaValue{receiver}
	value.MarkOriginalLambdaFactory(pc)
	return value
}
