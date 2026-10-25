package javaclassparser

import (
	"reflect"
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A flat superclass with its own formals erases hidden enclosing occurrences.
// Its overriding declaration must expose the same parameter types to javac.
// Recover the original Signature only from a closed original override edge and
// the exact projected parent declaration, never from a dollar-name heuristic,
// matching erasures alone, or an arbitrary raw parameter in fresh output.
func (a *sourceConstructorLoweringAuthority) projectedOverrideSignature(child *ClassObject, method *MemberInfo, original, fresh string, work *workbudget.Budget) bool {
	if child == nil || method == nil || a == nil || a.jar == nil || child.MajorVersion < 49 || child.MajorVersion > 52 || child.MinorVersion != 0 || method.AccessFlags&0x114a != 0 || len(types.MethodFormalTypeParamNames(original)) != 0 || len(original)+len(fresh) > 131070 || !nativeProofWork(work, int64(len(original)+len(fresh))+1) {
		return false
	}
	if work.CheckAlloc(int64(len(original)+len(fresh))*128) != nil {
		return false
	}
	name, named := sourceBridgeUTF8(child, method.NameIndex)
	desc, typed := sourceBridgeUTF8(child, method.DescriptorIndex)
	if !named || !typed || name == "<init>" || name == "<clinit>" || !memberReferenceAttributesUnannotated(method.Attributes) {
		return false
	}
	d := NewClassObjectDumper(child)
	d.Work, d.foldSiblingResolver, d.declarationResolver = work, a.jar.enumSiblingResolver(), a.jar.declarationResolver
	resolve := d.nativeAnnotationDeclarationResolver()
	childScopes, local, valid := d.flattenedMethodLexicalScopesResolved(resolve)
	if !valid {
		return false
	}
	if local {
		if _, closed := d.anonymousGenericScopeOwner(); !closed {
			return false
		}
	}
	erased, _, valid := types.EraseLexicalScopedMethodSignatureWithThrows(childScopes, original)
	freshErased, _, freshValid := types.EraseLexicalScopedMethodSignatureWithThrows(childScopes, fresh)
	if !valid || !freshValid || erased != desc || freshErased != desc {
		return false
	}
	classSignature, valid := nativeMethodLocalOriginalSignature(child, child.Attributes, work)
	if !valid {
		return false
	}
	super, _ := types.ParseClassSignatureSupers(classSignature)
	parameterized, valid := types.AsParameterizedType(super)
	if !valid || strings.ReplaceAll(parameterized.RawClassName, ".", "/") != child.GetSupperClassName() || len(parameterized.OwnerSegments) < 2 || len(parameterized.OwnerSegments) > 64 {
		return false
	}
	parent, known := resolve(child.GetSupperClassName())
	if !known || parent == nil || parent.GetClassName() != child.GetSupperClassName() || parent.AccessFlags&0x0200 != 0 || len(parent.Methods) > 4096 {
		return false
	}
	projection := d.buildSiblingReferenceSourceFormals(func(n string, raw []byte) bool { return a.memberGenericSourceReturned(n, raw, work) })
	if projection == nil {
		return false
	}
	if _, known := projection(parent.GetClassName()); !known {
		return false
	}
	sigma := map[string]types.JavaType{}
	previous := ""
	for _, segment := range parameterized.OwnerSegments {
		owner := strings.ReplaceAll(segment.BinaryName, ".", "/")
		obj, known := resolve(owner)
		if !known || obj == nil || obj.GetClassName() != owner || !nativeProofWork(work, 1) {
			return false
		}
		if previous != "" {
			outer, _, flags, member := originalMemberOwner(obj)
			if !member || flags&StaticFlag != 0 || outer != previous {
				return false
			}
		}
		sig, valid := nativeMethodLocalOriginalSignature(obj, obj.Attributes, work)
		formals := types.ClassFormalTypeParamNames(sig)
		if !valid || len(formals) != len(segment.TypeArgs) || len(formals) > 128 {
			return false
		}
		for i, n := range formals {
			sigma[n] = segment.TypeArgs[i]
		}
		previous = owner
	}
	var declaration *MemberInfo
	for _, candidate := range parent.Methods {
		if candidate == nil || !nativeProofWork(work, 1) {
			return false
		}
		n, nk := sourceBridgeUTF8(parent, candidate.NameIndex)
		s, sk := sourceBridgeUTF8(parent, candidate.DescriptorIndex)
		if !nk || !sk {
			return false
		}
		if n == name && (s == desc || projectedOverrideCovariantBridge(child, method, s, work)) {
			if declaration != nil {
				return false
			}
			declaration = candidate
		}
	}
	if declaration == nil || declaration.AccessFlags&0x115a != 0 || !memberReferenceAttributesUnannotated(declaration.Attributes) {
		return false
	}
	if declaration.AccessFlags&5 == 0 {
		pkg := func(n string) string {
			if i := strings.LastIndexByte(n, '/'); i >= 0 {
				return n[:i]
			}
			return ""
		}
		if pkg(parent.GetClassName()) != pkg(child.GetClassName()) {
			return false
		}
	}
	parentSignature, valid := nativeMethodLocalOriginalSignature(parent, declaration.Attributes, work)
	if !valid || parentSignature == "" || len(types.MethodFormalTypeParamNames(parentSignature)) != 0 {
		return false
	}
	pd := NewClassObjectDumper(parent)
	pd.Work = work
	parentScopes, local, valid := pd.flattenedMethodLexicalScopesResolved(resolve)
	if !valid || local {
		return false
	}
	parentErased, _, valid := types.EraseLexicalScopedMethodSignatureWithThrows(parentScopes, parentSignature)
	projected, proved := types.ProjectHiddenLexicalMethodSignature(parentScopes, parentSignature)
	parentDescriptor, parentTyped := sourceBridgeUTF8(parent, declaration.DescriptorIndex)
	if !valid || !parentTyped || parentErased != parentDescriptor || !proved || projected == parentSignature {
		return false
	}
	oldArgs, oldResult, oldThrows := types.ParseMethodSignatureWithThrows(original)
	newArgs, newResult, newThrows := types.ParseMethodSignatureWithThrows(fresh)
	parentArgs, parentResult := types.ParseMethodSignature(parentSignature)
	sourceArgs, _ := types.ParseMethodSignature(projected)
	if len(oldArgs) != len(newArgs) || len(oldArgs) != len(parentArgs) || len(oldArgs) != len(sourceArgs) || !reflect.DeepEqual(oldResult, newResult) || !reflect.DeepEqual(oldThrows, newThrows) || !reflect.DeepEqual(oldResult, types.SubstituteTypeVars(parentResult, sigma)) {
		return false
	}
	changed := false
	for i, arg := range oldArgs {
		if !nativeProofWork(work, 1) || !reflect.DeepEqual(arg, types.SubstituteTypeVars(parentArgs[i], sigma)) || !reflect.DeepEqual(newArgs[i], types.SubstituteTypeVars(sourceArgs[i], sigma)) {
			return false
		}
		changed = changed || !reflect.DeepEqual(arg, newArgs[i])
	}
	return changed
}

// A different erased result is an override edge only when the original child
// contains the exact compiler bridge that transports the parent protocol to
// this declaration. Preserve receiver and argument identity; a cast, helper,
// foreign receiver, static invocation or ambiguous bridge proves nothing.
func projectedOverrideCovariantBridge(child *ClassObject, target *MemberInfo, parentDescriptor string, work *workbudget.Budget) bool {
	name, named := sourceBridgeUTF8(child, target.NameIndex)
	desc, typed := sourceBridgeUTF8(child, target.DescriptorIndex)
	params, result, err := callbinding.Descriptor(desc)
	parentParams, parentResult, parentErr := callbinding.Descriptor(parentDescriptor)
	if !named || !typed || err != nil || parentErr != nil || desc == parentDescriptor || !callbinding.Reference(result) || !callbinding.Reference(parentResult) || !slices.Equal(params, parentParams) {
		return false
	}
	var found bool
	for _, m := range child.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return false
		}
		n, nk := sourceBridgeUTF8(child, m.NameIndex)
		d, dk := sourceBridgeUTF8(child, m.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		if n != name || d != parentDescriptor {
			continue
		}
		if found || m.AccessFlags&^uint16(7) != 0x1040 || m.AccessFlags&7 != target.AccessFlags&7 {
			return false
		}
		packet := inheritedBridgeForwardPacket(child, m, core.OP_INVOKEVIRTUAL, work)
		if packet == nil || packet.targetOwner != child.GetClassName() || packet.targetDescriptor != desc || packet.checkedArguments {
			return false
		}
		found = true
	}
	return found
}
