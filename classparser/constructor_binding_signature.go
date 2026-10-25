package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A constructor Signature may omit the enclosing instance which its physical
// descriptor still carries. Lift that port only after original membership,
// capture identity and the complete payload erasure agree. This is a separate
// source binding view: the original Signature/descriptor remain immutable.
// A same-arity rival needs an overload proof, not an erased-bound cast removal.
func constructorEnclosingBindingSignature(object *ClassObject, method *MemberInfo, work *workbudget.Budget, resolver ...func(string) (*ClassObject, bool)) (string, bool) {
	return constructorEnclosingSignature(object, method, true, work, resolver...)
}

// A declaration is identified by its complete descriptor and original member,
// unlike an arity-only call-site query. Overloads do not make that declaration
// ambiguous. Keep the stronger arity requirement on the call-binding API above.
func constructorEnclosingDeclarationSignature(object *ClassObject, method *MemberInfo, work *workbudget.Budget, resolver ...func(string) (*ClassObject, bool)) (string, bool) {
	return constructorEnclosingSignature(object, method, false, work, resolver...)
}

func (c *ClassObjectDumper) constructorDeclarationHasEnclosingPort(method *MemberInfo, signature string) bool {
	if c == nil || c.obj == nil || method == nil {
		return false
	}
	// Require the same original Signature that the declaration writer consumed.
	// A field with a conventional compiler spelling cannot establish membership,
	// capture provenance, or the erasure of the remaining arguments.
	original, valid := nativeMethodLocalOriginalSignature(c.obj, method.Attributes, c.Work)
	if !valid || original != signature {
		return false
	}
	_, proved := constructorEnclosingDeclarationSignature(c.obj, method, c.Work, c.nativeAnnotationDeclarationResolver())
	return proved
}

func constructorEnclosingSignature(object *ClassObject, method *MemberInfo, uniqueArity bool, work *workbudget.Budget, resolver ...func(string) (*ClassObject, bool)) (string, bool) {
	if object == nil || method == nil || len(object.Methods) > 4096 || !nativeProofWork(work, int64(len(object.Methods))+1) {
		return "", false
	}
	owner, _, flags, valid := originalMemberOwner(object)
	if !valid || flags&8 != 0 {
		return "", false
	}
	descriptor, known := sourceBridgeUTF8(object, method.DescriptorIndex)
	params, result, err := callbinding.Descriptor(descriptor)
	if !known || err != nil || result != "V" || len(params) == 0 || params[0] != "L"+owner+";" {
		return "", false
	}
	matches := 0
	for _, m := range object.Methods {
		if m == nil {
			return "", false
		}
		name, nk := sourceBridgeUTF8(object, m.NameIndex)
		desc, dk := sourceBridgeUTF8(object, m.DescriptorIndex)
		if !nk || !dk {
			return "", false
		}
		if name != "<init>" {
			continue
		}
		ps, ret, e := callbinding.Descriptor(desc)
		if e != nil || ret != "V" {
			return "", false
		}
		if uniqueArity && len(ps) == len(params) || !uniqueArity && desc == descriptor {
			if m != method {
				return "", false
			}
			matches++
		}
	}
	if matches != 1 {
		return "", false
	}
	signature, valid := nativeMethodLocalOriginalSignature(object, method.Attributes, work)
	classSig, validClass := nativeMethodLocalOriginalSignature(object, object.Attributes, work)
	if signature == "" || !valid || !validClass || len(signature) > 4096 || len(classSig) > 4096 || !nativeProofWork(work, int64(len(signature)+len(classSig))*130+1) {
		return "", false
	}
	scopes := []types.LexicalTypeScope{{Signature: classSig}}
	erasure := func(sig string) (string, bool) {
		erased, _, valid := types.EraseLexicalScopedMethodSignatureWithThrows(scopes, sig)
		return erased, valid
	}
	erased, valid := erasure(signature)
	if !valid && len(resolver) == 1 && resolver[0] != nil {
		d := NewClassObjectDumper(object)
		d.Work = work
		var local bool
		resolve := func(name string) (*ClassObject, bool) {
			obj, known := resolver[0](name)
			return obj, known && obj != nil && obj.GetClassName() == name
		}
		scopes, local, valid = d.flattenedMethodLexicalScopesResolved(resolve)
		if !valid || local {
			return "", false
		}
		var signatures []string
		for _, scope := range scopes {
			signatures = append(signatures, scope.Signature)
		}
		if !nativeMethodLocalBindingBudget(signatures, signature, work) {
			return "", false
		}
		erased, valid = erasure(signature)
	}
	if !valid || erased != "("+strings.Join(params[1:], "")+")V" {
		return "", false
	}
	// Reuse the typed original initialization-cut proof. It accepts only own
	// final synthetic captures from physical parameter slots, followed by SUPER
	// and a closed suffix. A similarly named field or copied value is no proof.
	// The enclosing slot is transported through the entire THIS graph. Its
	// other operands can widen from a concrete allocation, factory result or
	// static field to an interface/superclass formal. Use the same original
	// declaration hierarchy as the source binder; discarding that resolver
	// incorrectly erases the constructor's complete generic declaration.
	var provider callbinding.Provider
	if len(resolver) == 1 && resolver[0] != nil {
		d := NewClassObjectDumper(object)
		d.Work = work
		provider = d.buildInvocationMetadataResolved(resolver[0])
	}
	packet := constructorLoweringTypedPacketFor(object, method, true, true, work, provider)
	captured := packet != nil && len(packet.plan.captures) == 1 && packet.plan.captures[0].slot == 1 && packet.plan.captures[0].field.descriptor == params[0]
	if !captured && !uniqueArity {
		proof := memberScopeConstructorGraph(object, owner, true, work, provider)[descriptor]
		captured = proof != nil && proof.forward != nil
	}
	if !captured {
		return "", false
	}
	start := strings.IndexByte(signature, '(')
	if start < 0 || work.CheckAlloc(int64(len(signature)+len(params[0]))) != nil {
		return "", false
	}
	lifted := signature[:start+1] + params[0] + signature[start+1:]
	actual, valid := erasure(lifted)
	return lifted, valid && actual == descriptor
}
