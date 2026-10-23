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
func constructorEnclosingBindingSignature(object *ClassObject, method *MemberInfo, work *workbudget.Budget) (string, bool) {
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
		if len(ps) == len(params) {
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
	erased, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{classSig}, signature)
	if !valid || erased != "("+strings.Join(params[1:], "")+")V" {
		return "", false
	}
	// Reuse the typed original initialization-cut proof. It accepts only own
	// final synthetic captures from physical parameter slots, followed by SUPER
	// and a closed suffix. A similarly named field or copied value is no proof.
	packet := constructorLoweringPacketFor(object, method, true, true, work)
	if packet == nil || len(packet.plan.captures) != 1 || packet.plan.captures[0].slot != 1 || packet.plan.captures[0].field.descriptor != params[0] {
		return "", false
	}
	start := strings.IndexByte(signature, '(')
	if start < 0 || work.CheckAlloc(int64(len(signature)+len(params[0]))) != nil {
		return "", false
	}
	lifted := signature[:start+1] + params[0] + signature[start+1:]
	actual, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows([]string{classSig}, lifted)
	return lifted, valid && actual == descriptor
}
