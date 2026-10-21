package javaclassparser

import (
	"slices"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A static varargs adapter has no receiver or eager-null-check port. Its
// implementation formals are the selected declaration's expanded erased
// formals, which may differ from the instantiated functional interface. Only
// a proved method reference can ask javac to regenerate that physical adapter.
// The initial static domain is an ordinary factory and target in this exact
// original top-level class; no foreign/private lexical access is inferred.
func (c *ClassObjectDumper) proveCompilerStaticVarargsReference(name, descriptor string, request methodSourceRequest, ctx *class_context.ClassContext) *lambdaCompilerReference {
	if c == nil || c.obj == nil || ctx == nil || ctx.InvocationMetadata == nil || c.options.SourceCompiler.effective() != ModernJavac || !request.valid() || request.captureCount != 0 || !request.factory.closed() || !request.factory.sealed || request.factory.object != c.obj || isSyntheticMethod(request.factory.method.AccessFlags) || !c.compilerReferenceConcreteFactoryScope(request.factory) || !nativeMemberTopLevelEvidence(c.obj, c.Work) {
		return nil
	}
	f := request.factory
	parentName, named := sourceBridgeUTF8(c.obj, f.method.NameIndex)
	parentDescriptor, typed := sourceBridgeUTF8(c.obj, f.method.DescriptorIndex)
	if !named || !typed || strings.ReplaceAll(ctx.ClassName, ".", "/") != c.obj.GetClassName() || ctx.FunctionName != parentName || ctx.CurrentMethodDesc != parentDescriptor || ctx.IsStatic != (f.method.AccessFlags&8 != 0) {
		return nil
	}
	site, known := c.originalCompilerReferenceSite(f)
	if !known || site.captureCount != 0 || site.implementation.Member != name || site.implementation.Description != descriptor {
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
	if !known || shape.receiverCount() != 0 || shape.member.Name != c.obj.GetClassName() || !c.compilerVarargsParameterSchema(shape) {
		return nil
	}
	actualParameters, actualResult, err := callbinding.Descriptor(site.actual)
	if err != nil || len(actualParameters) != len(shape.parameters) || actualResult != shape.result {
		return nil
	}
	provider := c.compilerStaticReferenceAccessProvider(ctx.InvocationMetadata)
	plan := callbinding.SelectVariableArityReference(callbinding.Witness{Owner: shape.member.Name, Name: shape.member.Member, Desc: shape.member.Description, Kind: callbinding.Static}, site.actual, provider, c.Work)
	if !plan.Supported || !slices.Equal(plan.ExpandedParameters, shape.parameters) || !c.compilerReferenceOriginalAccess(shape, plan) || !plan.Selected.ExceptionsKnown || len(plan.Selected.Exceptions) != 0 || !class_context.PreservesJava8Identifier(shape.member.Member) {
		return nil
	}
	target := c.compilerReferenceFunctionalTarget(site, ctx)
	if target == nil {
		return nil
	}
	component, named := c.lambdaSourceFactoryNameComponent(f)
	suffix, prefixed := strings.CutPrefix(name, "lambda$"+component+"$")
	ordinal, err := strconv.Atoi(suffix)
	if !named || !prefixed || err != nil || ordinal < 0 || ordinal >= len(c.obj.Methods) || suffix != strconv.Itoa(ordinal) {
		return nil
	}
	qualifier, known := ctx.StaticMethodReferenceTypeName(strings.ReplaceAll(shape.member.Name, "/", "."))
	if !known {
		return nil
	}
	return &lambdaCompilerReference{static: true, qualifier: qualifier, factory: f, shape: shape, context: ctx, target: target, erased: site.erased, instantiated: site.actual, template: qualifier + "::" + shape.member.Member, maxStack: shape.code.MaxStack, maxLocals: shape.code.MaxLocals, factoryStack: f.code.MaxStack, factoryLocals: f.code.MaxLocals}
}

// Source within the very same physical class can access its private overloads.
// Promote only entries corroborated by that original complete method table,
// in a private provider view. This is caller access, not public metadata.
func (c *ClassObjectDumper) compilerStaticReferenceAccessProvider(provider callbinding.Provider) callbinding.Provider {
	fallback := c.compilerReferenceAccessProvider(provider)
	return func(name string) (callbinding.Class, bool) {
		if c == nil || c.obj == nil || name != c.obj.GetClassName() {
			return fallback(name)
		}
		cl, known := provider(name)
		if !known || cl.Name != name || !cl.MembersComplete || !cl.ParentsComplete || len(cl.Methods) > 4096 || len(c.obj.Methods) > 4096 || len(cl.Parents) > 64 || len(c.obj.Interfaces) > 64 || !nativeProofWork(c.Work, int64(len(cl.Methods)+len(c.obj.Methods)+len(cl.Parents))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(cl.Methods)+len(c.obj.Methods))*256+int64(len(c.obj.Interfaces)+1)*32) != nil {
			return cl, false
		}
		// No Signature attribute is a positively known non-generic declaration.
		// nativeOriginalClassSignature instead asks whether one is present.
		signature, valid := nativeMethodLocalOriginalSignature(c.obj, c.obj.Attributes, c.Work)
		if !valid || signature != cl.Signature || cl.IsInterface != (c.obj.AccessFlags&0x0200 != 0) {
			return cl, false
		}
		var parents []string
		if c.obj.SuperClass != 0 {
			parent, known := sourceBridgeClassName(c.obj, c.obj.SuperClass)
			if !known {
				return cl, false
			}
			parents = append(parents, parent)
		}
		for _, index := range c.obj.Interfaces {
			parent, known := sourceBridgeClassName(c.obj, index)
			if !known || !nativeProofWork(c.Work, 1) {
				return cl, false
			}
			parents = append(parents, parent)
		}
		if !slices.Equal(parents, cl.Parents) {
			return cl, false
		}
		// Reopen both directions of the complete table. Otherwise a provider
		// could omit a competing overload while retaining MembersComplete.
		type methodKey struct{ name, descriptor string }
		originals := make(map[methodKey]*MemberInfo, len(c.obj.Methods))
		for _, original := range c.obj.Methods {
			if original == nil || !nativeProofWork(c.Work, 1) {
				return cl, false
			}
			n, nk := sourceBridgeUTF8(c.obj, original.NameIndex)
			d, dk := sourceBridgeUTF8(c.obj, original.DescriptorIndex)
			if !nk || !dk {
				return cl, false
			}
			if n == "<clinit>" {
				continue
			}
			key := methodKey{n, d}
			if originals[key] != nil {
				return cl, false
			}
			originals[key] = original
		}
		cl.Methods = slices.Clone(cl.Methods)
		for i, method := range cl.Methods {
			key := methodKey{method.Name, method.Desc}
			original := originals[key]
			if original == nil || !nativeProofWork(c.Work, 1) {
				return cl, false
			}
			signature, sk := nativeMethodLocalOriginalSignature(c.obj, original.Attributes, c.Work)
			exceptions, ek := originalMethodExceptions(c.obj, original)
			if !sk || signature != method.Signature || (signature != "") != method.Generic || (original.AccessFlags&8 != 0) != method.Static || (original.AccessFlags&0x80 != 0) != method.Varargs || (original.AccessFlags&0x40 != 0) != method.Bridge || !ek || !method.ExceptionsKnown || !slices.Equal(exceptions, method.Exceptions) {
				return cl, false
			}
			delete(originals, key)
			cl.Methods[i].Public = true
		}
		if len(originals) != 0 {
			return cl, false
		}
		cl.Public = true
		return cl, true
	}
}

// Mixed ordinary/nested arrows can share a counter with a compiler adapter,
// provided every original factory targets a distinct original synthetic body
// in this class. Direct references, foreign bodies and other bootstraps need
// their own counter representation and keep this projection unavailable.
func (c *ClassObjectDumper) compilerReferenceCounterScopeClosed() bool {
	if c == nil || c.obj == nil || len(c.obj.Methods) > 4096 {
		return false
	}
	total, sites := 0, 0
	implementations := map[string]bool{}
	for _, method := range c.obj.Methods {
		if method == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		for _, attribute := range method.Attributes {
			code, isCode := attribute.(*CodeAttribute)
			if !isCode {
				continue
			}
			if code == nil || len(code.Code) > 65535 || len(code.Code) > (1<<20)-total {
				return false
			}
			total += len(code.Code)
			decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, index) })
			decoder.Work = c.Work
			if decoder.ParseOpcode() != nil {
				return false
			}
			for _, instruction := range constructorMotionOps(decoder) {
				if !nativeProofWork(c.Work, 1) {
					return false
				}
				if instruction.Instr.OpCode != core.OP_INVOKEDYNAMIC {
					continue
				}
				sites++
				factory := &lambdaSourceFactory{object: c.obj, method: method, code: code, pc: int(instruction.CurrentOffset)}
				if sites > 1024 || !factory.seal(c.Work) {
					return false
				}
				site, known := c.originalCompilerReferenceSite(factory)
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
