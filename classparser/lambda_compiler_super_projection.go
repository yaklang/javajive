package javaclassparser

import (
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Binding super::m requires two views of one declaration: overload selection
// uses the concrete superclass substitution, while javac's private adapter
// still calls its erased JVM descriptor. Keep these views separate. Project
// only a directly declared, class-generic method through the original concrete
// superclass edge; method binders, nested owner binders and result/throws
// substitutions need separate proofs. Every other overload remains in the
// complete provider, so projection cannot hide a competitor.
func (c *ClassObjectDumper) compilerSuperReferenceSelection(shape *lambdaVarargsAdapter, site compilerReferenceSite, ctx *class_context.ClassContext) callbinding.VariableArityReferencePlan {
	provider := c.compilerReferenceAccessProvider(ctx.InvocationMetadata)
	witness := callbinding.Witness{Owner: shape.member.Name, Name: shape.member.Member, Desc: shape.member.Description, Kind: callbinding.Virtual}
	plan := callbinding.SelectFixedArityReference(witness, site.actual, provider, c.Work)
	if plan.Supported {
		return plan
	}
	unknown := callbinding.VariableArityReferencePlan{}
	owner := c.obj.GetSupperClassName()
	if owner != witness.Owner || !nativeProofWork(c.Work, 1) {
		return unknown
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	parent, known := resolve(owner)
	meta, available := provider(owner)
	if !known || parent == nil || parent.GetClassName() != owner || !available || meta.Name != owner || meta.IsInterface || !meta.MembersComplete || !meta.ParentsComplete || len(meta.Methods) > 4096 {
		return unknown
	}
	childSignature, valid := nativeMethodLocalOriginalSignature(c.obj, c.obj.Attributes, c.Work)
	parentSignature, parentValid := nativeMethodLocalOriginalSignature(parent, parent.Attributes, c.Work)
	if !valid || !parentValid || meta.Signature != parentSignature || len(childSignature)+len(parentSignature) > 65535 || !nativeProofWork(c.Work, int64(len(childSignature)+len(parentSignature))+1) {
		return unknown
	}
	if c.Work.CheckAlloc(int64(len(childSignature)+len(parentSignature)+1)*64+int64(len(meta.Methods))*64) != nil {
		return unknown
	}
	formals, references, concrete := types.SignatureTypeVariableReferences(childSignature)
	if !concrete || len(formals) != 0 || len(references) != 0 {
		return unknown
	}
	super, _ := types.ParseClassSignatureSupers(childSignature)
	parameterized, valid := types.AsParameterizedType(super)
	names := types.ClassFormalTypeParamNames(parentSignature)
	if !valid || strings.ReplaceAll(parameterized.RawClassName, ".", "/") != owner || len(parameterized.OwnerSegments) > 1 || len(names) == 0 || len(names) > 128 || len(names) != len(parameterized.TypeArgs) {
		return unknown
	}
	// Denotable, reifiable substitutions make the notional source parameters
	// exact descriptors. No wildcard capture or nested generic assignability is
	// approximated by an erasure-only overload query.
	reifiable := func(typ types.JavaType) (string, bool) {
		if typ == nil {
			return "", false
		}
		element := typ
		for element.IsArray() {
			element = element.ElementType()
		}
		if _, variable := types.TypeVariableName(element); variable || types.IsWildcardType(element) {
			return "", false
		}
		if _, generic := types.AsParameterizedType(element); generic {
			return "", false
		}
		return values.SourceTypeErasure(typ, ctx)
	}
	sigma := map[string]types.JavaType{}
	for i, name := range names {
		if _, known := sigma[name]; known {
			return unknown
		}
		if desc, known := reifiable(parameterized.TypeArgs[i]); !known || !callbinding.Reference(desc) {
			return unknown
		}
		sigma[name] = parameterized.TypeArgs[i]
	}
	var selected *callbinding.Method
	for i := range meta.Methods {
		m := &meta.Methods[i]
		if m.Name == witness.Name && m.Desc == witness.Desc {
			if selected != nil {
				return unknown
			}
			selected = m
		}
	}
	if selected == nil || !selected.Generic || selected.Static || selected.Bridge || selected.Varargs || !selected.Public || !strings.HasPrefix(selected.Signature, "(") || len(selected.Signature) > 65535 || !constructorBoundOriginalSignature(parent, selected.Name, selected.Desc, selected.Signature, c.Work) {
		return unknown
	}
	erased, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{parentSignature}, selected.Signature)
	parameters, result, throws := types.ParseMethodSignatureWithThrows(selected.Signature)
	physical, returned, err := callbinding.Descriptor(selected.Desc)
	if !valid || erased != selected.Desc || err != nil || len(parameters) != len(physical) || len(parameters) > 254 || len(throws) > 128 {
		return unknown
	}
	// The ordinary result/exception proof owns these ports. They must be
	// concrete before substitution, so this step grants input binding only.
	if resultDescriptor, valid := reifiable(result); !valid || resultDescriptor != returned {
		return unknown
	}
	for _, thrown := range throws {
		if _, valid := reifiable(thrown); !valid {
			return unknown
		}
	}
	projected := "("
	for _, parameter := range parameters {
		descriptor, known := reifiable(types.SubstituteTypeVars(parameter, sigma))
		if !known || descriptor == "V" || !nativeProofWork(c.Work, int64(len(descriptor))+1) {
			return unknown
		}
		projected += descriptor
	}
	projected += ")" + returned
	view, closed := c.compilerSuperReferenceAncestorView(owner, sigma, *selected, provider, reifiable)
	if !closed {
		return unknown
	}
	witness.Desc = projected
	plan = callbinding.SelectFixedArityReference(witness, site.actual, view, c.Work)
	if !plan.Supported || plan.DeclaringOwner != owner || plan.Selected.Desc != projected {
		return unknown
	}
	plan.Selected = *selected
	plan.ExpandedParameters = slices.Clone(physical)
	return plan
}

// A concrete superclass use also instantiates its original ancestor edges.
// Project the matching erased override declaration along those edges before
// overload selection. Retain every overload; an unproved generic rival still
// makes selection unknown. Reaching one ancestor with different substitutions
// is ambiguous, and a cycle or incomplete declaration cannot supply evidence.
func (c *ClassObjectDumper) compilerSuperReferenceAncestorView(owner string, sigma map[string]types.JavaType, selected callbinding.Method, provider callbinding.Provider, reifiable func(types.JavaType) (string, bool)) (callbinding.Provider, bool) {
	resolve := c.nativeAnnotationDeclarationResolver()
	views := map[string]callbinding.Class{}
	bindings := map[string]string{}
	active := map[string]bool{}
	totalMethods := 0
	var visit func(string, map[string]types.JavaType, int) bool
	visit = func(name string, substitutions map[string]types.JavaType, depth int) bool {
		if depth > 64 || active[name] || len(views) >= 128 || !nativeProofWork(c.Work, int64(len(name))+1) {
			return false
		}
		obj, found := resolve(name)
		meta, known := provider(name)
		if !found || obj == nil || obj.GetClassName() != name || !known || meta.Name != name || !meta.ParentsComplete || !meta.MembersComplete || len(meta.Parents) > 64 || len(meta.Methods) > 4096 {
			return false
		}
		physicalParents := slices.Clone(obj.GetInterfacesName())
		if parent := obj.GetSupperClassName(); parent != "" {
			physicalParents = append(physicalParents, parent)
		}
		providedParents := slices.Clone(meta.Parents)
		slices.Sort(physicalParents)
		slices.Sort(providedParents)
		if !slices.Equal(physicalParents, providedParents) {
			return false
		}
		signature, valid := nativeMethodLocalOriginalSignature(obj, obj.Attributes, c.Work)
		formals := types.ClassFormalTypeParamNames(signature)
		if !valid || signature != meta.Signature || len(formals) != len(substitutions) || len(signature) > 65535 || len(formals) > 128 {
			return false
		}
		key := ""
		for _, formal := range formals {
			desc, valid := reifiable(substitutions[formal])
			if !valid || !callbinding.Reference(desc) {
				return false
			}
			key += desc
		}
		if previous, seen := bindings[name]; seen {
			return previous == key
		}
		totalMethods += len(meta.Methods)
		if totalMethods > 16384 || !nativeProofWork(c.Work, int64(len(signature)+len(meta.Methods))+1) || c.Work.CheckAlloc(int64(len(meta.Methods)+1)*128+int64(len(signature)+1)*64) != nil {
			return false
		}
		active[name] = true
		defer delete(active, name)
		view := meta
		view.Methods = slices.Clone(meta.Methods)
		if len(formals) != 0 {
			view.Signature = "" // This node is the proved concrete use.
		}
		for i := range view.Methods {
			m := &view.Methods[i]
			if m.Name != selected.Name || m.Desc != selected.Desc || !m.Generic {
				continue
			}
			if m.Static || m.Bridge || m.Varargs || !m.Public || !m.AccessFlagsKnown || !strings.HasPrefix(m.Signature, "(") || len(m.Signature) > 65535 || !constructorBoundOriginalSignature(obj, m.Name, m.Desc, m.Signature, c.Work) {
				return false
			}
			originalFlags := false
			for _, original := range obj.Methods {
				if original == nil || !nativeProofWork(c.Work, 1) {
					return false
				}
				n, _ := sourceBridgeUTF8(obj, original.NameIndex)
				d, _ := sourceBridgeUTF8(obj, original.DescriptorIndex)
				if n == m.Name && d == m.Desc {
					if originalFlags || original.AccessFlags != m.AccessFlags {
						return false
					}
					originalFlags = true
				}
			}
			if !originalFlags {
				return false
			}
			erasure, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{signature}, m.Signature)
			params, result, exceptions := types.ParseMethodSignatureWithThrows(m.Signature)
			physical, returned, err := callbinding.Descriptor(m.Desc)
			ret, concrete := reifiable(result)
			if !valid || erasure != m.Desc || err != nil || len(params) != len(physical) || len(params) > 254 || len(exceptions) > 128 || !concrete || ret != returned {
				return false
			}
			for _, exception := range exceptions {
				if _, valid := reifiable(exception); !valid {
					return false
				}
			}
			projected := "("
			for _, param := range params {
				desc, valid := reifiable(types.SubstituteTypeVars(param, substitutions))
				if !valid || desc == "V" || !nativeProofWork(c.Work, int64(len(desc))+1) {
					return false
				}
				projected += desc
			}
			m.Desc, m.Signature, m.Generic = projected+")"+returned, "", false
		}
		super, interfaces := types.ParseClassSignatureSupers(signature)
		parents := append([]types.JavaType{super}, interfaces...)
		edges := map[string]types.JavaType{}
		for _, parent := range parents {
			if parent == nil {
				continue
			}
			raw, valid := types.RawClassFQN(parent)
			if !valid || edges[strings.ReplaceAll(raw, ".", "/")] != nil {
				return false
			}
			edges[strings.ReplaceAll(raw, ".", "/")] = parent
		}
		if signature != "" && len(edges) != len(meta.Parents) {
			return false
		}
		for _, parentName := range meta.Parents {
			if signature != "" && edges[parentName] == nil {
				return false
			}
			parent, known := resolve(parentName)
			if !known || parent == nil || parent.GetClassName() != parentName {
				return false
			}
			parentSig, valid := nativeMethodLocalOriginalSignature(parent, parent.Attributes, c.Work)
			names := types.ClassFormalTypeParamNames(parentSig)
			if !valid || len(names) > 128 {
				return false
			}
			next := map[string]types.JavaType{}
			if len(names) > 0 {
				edge, valid := types.AsParameterizedType(edges[parentName])
				if !valid || len(edge.OwnerSegments) > 1 || len(edge.TypeArgs) != len(names) {
					return false // A raw parent does not prove its type arguments.
				}
				for i, formal := range names {
					next[formal] = types.SubstituteTypeVars(edge.TypeArgs[i], substitutions)
				}
			}
			if !visit(parentName, next, depth+1) {
				return false
			}
		}
		views[name], bindings[name] = view, key
		return true
	}
	if !visit(owner, sigma, 0) {
		return nil, false
	}
	return func(name string) (callbinding.Class, bool) {
		if view, found := views[name]; found {
			return view, true
		}
		return provider(name)
	}, true
}
