package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// REF_invokeInterface names a directly declared public instance method using
// ConstantInterfaceMethodref, not ConstantMethodref. A static member interface
// keeps that physical receiver/descriptor contract under lexical regeneration.
// Concrete parameterized Signatures additionally prove their original erasure
// and throws; free/dependent formals need a different binding certificate.
// This grants no inherited resolution, private lookup or invokespecial access.
func nativeMemberInterfaceHandleSourceClosed(obj *ClassObject, member *nativeMemberClass, method *MemberInfo, target nativeMemberHandleTarget, work *workbudget.Budget) bool {
	if obj == nil || member == nil || method == nil || member.object != obj || !member.static || member.field != "" || member.formalCount != 0 || member.enumSynthesis != nil || target.kind != 9 || target.methodRef || !nativeProofWork(work, 1) {
		return false
	}
	owner, name, flags, known := originalMemberOwner(obj)
	if !known || flags&0x0608 != 0x0608 || owner != member.owner || name != member.name || flags != member.flags || !nativeMemberDeclarationKindRepresentable(obj, flags, work) {
		return false
	}
	if method.AccessFlags != 0x0401 && method.AccessFlags != 1 {
		return false
	}
	originalName, nk := sourceBridgeUTF8(obj, method.NameIndex)
	descriptor, dk := sourceBridgeUTF8(obj, method.DescriptorIndex)
	if !nk || !dk || originalName != target.name || descriptor != target.descriptor || originalName == "<init>" || originalName == "<clinit>" || class_context.SafeIdentifier(originalName) != originalName {
		return false
	}
	if _, _, err := callbinding.Descriptor(descriptor); err != nil {
		return false
	}
	// No class Signature is the ordinary nongeneric interface case. Unlike
	// a raw-generic-call proof, it needs no invented formal declaration.
	classSignature := ""
	classSignaturePresent := false
	for _, attribute := range obj.Attributes {
		if !nativeProofWork(work, 1) {
			return false
		}
		if a, ok := attribute.(*SignatureAttribute); ok {
			if a == nil || classSignaturePresent {
				return false
			}
			classSignaturePresent = true
			classSignature, known = sourceBridgeUTF8(obj, a.SignatureIndex)
			if !known || classSignature == "" || len(classSignature) > 4096 {
				return false
			}
		}
	}
	if !nativeProofWork(work, int64(len(classSignature))*130+1) || work != nil && work.CheckAlloc(int64(len(classSignature))*256) != nil {
		return false
	}
	if len(types.ClassFormalTypeParamNames(classSignature)) != 0 || !nativeMemberTypeScope(obj, nil, work) {
		return false
	}
	if classSignaturePresent {
		if _, _, valid := types.EraseLexicalMethodSignatureWithThrows(classSignature, descriptor); !valid {
			return false
		}
		super, interfaces := types.ParseClassSignatureSupers(classSignature)
		raw, named := types.RawClassFQN(super)
		if !named || strings.ReplaceAll(raw, ".", "/") != obj.GetSupperClassName() || len(interfaces) != len(obj.Interfaces) {
			return false
		}
		for i, parent := range interfaces {
			raw, named := types.RawClassFQN(parent)
			physical, known := sourceBridgeClassName(obj, obj.Interfaces[i])
			if !named || !known || strings.ReplaceAll(raw, ".", "/") != physical || !nativeProofWork(work, 1) {
				return false
			}
		}
	}
	signature := ""
	seen := false
	for _, attribute := range method.Attributes {
		if !nativeProofWork(work, 1) {
			return false
		}
		if a, ok := attribute.(*SignatureAttribute); ok {
			if a == nil || seen {
				return false
			}
			seen = true
			signature, known = sourceBridgeUTF8(obj, a.SignatureIndex)
			if !known || signature == "" || len(signature) > 4096 {
				return false
			}
		}
	}
	if !seen {
		return true
	}
	if !nativeProofWork(work, int64(len(signature))*130+1) || work != nil && work.CheckAlloc(int64(len(signature))*256) != nil {
		return false
	}
	erased, throws, valid := types.EraseConcreteMethodSignatureWithThrows(signature)
	return valid && erased == descriptor && nativeOriginalSignatureThrows(obj, method, throws, work)
}
