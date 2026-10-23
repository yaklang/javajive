package javaclassparser

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A compiler reference is a third source representation, distinct from an
// ordinary method declaration and from an arrow. Its physical receiver/fixed/
// packed ports come from verified original Code. Compiler names constrain only
// regeneration after binding, factory and source-operand proofs have closed.
// The receiver must retain either its initialized entry parameter or a closed
// typed local definition frontier. Exact physical SAM ports, no checked target
// exceptions and a flat factory scope are required. Unknown/adapted receivers
// retain the existing recovery path.
type compilerReferenceCaptureEmission struct {
	frame       *lambdaMethodEmission
	context     *class_context.ClassContext
	parameter   *values.JavaRef
	localSource *nativeLambdaLocalCaptureSource
	text        string
}

type lambdaCompilerReference struct {
	static                                           bool
	qualifier                                        string
	parameter                                        *values.JavaRef
	local                                            *nativeEnumSelectorProducer
	captureEmission                                  *compilerReferenceCaptureEmission
	originalReceiver                                 *values.JavaRef
	factory                                          *lambdaSourceFactory
	shape                                            *lambdaVarargsAdapter
	receiver                                         *values.JavaRef
	context                                          *class_context.ClassContext
	target                                           types.JavaType
	erased, instantiated, template                   string
	maxStack, maxLocals, factoryStack, factoryLocals uint16
}

type compilerReferenceSite struct {
	samName, captured, functional, erased, actual string
	captureCount                                  int
	implementation                                values.JavaClassMember
}

// Reopen the actual typed CP/bootstrap edge; callback labels never establish it.
func (c *ClassObjectDumper) originalCompilerReferenceSite(f *lambdaSourceFactory) (compilerReferenceSite, bool) {
	var out compilerReferenceSite
	if c == nil || c.obj == nil || !f.closed() || !f.sealed || f.object != c.obj || f.pc+5 > len(f.code.Code) || f.code.Code[f.pc] != byte(core.OP_INVOKEDYNAMIC) {
		return out, false
	}
	op := &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_INVOKEDYNAMIC}, CurrentOffset: uint16(f.pc), Data: f.code.Code[f.pc+1 : f.pc+5]}
	remaining := 4096
	desc, known := c.constructorReceiverDynamicDescriptor(c.obj, f.code, op, &remaining)
	if !known {
		return out, false
	}
	parameters, result, err := callbinding.Descriptor(desc)
	if err != nil || len(parameters) > 255 || !strings.HasPrefix(result, "L") {
		return out, false
	}
	site := c.obj.ConstantPool[core.Convert2bytesToInt(op.Data[:2])-1].(*ConstantInvokeDynamicInfo)
	nt := c.obj.ConstantPool[site.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
	out.samName, known = sourceBridgeUTF8(c.obj, nt.NameIndex)
	if !known {
		return out, false
	}
	var boot *BootstrapMethod
	for _, attribute := range c.obj.Attributes {
		if table, ok := attribute.(*BootstrapMethodsAttribute); ok {
			boot = table.BootstrapMethods[site.BootstrapMethodAttrIndex]
		}
	}
	if boot == nil || len(boot.BootstrapArguments) != 3 {
		return out, false
	}
	handle := c.obj.ConstantPool[boot.BootstrapMethodRef-1].(*ConstantMethodHandleInfo)
	member := constructorMotionMember(c.obj, &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_INVOKESTATIC}, Data: []byte{byte(handle.ReferenceIndex >> 8), byte(handle.ReferenceIndex)}}, core.OP_INVOKESTATIC)
	if handle.ReferenceKind != 6 || member == nil || member.Name != "java/lang/invoke/LambdaMetafactory" || member.Member != "metafactory" || member.Description != "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodHandle;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;" {
		return out, false
	}
	methodType := func(index uint16) (string, bool) {
		entry, ok := c.obj.ConstantPool[index-1].(*ConstantMethodTypeInfo)
		if !ok || entry == nil {
			return "", false
		}
		return sourceBridgeUTF8(c.obj, entry.DescriptorIndex)
	}
	var ek, ak bool
	out.erased, ek = methodType(boot.BootstrapArguments[0])
	out.actual, ak = methodType(boot.BootstrapArguments[2])
	impl, ik := c.obj.ConstantPool[boot.BootstrapArguments[1]-1].(*ConstantMethodHandleInfo)
	if !ek || !ak || !ik || impl == nil || impl.ReferenceKind != 6 {
		return out, false
	}
	implementation := constructorMotionMember(c.obj, &core.OpCode{Instr: &core.Instruction{OpCode: core.OP_INVOKESTATIC}, Data: []byte{byte(impl.ReferenceIndex >> 8), byte(impl.ReferenceIndex)}}, core.OP_INVOKESTATIC)
	if implementation == nil || implementation.Name != c.obj.GetClassName() {
		return out, false
	}
	out.captureCount = len(parameters)
	if len(parameters) == 1 {
		out.captured = parameters[0]
	}
	out.functional, out.implementation = result, *implementation
	return out, true
}

// Only the compiler's own adapter table is accepted. Port roles do not come
// from rec$/x$/xva$ spellings: those names are checked after the original pack.
func (c *ClassObjectDumper) compilerVarargsParameterSchema(shape *lambdaVarargsAdapter) bool {
	if c.options.TargetSourceVersion < 0 || core.EffectiveTargetSource(c.options.TargetSourceVersion, c.obj.MajorVersion) != 8 || c.obj.MajorVersion != 52 {
		return false
	}
	table, known := readLambdaParameterMetadata(c.obj, shape.method, c.Work)
	if !known {
		return false
	}
	switch c.options.SourceCompiler {
	case ModernJavac:
		return table == nil
	case NativeJavac8:
		if c.options.TargetSourceVersion != 8 {
			return false
		}
		if table == nil {
			return native8UnmarkedLambdaParametersClosed(c.obj, shape.method, c.Work)
		}
		if !native8LambdaParameterCompilerFlagClosed(c.obj, c.Work) {
			return false
		}
		formals, _, err := callbinding.Descriptor(shape.member.Description)
		if err != nil {
			return false
		}
		fixed := len(formals) - 1
		names := []string{"rec$"}
		for i := 1; i < len(shape.parameters); i++ {
			prefix := "x$"
			if i > fixed {
				prefix = "xva$"
			}
			names = append(names, prefix+strconv.Itoa(i-1))
		}
		if !slices.Equal(table.names, names) || len(table.flags) != len(names) {
			return false
		}
		for _, flags := range table.flags {
			if flags != 0x1000 {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// The fixed public binding kernel can also query a positively proved package
// access view. A nonpublic declaration is promoted only after reopening its
// original flags in this exact source package; private/foreign declarations
// remain unknown. This grants no lexical nesting or protected receiver rights.
func (c *ClassObjectDumper) compilerReferenceAccessProvider(provider callbinding.Provider) callbinding.Provider {
	return func(name string) (callbinding.Class, bool) {
		cl, known := provider(name)
		if !known || cl.Name != name || len(cl.Methods) > 4096 || len(cl.Parents) > 64 || !nativeProofWork(c.Work, int64(len(cl.Methods)+len(cl.Parents))+1) {
			return cl, false
		}
		if cl.Public {
			all := true
			for _, m := range cl.Methods {
				all = all && m.Public
			}
			if all {
				return cl, true
			}
		}
		original, ok := c.resolveBridgeClass(name)
		if !ok || original == nil || nativeBinaryPackage(name) != nativeBinaryPackage(c.obj.GetClassName()) || len(original.Methods) > 4096 {
			return cl, true
		}
		for _, attribute := range original.Attributes {
			if table, ok := attribute.(*InnerClassesAttribute); ok {
				if table == nil {
					return cl, false
				}
				for _, row := range table.Classes {
					if row == nil || !nativeProofWork(c.Work, 1) {
						return cl, false
					}
					self, sk := sourceBridgeClassName(original, row.InnerClassInfoIndex)
					if sk && self == name && row.InnerClassAccessFlags&2 != 0 {
						return cl, true
					}
				}
			}
		}
		if original.AccessFlags&2 != 0 {
			return cl, true
		}
		if c.Work != nil && c.Work.CheckAlloc(int64(len(cl.Methods))*128) != nil {
			return cl, false
		}
		cl.Public = true
		cl.Methods = slices.Clone(cl.Methods)
		for i, m := range cl.Methods {
			if m.Public {
				continue
			}
			matches := 0
			for _, raw := range original.Methods {
				if raw == nil || !nativeProofWork(c.Work, 1) {
					return cl, false
				}
				n, nk := sourceBridgeUTF8(original, raw.NameIndex)
				d, dk := sourceBridgeUTF8(original, raw.DescriptorIndex)
				if !nk || !dk {
					return cl, false
				}
				if n != m.Name || d != m.Desc {
					continue
				}
				sig, sk := nativeMethodLocalOriginalSignature(original, raw.Attributes, c.Work)
				if !sk || sig != m.Signature || raw.AccessFlags&2 != 0 || raw.AccessFlags&8 != 0 != m.Static || raw.AccessFlags&0x80 != 0 != m.Varargs || raw.AccessFlags&0x40 != 0 != m.Bridge {
					return cl, true
				}
				matches++
			}
			if matches == 1 {
				cl.Methods[i].Public = true
			}
		}
		return cl, true
	}
}

func (c *ClassObjectDumper) compilerReferenceFunctionalTarget(site compilerReferenceSite, ctx *class_context.ClassContext) types.JavaType {
	if ctx == nil || ctx.InvocationMetadata == nil {
		return nil
	}
	provider := ctx.InvocationMetadata
	remaining := 4096
	declarations := func(name string) (callbinding.Class, bool) {
		remaining--
		if remaining < 0 || !nativeProofWork(c.Work, 1) {
			return callbinding.Class{}, false
		}
		return provider(name)
	}
	fiName := site.functional[1 : len(site.functional)-1]
	fi, ok := declarations(fiName)
	if !ok || fi.Name != fiName || !fi.IsInterface || !fi.MembersComplete || !fi.ParentsComplete || len(fi.Methods) > 256 {
		return nil
	}
	signature := ""
	count := 0
	for _, m := range fi.Methods {
		if !nativeProofWork(c.Work, 1) {
			return nil
		}
		if m.Name == site.samName && m.Desc == site.erased && !m.Static && !m.Varargs && !m.Bridge {
			count++
			signature = m.Signature
		}
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	original, ok := resolve(fiName)
	if count != 1 || !ok || !nativeLambdaFunctionalTarget(site.functional, site.samName, site.erased, resolve, c.Work) {
		return nil
	}
	originalSignature, known := nativeMethodLocalOriginalSignature(original, original.Attributes, c.Work)
	if !known || originalSignature != fi.Signature || !constructorBoundOriginalSignature(original, site.samName, site.erased, signature, c.Work) {
		return nil
	}
	target := core.ProjectOriginalSamTarget(fiName, fi.Signature, signature, site.erased, site.actual, declarations)
	if target == nil || !constructorStaticSourceType(target, ctx) {
		return nil
	}
	return target
}

func (c *ClassObjectDumper) compilerReferenceParameterPort(f *lambdaSourceFactory, shape *lambdaVarargsAdapter, receiver *values.JavaRef, descriptor string, ctx *class_context.ClassContext) bool {
	if receiver == nil || receiver.Id == nil || ctx == nil || strings.ReplaceAll(ctx.ClassName, ".", "/") != c.obj.GetClassName() {
		return false
	}
	parameter, _, known := compilerReferenceParameterSource(receiver, f, ctx, c.Work)
	if known {
		name, concrete := types.RawClassFQN(parameter.Type())
		_, variable := types.TypeVariableName(parameter.Type())
		_, parameterized := types.AsParameterizedType(parameter.Type())
		known = concrete && !variable && !parameterized && "L"+strings.ReplaceAll(name, ".", "/")+";" == descriptor
	}
	if !known || !parameter.IsParam || parameter.IsThis {
		return false
	}
	slot, known := parameter.OriginalParameterSlot()
	if !known || !nativeMethodLocalParameterOperand(parameter, slot, descriptor, ctx, c.Work) {
		return false
	}
	parentName, nk := sourceBridgeUTF8(c.obj, f.method.NameIndex)
	parentDesc, dk := sourceBridgeUTF8(c.obj, f.method.DescriptorIndex)
	if !nk || !dk || ctx.FunctionName != parentName || ctx.CurrentMethodDesc != parentDesc || ctx.IsStatic != (f.method.AccessFlags&8 != 0) {
		return false
	}
	owned, codes := 0, 0
	for _, m := range c.obj.Methods {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if m == f.method {
			owned++
		}
	}
	for _, attribute := range f.method.Attributes {
		if code, ok := attribute.(*CodeAttribute); ok {
			if code != f.code {
				return false
			}
			codes++
		}
	}
	if owned != 1 || codes != 1 {
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
		if found || len(record.Uses) != 1 || record.Uses[0] != (ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: slot}) || len(record.Before.Stack) == 0 || record.Before.Stack[len(record.Before.Stack)-1].Kind != frametransfer.Ref {
			return false
		}
		found = true
	}
	if !found {
		return false
	}
	// A captured arrow may have the identical pack body while deliberately
	// allowing a null receiver until invocation. Only an original eager check
	// can authorize :: at creation; descriptor/flags/names cannot supply it.
	decoder := core.NewDecompiler(f.code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return false
	}
	ops := constructorMotionOps(decoder)
	for i, op := range ops {
		if int(op.CurrentOffset) == f.pc {
			load, known := nativeBoundAdapterCaptureLoad(c.obj, f.method, shape.method, f.code, ops, i, []string{descriptor}, c.Work)
			return known && core.GetRetrieveIdx(ops[load]) == slot
		}
	}
	return false
}

func (c *ClassObjectDumper) proveCompilerVarargsReference(name, descriptor string, captured []values.JavaValue, request methodSourceRequest, ctx *class_context.ClassContext) *lambdaCompilerReference {
	if len(captured) == 0 {
		return c.proveCompilerStaticVarargsReference(name, descriptor, request, ctx)
	}
	if c == nil || c.obj == nil || ctx == nil || ctx.InvocationMetadata == nil || len(captured) != 1 || !request.valid() || request.captureCount != 1 || !request.factory.closed() || !request.factory.sealed || isSyntheticMethod(request.factory.method.AccessFlags) {
		return nil
	}
	f := request.factory
	if !c.compilerReferenceConcreteFactoryScope(f) {
		return nil
	}
	site, known := c.originalCompilerReferenceSite(f)
	if !known || site.captureCount != 1 || !strings.HasPrefix(site.captured, "L") || site.implementation.Member != name || site.implementation.Description != descriptor {
		return nil
	}
	var method *MemberInfo
	for _, m := range c.obj.Methods {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return nil
		}
		n, nk := sourceBridgeUTF8(c.obj, m.NameIndex)
		d, dk := sourceBridgeUTF8(c.obj, m.DescriptorIndex)
		if !nk || !dk {
			return nil
		}
		if n == name && d == descriptor {
			if method != nil {
				return nil
			}
			method = m
		}
	}
	shape, known := c.originalVarargsAdapterShape(method)
	if !known || shape.receiverCount() != 1 || len(shape.parameters) == 0 || shape.parameters[0] != site.captured || !c.compilerVarargsParameterSchema(shape) {
		return nil
	}
	actualParameters, actualResult, err := callbinding.Descriptor(site.actual)
	if err != nil || len(actualParameters) != len(shape.parameters)-1 || actualResult != shape.result {
		return nil
	}
	receiver, ok := values.UnpackSoltValue(captured[0]).(*values.JavaRef)
	if !ok {
		return nil
	}
	var local *nativeEnumSelectorProducer
	if !c.compilerReferenceParameterPort(f, shape, receiver, site.captured, ctx) {
		local = c.compilerReferenceLocalPort(f, shape, receiver, site.captured, ctx)
		if local == nil {
			return nil
		}
	}
	kind := callbinding.Virtual
	if shape.opcode == core.OP_INVOKEINTERFACE {
		kind = callbinding.Interface
	}
	plan := callbinding.SelectVariableArityReference(callbinding.Witness{Owner: shape.member.Name, Name: shape.member.Member, Desc: shape.member.Description, Kind: kind}, site.actual, c.compilerReferenceAccessProvider(ctx.InvocationMetadata), c.Work)
	// Javac's reference adapter takes the selected declaration's erased fixed
	// and expanded component ports, not necessarily the instantiated SAM types.
	// The binding certificate proves each invocation conversion; the independently
	// verified original pack must consume exactly those physical ports. Writing
	// :: leaves those conversions in the metafactory's invocation phase, so a
	// nullable boxed input is not evaluated/unboxed at factory creation.
	if !plan.Supported || !slices.Equal(plan.ExpandedParameters, shape.parameters[1:]) || !c.compilerReferenceOriginalAccess(shape, plan) || !plan.Selected.ExceptionsKnown || len(plan.Selected.Exceptions) != 0 || !class_context.PreservesJava8Identifier(shape.member.Member) {
		return nil
	}
	target := c.compilerReferenceFunctionalTarget(site, ctx)
	if target == nil {
		return nil
	}
	component, known := lambdaFactoryNameComponent(f)
	suffix, prefix := strings.CutPrefix(name, "lambda$"+component+"$")
	ordinal, err := strconv.Atoi(suffix)
	if !known || !prefix || err != nil || ordinal < 0 || ordinal >= len(c.obj.Methods) || suffix != strconv.Itoa(ordinal) {
		return nil
	}
	if c.Work != nil && c.Work.CheckAlloc(int64(len(shape.member.Member))+1024) != nil {
		return nil
	}
	parameter, _, known := compilerReferenceParameterSource(receiver, f, ctx, c.Work)
	if !known && local == nil {
		return nil
	}
	return &lambdaCompilerReference{parameter: parameter, local: local, originalReceiver: receiver, factory: f, shape: shape, receiver: receiver, context: ctx, target: target, erased: site.erased, instantiated: site.actual, template: "\x00LCAP0\x00::" + shape.member.Member, maxStack: shape.code.MaxStack, maxLocals: shape.code.MaxLocals, factoryStack: f.code.MaxStack, factoryLocals: f.code.MaxLocals}
}

// An unknown reference must not silently disappear from the class-wide
// compiler counter. Until ordinary reference/no-adapter events are represented,
// decline this projection in mixed or nested scopes before consuming any body.
func (c *ClassObjectDumper) compilerReferenceFlatScope() bool {
	if c == nil || c.obj == nil || len(c.obj.Methods) > 4096 {
		return false
	}
	total, sites := 0, 0
	implementations := map[string]bool{}
	for _, m := range c.obj.Methods {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		for _, a := range m.Attributes {
			code, ok := a.(*CodeAttribute)
			if !ok {
				continue
			}
			if code == nil || len(code.Code) > 65535 || len(code.Code) > (1<<20)-total {
				return false
			}
			total += len(code.Code)
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
			decoder.Work = c.Work
			if decoder.ParseOpcode() != nil {
				return false
			}
			for _, op := range constructorMotionOps(decoder) {
				if !nativeProofWork(c.Work, 1) {
					return false
				}
				if op.Instr.OpCode != core.OP_INVOKEDYNAMIC {
					continue
				}
				sites++
				if sites > 1024 || isSyntheticMethod(m.AccessFlags) {
					return false
				}
				f := &lambdaSourceFactory{object: c.obj, method: m, code: code, pc: int(op.CurrentOffset)}
				if !f.seal(c.Work) {
					return false
				}
				site, known := c.originalCompilerReferenceSite(f)
				if !known || c.originalLambdaDeclaration(c.obj.GetClassName(), site.implementation.Member, site.implementation.Description) != core.LambdaDeclarationSyntheticBody {
					return false
				}
				key := site.implementation.Member + site.implementation.Description
				if implementations[key] || c.Work != nil && c.Work.CheckAlloc(int64(len(implementations)+1)*128+int64(len(key))) != nil {
					return false
				}
				implementations[key] = true
			}
		}
	}
	return sites > 0
}

func (c *ClassObjectDumper) dumpCompilerVarargsReference(name, descriptor string, captured []values.JavaValue, request methodSourceRequest) (core.LambdaBodySource, bool) {
	if c == nil || c.FuncCtx == nil {
		return core.LambdaBodySource{}, false
	}
	proof := c.proveCompilerVarargsReference(name, descriptor, captured, request, c.FuncCtx)
	if proof == nil || proof.static && !c.compilerReferenceCounterScopeClosed() || !proof.static && !c.compilerReferenceFlatScope() {
		return core.LambdaBodySource{}, false
	}
	context, known := c.FuncCtx.SnapshotForSourceProof(c.Work)
	if !known {
		return core.LambdaBodySource{}, false
	}
	proof.context = context
	// A lost or mutated receipt must not erase its final identity obligation.
	c.compilerReferenceFactorySeen = true
	request.role = methodSourceCompilerReference
	key := fmt.Sprintf("source:compiler-reference,request:%d", request.serial)
	body := &dumpedMethods{member: proof.shape.method, descriptor: descriptor, methodName: name, code: proof.template, sourceRequest: request, sourceDumper: c, sourceObject: c.obj, sourceCacheKey: key, compilerReference: proof}
	c.dumpedMethodsSet[key] = body
	if !c.recordLambdaMethodSourceBody(name, descriptor, request, body) {
		delete(c.dumpedMethodsSet, key)
		return core.LambdaBodySource{}, false
	}
	source := core.LambdaBodySource{Code: body.code, ReferenceTarget: proof.target}
	if proof.static {
		source.ReferenceOwner = strings.ReplaceAll(proof.shape.member.Name, "/", ".")
		source.ReferenceQualifier = proof.qualifier
	}
	source.RecordRenderedWithCaptures = func(observation core.LambdaSourceRender) {
		c.recordCompilerReferenceCapture(body, observation)
		c.recordNativeLambdaRenderedPacket(body, body.code, observation.Packet)
		c.recordLambdaMethodEmission(body, body.code, observation.Packet)
	}
	return source, true
}

func (c *ClassObjectDumper) compilerReferenceSourcePacketClosed(body *dumpedMethods) bool {
	if body == nil || body.compilerReference == nil || body.lambdaParameterBinding != nil || body.sourceRequest.role != methodSourceCompilerReference {
		return false
	}
	p := body.compilerReference
	if p.shape == nil || p.shape.code == nil || p.static && (p.receiver != nil || p.originalReceiver != nil || p.parameter != nil || p.shape.receiverCount() != 0) || !p.static && (p.receiver == nil || p.receiver != p.originalReceiver || p.shape.receiverCount() != 1) || p.context == nil || p.target == nil || !p.factory.closed() || !p.factory.sealed || body.member != p.shape.method || body.sourceRequest.factory != p.factory || p.shape.code.MaxStack != p.maxStack || p.shape.code.MaxLocals != p.maxLocals || p.factory.code.MaxStack != p.factoryStack || p.factory.code.MaxLocals != p.factoryLocals || body.code != p.template {
		return false
	}
	var captured []values.JavaValue
	if !p.static {
		captured = []values.JavaValue{p.receiver}
	}
	fresh := c.proveCompilerVarargsReference(body.methodName, body.descriptor, captured, body.sourceRequest, p.context)
	scopeClosed := p.static && c.compilerReferenceCounterScopeClosed() || !p.static && c.compilerReferenceFlatScope()
	return fresh != nil && fresh.static == p.static && fresh.qualifier == p.qualifier && fresh.parameter == p.parameter && reflect.DeepEqual(fresh.local, p.local) && fresh.shape.method == p.shape.method && fresh.shape.code == p.shape.code && slices.Equal(fresh.shape.codeBytes, p.shape.codeBytes) && fresh.shape.member == p.shape.member && fresh.shape.packCount == p.shape.packCount && slices.Equal(fresh.shape.parameters, p.shape.parameters) && fresh.template == p.template && fresh.erased == p.erased && fresh.instantiated == p.instantiated && reflect.DeepEqual(fresh.target.RawType(), p.target.RawType()) && scopeClosed
}

// A compiler-owned reference consumes an implementation ordinal at its real
// source occurrence, just as an arrow does. Ordinary :: tokens cannot borrow
// that role: cardinality, event kind and original factory receipts must match.
func nativeLambdaSourceImplementationTokens(source string, work *workbudget.Budget, references bool) ([]int, bool) {
	if !references {
		return nativeLambdaSourceArrows(source, work)
	}
	if len(source) > 16<<20 || !nativeProofWork(work, int64(len(source))+1) || work != nil && work.CheckAlloc(4096*8) != nil {
		return nil, false
	}
	var positions []int
	state, depth := scanNormal, 0
	for i := 0; i < len(source); i++ {
		state = scanAdvance(source, &i, state, &depth)
		if state != scanNormal || i+1 >= len(source) {
			continue
		}
		if source[i:i+2] == "->" || source[i:i+2] == "::" {
			if len(positions) == 4096 {
				return nil, false
			}
			positions = append(positions, i)
			i++
		}
	}
	return positions, state == scanNormal || state == scanLineComment
}

func (c *ClassObjectDumper) constructorBoundCompilerReferenceTarget(code *CodeAttribute, pc int, delegateOwner, delegateDescriptor string, receiver *values.JavaRef) (types.JavaType, string, string, *dumpedMethods, bool) {
	fail := func() (types.JavaType, string, string, *dumpedMethods, bool) { return nil, "", "", nil, false }
	if c == nil || c.FuncCtx == nil || len(c.lambdaMethodSourceBodies) > 4096 {
		return fail()
	}
	var body *dumpedMethods
	for _, receipt := range c.lambdaMethodSourceBodies {
		if !nativeProofWork(c.Work, 1) {
			return fail()
		}
		if receipt == nil || receipt.body == nil || receipt.body.compilerReference == nil {
			continue
		}
		f := receipt.request.factory
		if f == nil || f.code != code || f.pc != pc || f.method != c.CurrentMethod {
			continue
		}
		if body != nil || receipt.body.compilerReference.receiver != receiver || c.lambdaMethodSourceBody(receipt.method, c.Work) != receipt.body {
			return fail()
		}
		body = receipt.body
	}
	if body == nil {
		return fail()
	}
	p := body.compilerReference
	target := p.target
	provider := c.FuncCtx.InvocationMetadata
	if provider == nil {
		return fail()
	}
	owner, known := provider(delegateOwner)
	if !known || owner.Name != delegateOwner || !owner.MembersComplete || len(owner.Methods) > 4096 {
		return fail()
	}
	matched := false
	for _, m := range owner.Methods {
		if !nativeProofWork(c.Work, 1) {
			return fail()
		}
		if m.Name != "<init>" || m.Desc != delegateDescriptor {
			continue
		}
		physical, _, exact := types.EraseConcreteMethodSignatureWithThrows(m.Signature)
		original, known := c.nativeAnnotationDeclarationResolver()(delegateOwner)
		if !exact || physical != delegateDescriptor || !known || !constructorBoundOriginalSignature(original, "<init>", delegateDescriptor, m.Signature, c.Work) {
			return fail()
		}
		formals, parameters, result := types.ParseMethodSignatureFull(m.Signature, c.FuncCtx)
		if matched || formals != "" || result == nil || len(parameters) == 0 || parameters[0] == nil || !reflect.DeepEqual(parameters[0].RawType(), target.RawType()) {
			return fail()
		}
		matched = true
	}
	if !matched {
		return fail()
	}
	return target, p.shape.member.Member, p.instantiated, body, true
}

// Pending evaluation and completed source binding are different phases. Only
// the original per-PC/per-port witness permits following a snapshot's seed;
// an arbitrary Val or a same-spelled ref never identifies the source parameter.
func compilerReferenceParameterSource(receiver *values.JavaRef, f *lambdaSourceFactory, ctx *class_context.ClassContext, work *workbudget.Budget) (*values.JavaRef, bool, bool) {
	if receiver == nil || receiver.Id == nil || receiver.CustomValue != nil || !f.closed() || ctx == nil || !nativeProofWork(work, 1) {
		return nil, false, false
	}
	if _, known := receiver.OriginalParameterSlot(); known && receiver.StackVar == nil {
		return receiver, false, true
	}
	pc, index, known := receiver.OriginalInlinedDynamicOperandWitness(receiver.Val)
	var value values.JavaValue
	pending := false
	if known {
		value = receiver.StackVar
	} else {
		pc, index, known = receiver.OriginalPendingDynamicOperand(receiver.Val)
		value = receiver.Val
		pending = true
	}
	if !known || pc != f.pc || index != 0 {
		return nil, false, false
	}
	seed, bounded := nativeMemberEnclosingUnpack(value, work)
	parameter, typed := seed.(*values.JavaRef)
	if !bounded || !typed || parameter == nil || parameter.Id == nil || parameter.CustomValue != nil || parameter.StackVar != nil {
		return nil, false, false
	}
	if _, known := parameter.OriginalParameterSlot(); !known {
		return nil, false, false
	}
	a, ak := values.SourceTypeErasure(receiver.Type(), ctx)
	b, bk := values.SourceTypeErasure(parameter.Type(), ctx)
	return parameter, pending, ak && bk && a == b
}

// Observe the actual writer once. Final publication requires the original
// parameter copy to have been inlined by its existing unchanged-word proof;
// a pending snapshot, materialized alias or preview render cannot certify it.
func (c *ClassObjectDumper) recordCompilerReferenceCapture(body *dumpedMethods, observation core.LambdaSourceRender) bool {
	if c == nil || body == nil || body.compilerReference == nil {
		return false
	}
	p := body.compilerReference
	frame := c.lambdaMethodRendering
	if frame == nil {
		return true
	}
	if p.static {
		if !observation.Complete || len(observation.Captures) != 0 || frame.body == nil || frame.body.member != p.factory.method || frame.ended || frame.invalid || p.captureEmission != nil || c.FuncCtx == nil || c.CurrentMethod != p.factory.method {
			return false
		}
		context, known := c.FuncCtx.SnapshotForSourceProof(c.Work)
		if !known {
			return false
		}
		qualifier, known := context.StaticMethodReferenceTypeName(strings.ReplaceAll(p.shape.member.Name, "/", "."))
		if !known || qualifier != p.qualifier {
			return false
		}
		p.captureEmission = &compilerReferenceCaptureEmission{frame: frame, context: context, text: qualifier}
		return true
	}
	if !observation.Complete || len(observation.Captures) != 1 || observation.Captures[0].Index != 0 || frame.body == nil || frame.body.member != p.factory.method || frame.ended || frame.invalid || p.captureEmission != nil || c.FuncCtx == nil || c.CurrentMethod != p.factory.method {
		return false
	}
	if p.local != nil {
		source := c.compilerReferenceLocalCaptureSource(p)
		context, known := c.FuncCtx.SnapshotForSourceProof(c.Work)
		if source == nil || !known || p.receiver.String(context) != observation.Captures[0].Text {
			return false
		}
		p.captureEmission = &compilerReferenceCaptureEmission{frame: frame, context: context, localSource: source, text: observation.Captures[0].Text}
		return true
	}
	parameter, pending, known := compilerReferenceParameterSource(p.receiver, p.factory, c.FuncCtx, c.Work)
	if !known || pending || parameter != p.parameter {
		return false
	}
	context, known := c.FuncCtx.SnapshotForSourceProof(c.Work)
	if !known || parameter.CustomValue != nil || parameter.StackVar != nil || parameter.String(context) != observation.Captures[0].Text {
		return false
	}
	p.captureEmission = &compilerReferenceCaptureEmission{frame: frame, context: context, parameter: parameter, text: observation.Captures[0].Text}
	return true
}

func (c *ClassObjectDumper) compilerReferenceFinalCaptureClosed(body, parent *dumpedMethods) bool {
	if body == nil || body.compilerReference == nil || parent == nil {
		return false
	}
	p := body.compilerReference
	emission := p.captureEmission
	if emission == nil || emission.frame != parent.lambdaEmission || !emission.frame.closed() || emission.frame.body != parent || emission.frame.request != parent.sourceRequest || emission.parameter != p.parameter || p.factory.method != parent.member || emission.context == nil {
		return false
	}
	if p.static {
		qualifier, known := emission.context.StaticMethodReferenceTypeName(strings.ReplaceAll(p.shape.member.Name, "/", "."))
		return known && emission.parameter == nil && qualifier == p.qualifier && qualifier == emission.text
	}
	if p.local != nil {
		site := &nativeLambdaLocalCaptureSite{method: p.factory.method, code: p.factory.code, pc: p.factory.pc, operands: []*nativeEnumSelectorProducer{p.local}}
		source := emission.localSource
		if p.parameter != nil || source == nil || source.method != p.factory.method || source.code != p.factory.code || !source.hasFactoryPC || source.factoryPC != p.factory.pc || len(source.values) != 1 || source.values[0] != p.receiver {
			return false
		}
		// The enclosing source and the writer's context are separately retained.
		// Reopen both against this same original definition and factory; a
		// same-spelled reference in another method cannot borrow either view.
		written := c.compilerReferenceLocalPort(p.factory, p.shape, p.receiver, p.shape.parameters[0], emission.context)
		retained := c.compilerReferenceLocalPort(p.factory, p.shape, p.receiver, p.shape.parameters[0], &source.context)
		return written != nil && retained != nil && reflect.DeepEqual(written, p.local) && reflect.DeepEqual(retained, p.local) && nativeLambdaLocalCaptureSourceClosed(site, source, c.Work) && p.receiver.String(emission.context) == emission.text
	}
	parameter, pending, known := compilerReferenceParameterSource(p.receiver, p.factory, emission.context, c.Work)
	return known && !pending && parameter == p.parameter && c.compilerReferenceParameterPort(p.factory, p.shape, p.receiver, p.shape.parameters[0], emission.context) && parameter.String(emission.context) == emission.text
}

// A metadata public bit is not source accessibility. Reopen the original
// receiver type and selected declaring method. This initial source domain
// admits top-level receivers in a public or the caller's package scope; member,
// local and anonymous receiver types need their own lexical access certificate.
// The declaring type can differ through inheritance: only the printed receiver
// type needs this top-level spelling proof, while the actual selected method
// still needs its original access/varargs/Signature/throws packet.
func (c *ClassObjectDumper) compilerReferenceOriginalAccess(shape *lambdaVarargsAdapter, plan callbinding.VariableArityReferencePlan) bool {
	if c == nil || c.obj == nil || shape == nil || !plan.Supported || plan.DeclaringOwner == "" {
		return false
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	receiver, known := resolve(shape.member.Name)
	if !known || receiver == nil || receiver.GetClassName() != shape.member.Name || receiver.MajorVersion < 49 || receiver.AccessFlags&0x000e != 0 || len(receiver.Attributes) > 65535 || !nativeProofWork(c.Work, int64(len(receiver.Attributes))+1) {
		return false
	}
	samePackage := nativeBinaryPackage(receiver.GetClassName()) == nativeBinaryPackage(c.obj.GetClassName())
	if receiver.AccessFlags&1 == 0 && !samePackage {
		return false
	}
	static := shape.opcode == core.OP_INVOKESTATIC
	if !static && (receiver.AccessFlags&0x0200 != 0) != (shape.opcode == core.OP_INVOKEINTERFACE) {
		return false
	}
	for _, attribute := range receiver.Attributes {
		switch a := attribute.(type) {
		case *UnparsedAttribute:
			if a == nil || a.Name == "EnclosingMethod" {
				return false
			}
		case *InnerClassesAttribute:
			if a == nil || len(a.Classes) > 65535 {
				return false
			}
			for _, row := range a.Classes {
				if row == nil || !nativeProofWork(c.Work, 1) {
					return false
				}
				name, known := sourceBridgeClassName(receiver, row.InnerClassInfoIndex)
				if !known || name == receiver.GetClassName() {
					return false
				}
			}
		}
	}
	declaring, known := resolve(plan.DeclaringOwner)
	if !known || declaring == nil || declaring.GetClassName() != plan.DeclaringOwner || len(declaring.Methods) > 4096 {
		return false
	}
	matched := false
	for _, m := range declaring.Methods {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		name, nk := sourceBridgeUTF8(declaring, m.NameIndex)
		desc, dk := sourceBridgeUTF8(declaring, m.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		if name != shape.member.Member || desc != shape.member.Description {
			continue
		}
		if matched || m.AccessFlags&0x0080 == 0 || m.AccessFlags&0x0040 != 0 || (m.AccessFlags&0x0008 != 0) != static || m.AccessFlags&0x0002 != 0 && !(static && declaring.GetClassName() == c.obj.GetClassName()) || m.AccessFlags&1 == 0 && nativeBinaryPackage(declaring.GetClassName()) != nativeBinaryPackage(c.obj.GetClassName()) || len(m.Attributes) > 64 {
			return false
		}
		signature, known := nativeMethodLocalOriginalSignature(declaring, m.Attributes, c.Work)
		if !known || signature != plan.Selected.Signature {
			return false
		}
		if !nativeProofWork(c.Work, int64(len(m.Attributes))+1) {
			return false
		}
		exceptionAttributes := 0
		for _, attribute := range m.Attributes {
			if !nativeProofWork(c.Work, 1) {
				return false
			}
			switch a := attribute.(type) {
			case *ExceptionsAttribute:
				exceptionAttributes++
				if a == nil || exceptionAttributes > 1 || len(a.ExceptionIndexTable) != 0 {
					return false
				}
			case *UnparsedAttribute:
				if a == nil || a.Name == "Exceptions" {
					return false
				}
			}
		}
		exceptions, known := originalMethodExceptions(declaring, m)
		if !known || len(exceptions) != 0 {
			return false
		}
		matched = true
	}
	return matched
}

// Erasure is not a Java lookup domain. An original intersection/type-variable
// declaration can have overloads absent from its first bound, even when the
// solver's JavaRef has that bound's concrete type. Until an original source
// qualifier is separately certified, require an empty declaration environment.
func (c *ClassObjectDumper) compilerReferenceConcreteFactoryScope(f *lambdaSourceFactory) bool {
	if c == nil || c.obj == nil || !f.closed() || f.object != c.obj || len(c.obj.Methods) > 4096 || len(f.method.Attributes) > 65535 || !nativeProofWork(c.Work, int64(len(c.obj.Methods)+len(f.method.Attributes))+1) {
		return false
	}
	// Sealing retains flags, descriptor and Code bytes. It does not establish
	// declaration ownership: a detached shallow copy can retain all of them.
	owned, codes := 0, 0
	for _, method := range c.obj.Methods {
		if method == nil {
			return false
		}
		if method == f.method {
			owned++
		}
	}
	for _, attribute := range f.method.Attributes {
		if code, ok := attribute.(*CodeAttribute); ok {
			if code != f.code {
				return false
			}
			codes++
		}
	}
	if owned != 1 || codes != 1 {
		return false
	}
	classSig, known := nativeMethodLocalOriginalSignature(c.obj, c.obj.Attributes, c.Work)
	if !known {
		return false
	}
	methodSig, known := nativeMethodLocalOriginalSignature(c.obj, f.method.Attributes, c.Work)
	if !known {
		return false
	}
	cost := int64(len(classSig)+len(methodSig)) + 1
	if cost > 1<<20 || !nativeProofWork(c.Work, cost) || c.Work != nil && c.Work.CheckAlloc(cost*32+256) != nil {
		return false
	}
	if classSig != "" {
		formals, refs, valid := types.SignatureTypeVariableReferences(classSig)
		if !valid || len(formals) != 0 || len(refs) != 0 {
			return false
		}
	}
	if methodSig != "" {
		erased, _, valid := types.EraseConcreteMethodSignatureWithThrows(methodSig)
		descriptor, known := sourceBridgeUTF8(c.obj, f.method.DescriptorIndex)
		if !valid || !known || erased != descriptor {
			return false
		}
	}
	return true
}
