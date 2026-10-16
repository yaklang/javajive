package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Signature names alone cannot bind a constructor's enclosing formal. Find its
// nearest original declaration through the certified lexical edges, including
// method formals at an anonymous class's EnclosingMethod edge. Static cuts are
// owned by nativeMemberLexicalAncestors, not inferred from binary-name prefixes.
func nativeMemberLexicalFormalDeclaration(object *ClassObject, objects map[string]*ClassObject, variable string, work *workbudget.Budget) (*ClassObject, *MemberInfo, bool) {
	ancestors, known := nativeMemberLexicalAncestors(object, objects, work)
	if !known {
		return nil, nil, false
	}
	declares := func(object *ClassObject, attrs []AttributeInfo) (bool, bool) {
		seen, found := false, false
		for _, attr := range attrs {
			if !nativeProofWork(work, 1) {
				return false, false
			}
			if sig, ok := attr.(*SignatureAttribute); ok {
				if sig == nil || seen {
					return false, false
				}
				seen = true
				raw, valid := sourceBridgeUTF8(object, sig.SignatureIndex)
				own, _, grammar := types.SignatureTypeVariableReferences(raw)
				if !valid || !grammar || !nativeProofWork(work, int64(len(raw))) {
					return false, false
				}
				for _, name := range own {
					if name == variable {
						if found {
							return false, false
						}
						found = true
					}
				}
			}
		}
		return found, true
	}
	for _, node := range ancestors {
		found, valid := declares(node, node.Attributes)
		if !valid || found {
			return node, nil, valid
		}
		if owner, enclosing, anonymous := originalAnonymousOwner(node); anonymous && enclosing != "" {
			parent := objects[owner]
			if parent == nil || parent.GetClassName() != owner {
				return nil, nil, false
			}
			matches := 0
			var selected *MemberInfo
			for _, method := range parent.Methods {
				if method == nil || !nativeProofWork(work, 1) {
					return nil, nil, false
				}
				name, nk := sourceBridgeUTF8(parent, method.NameIndex)
				desc, dk := sourceBridgeUTF8(parent, method.DescriptorIndex)
				if !nk || !dk {
					return nil, nil, false
				}
				if name+desc == enclosing {
					matches++
					selected = method
				}
			}
			if matches != 1 {
				return nil, nil, false
			}
			found, valid = declares(parent, selected.Attributes)
			if !valid || found {
				return parent, selected, valid
			}
		}
	}
	return nil, nil, true
}

// A joint member allocation already proves its exact physical enclosing word.
// Restore erased casts only when the source formal and caller name denote the
// same original declaration, and that cast has exactly the original descriptor
// erasure. No qualified allocation/null check or runtime narrowing is added.
// A unique source constructor prevents a new overload selection; generic
// constructors and parameterized/foreign enclosing instances remain separate.
func nativeMemberLexicalConstructorArguments(p *nativeMemberFamily, child *nativeMemberClass, descriptor, caller string, args []values.JavaValue, ctx *class_context.ClassContext, work *workbudget.Budget) []values.JavaValue {
	if p == nil || child == nil || child.object == nil || child.static || child.formalCount != 0 || len(child.constructors) != 1 || child.accessBridges[descriptor] != nil || ctx == nil || ctx.IsStatic || len(args) > 128 || p.lexicalObjects[caller] == nil || p.lexicalObjects[child.object.GetClassName()] != child.object {
		return args
	}
	ctor := child.constructors[descriptor]
	if ctor == nil || len(descriptor) > 65535 || len(ctor.sourceDescriptor) > 65535 || !nativeProofWork(work, int64(len(descriptor)+len(ctor.sourceDescriptor))) || work != nil && work.CheckAlloc(int64(len(descriptor)+len(ctor.sourceDescriptor))*32) != nil {
		return args
	}
	params, result, err := callbinding.Descriptor(ctor.sourceDescriptor)
	if err != nil || result != "V" || len(params) != len(args) {
		return args
	}
	physical, result, err := callbinding.Descriptor(descriptor)
	if err != nil || result != "V" || len(physical) != len(params)+1 || physical[0] != "L"+child.owner+";" {
		return args
	}
	for i, param := range params {
		if physical[i+1] != param {
			return args
		}
	}
	signature, matches := "", 0
	for _, method := range child.object.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return args
		}
		name, nk := sourceBridgeUTF8(child.object, method.NameIndex)
		desc, dk := sourceBridgeUTF8(child.object, method.DescriptorIndex)
		if !nk || !dk {
			return args
		}
		if name != "<init>" || desc != descriptor {
			continue
		}
		matches++
		seen := false
		for _, attr := range method.Attributes {
			if !nativeProofWork(work, 1) {
				return args
			}
			if sig, ok := attr.(*SignatureAttribute); ok {
				if sig == nil || seen {
					return args
				}
				seen = true
				var valid bool
				signature, valid = sourceBridgeUTF8(child.object, sig.SignatureIndex)
				if !valid {
					return args
				}
			}
		}
	}
	own, references, grammar := types.SignatureTypeVariableReferences(signature)
	if matches != 1 || signature == "" || !grammar || len(own) != 0 || !nativeProofWork(work, int64(len(signature))) || work != nil && work.CheckAlloc(int64(len(signature))*32+int64(len(args))*32) != nil {
		return args
	}
	_, formals, returned := types.ParseMethodSignatureFull(signature, ctx)
	if len(formals) != len(params) || returned == nil || returned.String(ctx) != "void" {
		return args
	}
	// Current-method binders are nearer than the caller's class/anonymous edge.
	// Reopen the exact original declaration instead of trusting a same-spelled
	// name or a mutable rendering context to stand for that declaration.
	currentOwn := map[string]bool{}
	currentMatches := 0
	for _, method := range p.lexicalObjects[caller].Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return args
		}
		name, nk := sourceBridgeUTF8(p.lexicalObjects[caller], method.NameIndex)
		desc, dk := sourceBridgeUTF8(p.lexicalObjects[caller], method.DescriptorIndex)
		if !nk || !dk {
			return args
		}
		if name != ctx.FunctionName || desc != ctx.CurrentMethodDesc {
			continue
		}
		currentMatches++
		seen := false
		for _, attr := range method.Attributes {
			if !nativeProofWork(work, 1) {
				return args
			}
			if sig, ok := attr.(*SignatureAttribute); ok {
				if sig == nil || seen {
					return args
				}
				seen = true
				raw, valid := sourceBridgeUTF8(p.lexicalObjects[caller], sig.SignatureIndex)
				owned, _, grammar := types.SignatureTypeVariableReferences(raw)
				if !valid || !grammar || !nativeProofWork(work, int64(len(raw))) {
					return args
				}
				for _, name := range owned {
					currentOwn[name] = true
				}
			}
		}
	}
	if currentMatches != 1 {
		return args
	}
	out := append([]values.JavaValue(nil), args...)
	for i, formal := range formals {
		if formal == nil {
			return args
		}
		variable, bare := formal.RawType().(*types.JavaClass)
		if !bare || variable == nil || !ctx.IsTypeParam(variable.Name) {
			continue
		}
		referenced := false
		for _, name := range references {
			referenced = referenced || name == variable.Name
		}
		if !referenced {
			continue
		}
		// The currently rendered method is nearer than its class declaration.
		shadowed := currentOwn[variable.Name]
		for _, name := range types.MethodFormalTypeParamNames(ctx.CurrentMethodSig) {
			shadowed = shadowed || name == variable.Name
		}
		if shadowed {
			continue
		}
		targetOwner, targetMethod, targetKnown := nativeMemberLexicalFormalDeclaration(child.object, p.lexicalObjects, variable.Name, work)
		callerOwner, callerMethod, callerKnown := nativeMemberLexicalFormalDeclaration(p.lexicalObjects[caller], p.lexicalObjects, variable.Name, work)
		if !targetKnown || !callerKnown || targetOwner == nil || targetOwner != callerOwner || targetMethod != callerMethod || args[i] == nil {
			continue
		}
		formalErasure, fk := values.SourceTypeErasure(formal, ctx)
		actualErasure, ak := values.SourceTypeErasure(args[i].Type(), ctx)
		if !fk || !ak || formalErasure != params[i] || actualErasure != params[i] {
			continue
		}
		out[i] = &values.CastExpression{Value: args[i], TargetType: formal, Binding: true}
	}
	return out
}
