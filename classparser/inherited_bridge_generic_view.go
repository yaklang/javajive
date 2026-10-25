package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Instantiate the exact superclass declaration in the child's original type
// environment. The source helper may have a narrower erasure than the physical
// invokespecial target; the original bridge's CHECKCAST must name precisely
// that narrower erasure. Assignability alone cannot authorize a weaker check.
func (c *ClassObjectDumper) inheritedBridgeGenericView(p *inheritedBridgePacket, parent *ClassObject, target *MemberInfo, signature string) bool {
	if p == nil || parent == nil || parent.GetClassName() != p.targetOwner || parent.GetClassName() != c.obj.GetSupperClassName() {
		return false
	}
	parentSig, pk := nativeOriginalClassSignature(parent, c.Work)
	childSig, ck := nativeOriginalClassSignature(c.obj, c.Work)
	if !pk || !ck || len(parentSig)+len(childSig)+len(signature) > 16384 || !nativeProofWork(c.Work, int64(len(parentSig)+len(childSig)+len(signature))*260+1) || c.Work.CheckAlloc(int64(len(parentSig)+len(childSig)+len(signature))*256) != nil || !nativeOriginalClassSignatureSupersClosed(parent, parentSig, c.Work) || !nativeOriginalClassSignatureSupersClosed(c.obj, childSig, c.Work) {
		return false
	}
	if len(types.MethodFormalTypeParamNames(signature)) != 0 {
		return false
	}
	erased, throws, valid := types.EraseClassBoundMethodSignatureWithThrows(parentSig, signature)
	if !valid || erased != p.targetDescriptor || !nativeOriginalSignatureThrows(parent, target, throws, c.Work) {
		return false
	}
	super, _ := types.ParseClassSignatureSupers(childSig)
	view, known := types.AsParameterizedType(super)
	formals := types.ClassFormalTypeParamNames(parentSig)
	if !known || strings.ReplaceAll(view.RawClassName, ".", "/") != p.targetOwner || len(view.OwnerSegments) > 1 || len(formals) == 0 || len(formals) > 128 || len(view.TypeArgs) != len(formals) {
		return false
	}
	// Binding uses the original lexical declaration graph, including static
	// method cuts. An enclosing-instance constructor receipt proves a different
	// obligation and cannot authorize or veto these method type variables.
	scopes, _, known := c.flattenedMethodLexicalScopesResolved(c.nativeAnnotationDeclarationResolver())
	if !known {
		return false
	}
	if _, _, known := types.EraseLexicalScopedMethodSignatureWithThrows(scopes, "()V"); !known {
		return false
	}
	sigma := map[string]types.JavaType{}
	for i, formal := range formals {
		arg := view.TypeArgs[i]
		if arg == nil || types.IsWildcardType(arg) {
			return false
		}
		sigma[formal] = arg
	}
	parameters, result := types.ParseMethodSignature(signature)
	targetArgs, targetResult, err := callbinding.Descriptor(p.targetDescriptor)
	bridgeArgs, _, bridgeErr := callbinding.Descriptor(p.descriptor)
	if err != nil || bridgeErr != nil || len(parameters) != len(targetArgs) || len(bridgeArgs) != len(parameters) || len(p.argumentCasts) != len(parameters) || result == nil {
		return false
	}
	// Only top-level erasures determine this temporary physical method. Keep
	// the complete substituted types for source rendering; do not use printed
	// names as evidence for variable binding or overload identity.
	var typeSignature func(types.JavaType, int) (string, bool)
	typeSignature = func(t types.JavaType, depth int) (string, bool) {
		if t == nil || depth > 64 || !nativeProofWork(c.Work, 1) {
			return "", false
		}
		if t.IsArray() {
			element, known := typeSignature(t.ElementType(), depth+1)
			return "[" + element, known
		}
		if n, variable := types.TypeVariableName(t); variable {
			return "T" + n + ";", true
		}
		if primitive, known := t.RawType().(*types.JavaPrimer); known {
			desc := map[string]string{"boolean": "Z", "byte": "B", "char": "C", "short": "S", "int": "I", "long": "J", "float": "F", "double": "D", "void": "V"}[primitive.Name]
			return desc, desc != ""
		}
		name, known := types.RawClassFQN(t)
		return "L" + strings.ReplaceAll(name, ".", "/") + ";", known && nativeSourceBinaryName(strings.ReplaceAll(name, ".", "/"))
	}
	params := make([]types.JavaType, len(parameters))
	var parts []string
	for i, t := range parameters {
		params[i] = types.SubstituteTypeVars(t, sigma)
		s, known := typeSignature(params[i], 0)
		if !known {
			return false
		}
		parts = append(parts, s)
	}
	ret, known := typeSignature(types.SubstituteTypeVars(result, sigma), 0)
	if !known {
		return false
	}
	source, _, known := types.EraseLexicalScopedMethodSignatureWithThrows(scopes, "("+strings.Join(parts, "")+")"+ret)
	actualArgs, actualResult, err := callbinding.Descriptor(source)
	if !known || err != nil || actualResult != targetResult || len(actualArgs) != len(targetArgs) {
		return false
	}
	widening := newConstructorWideningQuery(c.buildInvocationMetadata())
	for i, actual := range actualArgs {
		checked := p.argumentCasts[i]
		if checked == "" {
			checked = bridgeArgs[i]
		}
		if checked != actual || !widening.assignable(actual, targetArgs[i]) {
			return false
		}
	}
	for _, m := range c.obj.Methods {
		n, nk := sourceBridgeUTF8(c.obj, m.NameIndex)
		d, dk := sourceBridgeUTF8(c.obj, m.DescriptorIndex)
		if !nk || !dk || !nativeProofWork(c.Work, 1) || n == p.name && d == source {
			return false
		}
	}
	p.sourceDescriptor, p.sourceParameters = source, params
	return true
}
