package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Original named ownership and static flags delimit lexical type scope. Keep
// signatures ordered by declaration instead of flattening equal-spelled names.
func nativeMethodLocalLexicalSignatures(enclosing *ClassObject, owner *nativeMethodLocalOwner, family *nativeMemberFamily, work *workbudget.Budget) ([]string, bool) {
	if enclosing == nil || owner == nil || owner.declaration == nil || owner.owner != enclosing.GetClassName() {
		return nil, false
	}
	if family != nil && (family.owner == "" || family.lexicalObjects[enclosing.GetClassName()] != enclosing) {
		return nil, false
	}
	found := false
	for _, m := range enclosing.Methods {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if m == owner.declaration {
			found = true
		}
	}
	if !found {
		return nil, false
	}
	name, named := sourceBridgeUTF8(enclosing, owner.declaration.NameIndex)
	desc, typed := sourceBridgeUTF8(enclosing, owner.declaration.DescriptorIndex)
	if !named || !typed || name != owner.method || desc != owner.descriptor {
		return nil, false
	}

	if owner.declaration.AccessFlags&8 != 0 {
		return nil, true
	}
	signatures := []string{}
	current := enclosing
	seen := map[*ClassObject]bool{}
	for depth := 0; depth < 64; depth++ {
		if current == nil || seen[current] || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(depth+1)*128) != nil {
			return nil, false
		}
		if family != nil && family.lexicalObjects[current.GetClassName()] != current {
			return nil, false
		}
		seen[current] = true
		sig, ok := nativeMethodLocalOriginalSignature(current, current.Attributes, work)
		if !ok {
			return nil, false
		}
		signatures = append(signatures, sig)
		if family == nil || current.GetClassName() == family.owner {
			for i, j := 0, len(signatures)-1; i < j; i, j = i+1, j-1 {
				signatures[i], signatures[j] = signatures[j], signatures[i]
			}
			return signatures, true
		}
		child := family.children[current.GetClassName()]
		if child == nil || child.object != current || family.lexicalObjects[current.GetClassName()] != current {
			return nil, false
		}
		actual, name, flags, known := originalMemberOwner(current)
		if !known || actual != child.owner || name != child.name || flags != child.flags || child.static != (flags&8 != 0) {
			return nil, false
		}
		if child.static {
			for i, j := 0, len(signatures)-1; i < j; i, j = i+1, j-1 {
				signatures[i], signatures[j] = signatures[j], signatures[i]
			}
			return signatures, true
		}
		current = family.lexicalObjects[actual]
	}
	return nil, false
}

func nativeMethodLocalOriginalSignature(obj *ClassObject, attrs []AttributeInfo, work *workbudget.Budget) (string, bool) {
	result := ""
	seen := false
	for _, a := range attrs {
		if !nativeProofWork(work, 1) {
			return "", false
		}
		if s, ok := a.(*SignatureAttribute); ok {
			if seen || s == nil {
				return "", false
			}
			seen = true
			var valid bool
			result, valid = sourceBridgeUTF8(obj, s.SignatureIndex)
			if !valid || result == "" || !nativeProofWork(work, int64(len(result))) || work != nil && work.CheckAlloc(int64(len(result))*32) != nil {
				return "", false
			}
		}
	}
	return result, true
}

// Only the physically closed method owner can supply erasures for source
// captures. Existing renderer-only erasure remains conservative elsewhere.
func (c *ClassObjectDumper) nativeMethodLocalSourceErasure(t types.JavaType, local *nativeMethodLocalClass) (string, bool) {
	if c == nil || c.FuncCtx == nil || c.obj == nil || local == nil || local.owner == nil || c.obj.GetClassName() != local.owner.owner || c.FuncCtx.FunctionName != local.executionOwner().method || c.FuncCtx.CurrentMethodDesc != local.executionOwner().descriptor {
		return "", false
	}
	prefix := ""
	for depth := 0; depth < 32; depth++ {
		if t == nil || !nativeProofWork(c.Work, 1) {
			return "", false
		}
		if t.IsArray() {
			prefix += "["
			t = t.ElementType()
			continue
		}
		name, formal := types.RawClassFQN(t)
		if !formal || !c.FuncCtx.IsTypeParam(name) {
			d, ok := values.SourceTypeErasure(t, c.FuncCtx)
			return prefix + d, ok
		}
		classes, ok := nativeMethodLocalLexicalSignatures(c.obj, local.owner, c.nativeMemberRoot, c.Work)
		if !ok {
			return "", false
		}
		signature, ok := nativeMethodLocalOriginalSignature(c.obj, local.owner.declaration.Attributes, c.Work)
		if !ok {
			return "", false
		}
		if signature == "" {
			signature = local.owner.descriptor
		}
		if !nativeMethodLocalBindingBudget(classes, signature, c.Work) {
			return "", false
		}
		d, descriptor, ok := types.LexicalOwnerTypeVariableErasure(classes, signature, name)
		return prefix + d, ok && descriptor == local.owner.descriptor
	}
	return "", false
}

// Bound the derived environment work/storage before the grammar resolver
// snapshots declaration maps. The original UTF8 length alone understates a
// deep stack of inherited formals retained by successive owner declarations.
func nativeMethodLocalBindingBudget(classes []string, method string, work *workbudget.Budget) bool {
	if len(classes) > 64 {
		return false
	}
	bytes, names, cells := int64(len(method)), int64(0), int64(0)
	for _, sig := range classes {
		if len(sig) > 65535 || !nativeProofWork(work, int64(len(sig))+1) {
			return false
		}
		bytes += int64(len(sig))
		names += int64(len(types.ClassFormalTypeParamNames(sig)))
		cells += names
	}
	names += int64(len(types.MethodFormalTypeParamNames(method)))
	cells += names
	if bytes > 1<<20 || names > 512 {
		return false
	}
	return nativeProofWork(work, bytes*4+cells*4+names*128) && (work == nil || work.CheckAlloc(bytes*16+cells*96+names*128) == nil)
}

// Instantiate the original direct parent declaration before choosing a source
// constructor. This domain has one accessible fixed-arity candidate at the
// original arity, no constructor-owned formals or throws, and exactly the same
// runtime parameter erasures after instantiation. Consequently the typed view
// cannot add a runtime check or select a different overload. Captured operands
// and their pre-SUPER stores are still proved by the ordinary local packet.
func (c *ClassObjectDumper) nativeMethodLocalGenericDelegationParameters(parent *ClassObject, method *MemberInfo, parentSignature string) ([]types.JavaType, bool) {
	if c == nil || c.obj == nil || c.FuncCtx == nil || c.nativeMemberRoot == nil || parent == nil || method == nil || !nativeProofWork(c.Work, 1) {
		return nil, false
	}
	local := c.nativeMethodLocalCurrent
	if local == nil || local.object != c.obj || local.owner == nil || local.constructor == nil || method.AccessFlags&0x0080 != 0 || parent.GetClassName() != local.constructor.delegateOwner {
		return nil, false
	}
	original, proved := originalMethodLocalSourceConstructor(c.obj, c.nativeMemberRoot.lexicalObjects[local.owner.owner], c.Work)
	if !proved || !sameOriginalMethodLocalConstructor(original, local.constructor, c.Work) {
		return nil, false
	}
	signature, known := nativeMethodLocalOriginalSignature(parent, method.Attributes, c.Work)
	if !known || len(parentSignature) > 65535 || !nativeProofWork(c.Work, int64(len(parentSignature)+len(signature))*130+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(parentSignature)+len(signature))*256) != nil {
		return nil, false
	}
	seenThrows := false
	for _, attribute := range method.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if declared, ok := attribute.(*ExceptionsAttribute); ok {
			if declared == nil || seenThrows || len(declared.ExceptionIndexTable) != 0 {
				return nil, false
			}
			seenThrows = true
		}
	}
	erased, throws, valid := types.EraseClassBoundMethodSignatureWithThrows(parentSignature, signature)
	if !known || !valid || erased != local.constructor.delegateDescriptor || len(throws) != 0 {
		return nil, false
	}
	physical, result, err := callbinding.Descriptor(erased)
	if err != nil || result != "V" || len(physical) == 0 || len(physical) != len(local.constructor.delegateParams) {
		return nil, false
	}
	matches := 0
	for _, candidate := range parent.Methods {
		if candidate == nil || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		name, named := sourceBridgeUTF8(parent, candidate.NameIndex)
		desc, typed := sourceBridgeUTF8(parent, candidate.DescriptorIndex)
		if !named || !typed {
			return nil, false
		}
		if name != "<init>" {
			continue
		}
		params, ret, err := callbinding.Descriptor(desc)
		if err != nil || ret != "V" || candidate.AccessFlags&(0x0008|0x0100|0x0400) != 0 {
			return nil, false
		}
		if candidate.AccessFlags&2 != 0 || candidate.AccessFlags&5 == 0 && nativeAnonymousCallPackage(parent.GetClassName()) != nativeAnonymousCallPackage(c.obj.GetClassName()) {
			continue
		}
		if len(params) == len(physical) {
			if candidate != method || desc != erased || candidate.AccessFlags&0x0080 != 0 {
				return nil, false
			}
			matches++
		}
	}
	if matches != 1 {
		return nil, false
	}
	// The fresh provider must describe this exact original class/constructor;
	// a cached Signature cannot license a changed resolver declaration.
	metadata, known := c.buildInvocationMetadata()(parent.GetClassName())
	if !known || metadata.Name != parent.GetClassName() || !metadata.MembersComplete || metadata.Signature != parentSignature {
		return nil, false
	}
	matchedMetadata := 0
	for _, candidate := range metadata.Methods {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if candidate.Name == "<init>" && candidate.Desc == erased {
			if candidate.Signature != signature || candidate.Static || candidate.Varargs || candidate.Bridge {
				return nil, false
			}
			matchedMetadata++
		}
	}
	if matchedMetadata != 1 {
		return nil, false
	}
	localSignature, known := nativeMethodLocalOriginalSignature(c.obj, c.obj.Attributes, c.Work)
	if !known || localSignature == "" || c.FuncCtx.ClassSig != localSignature {
		return nil, false
	}
	localParent, interfaces := types.ParseClassSignatureSupers(localSignature)
	view, parameterized := types.AsParameterizedType(localParent)
	formals := types.ClassFormalTypeParamNames(parentSignature)
	if !parameterized || len(view.OwnerSegments) > 1 || strings.ReplaceAll(view.RawClassName, ".", "/") != parent.GetClassName() || c.obj.GetSupperClassName() != parent.GetClassName() || len(formals) == 0 || len(view.TypeArgs) != len(formals) || len(interfaces) != len(c.obj.Interfaces) {
		return nil, false
	}
	for i, typ := range interfaces {
		name, valid := types.RawClassFQN(typ)
		original, known := sourceBridgeClassName(c.obj, c.obj.Interfaces[i])
		if !valid || !known || strings.ReplaceAll(name, ".", "/") != original || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
	}
	owner := c.nativeMemberRoot.lexicalObjects[local.owner.owner]
	if _, known := nativeMethodLocalScopeSignature(c.obj, owner, local.owner, c.Work, c.nativeMemberRoot); !known {
		return nil, false
	}
	classes, known := nativeMethodLocalLexicalSignatures(owner, local.owner, c.nativeMemberRoot, c.Work)
	if !known {
		return nil, false
	}
	methodSignature, known := nativeMethodLocalOriginalSignature(owner, local.owner.declaration.Attributes, c.Work)
	if !known {
		return nil, false
	}
	if methodSignature == "" {
		methodSignature = local.owner.descriptor
	}
	// Source and original lexical scopes must agree before a formal's source
	// spelling can stand for the declaration whose erasure we just proved.
	expected := []string{}
	if methodSignature != local.owner.descriptor {
		expected = append(expected, methodSignature)
	}
	for i := len(classes) - 1; i >= 0; i-- {
		if classes[i] != "" {
			expected = append(expected, classes[i])
		}
	}
	observed := []string{}
	for _, signature := range c.FuncCtx.LexicalTypeParamSignatures {
		if signature != "" {
			observed = append(observed, signature)
		}
	}
	if len(expected) != len(observed) {
		return nil, false
	}
	for i := range expected {
		if expected[i] != observed[i] || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
	}
	if !nativeMethodLocalBindingBudget(append(append([]string(nil), classes...), localSignature), methodSignature, c.Work) || !nativeProofWork(c.Work, int64(len(parentSignature)+len(signature)+len(localSignature))*130+1) {
		return nil, false
	}
	scopes := make([]types.LexicalTypeScope, 0, len(classes)+2)
	for _, signature := range classes {
		scopes = append(scopes, types.LexicalTypeScope{Signature: signature})
	}
	scopes = append(scopes, types.LexicalTypeScope{Signature: methodSignature, Method: true, Descriptor: local.owner.descriptor}, types.LexicalTypeScope{Signature: localSignature})
	_, references, known := types.SignatureTypeVariableReferences(localSignature)
	if !known {
		return nil, false
	}
	bounds, known := types.LexicalTypeParameterErasures(scopes, references)
	if !known {
		return nil, false
	}
	for _, name := range references {
		if !c.FuncCtx.IsTypeParam(name) {
			return nil, false
		}
	}
	sigma := map[string]types.JavaType{}
	for i, name := range formals {
		if view.TypeArgs[i] == nil || types.IsWildcardType(view.TypeArgs[i]) {
			return nil, false
		}
		sigma[name] = view.TypeArgs[i]
	}
	params, ret := types.ParseMethodSignature(signature)
	if ret == nil || len(params) != len(physical) {
		return nil, false
	}
	for i, param := range params {
		params[i] = types.SubstituteTypeVars(param, sigma)
		erasure, known := nativeMethodLocalGenericParameterErasure(params[i], bounds, 0)
		if !known || erasure != physical[i] || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
	}
	return params, true
}

// Runtime erasure depends on a T-reference's original scope, not on whether a
// concrete class happens to share a visible formal's name.
func nativeMethodLocalGenericParameterErasure(t types.JavaType, bounds map[string]string, depth int) (string, bool) {
	if t == nil || depth > 64 {
		return "", false
	}
	if t.IsArray() {
		element, known := nativeMethodLocalGenericParameterErasure(t.ElementType(), bounds, depth+1)
		return "[" + element, known
	}
	if name, variable := types.TypeVariableName(t); variable {
		return bounds[name], bounds[name] != ""
	}
	if primitive, ok := t.RawType().(*types.JavaPrimer); ok {
		descriptor := map[string]string{"boolean": "Z", "byte": "B", "char": "C", "short": "S", "int": "I", "long": "J", "float": "F", "double": "D"}[primitive.Name]
		return descriptor, descriptor != ""
	}
	name, known := types.RawClassFQN(t)
	return "L" + strings.ReplaceAll(name, ".", "/") + ";", known
}
