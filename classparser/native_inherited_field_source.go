package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// nativeMemberInheritedFieldSourceSignature first retains the concrete declaration proof.
// For class-formal instance storage it additionally proves the source view used
// by the accessor: ((RawOwner) receiver).field must have the declaration erasure.
// JLS 4.8 erases the supertypes of a raw generic class. A nongeneric class is
// never raw, so its parameterized superclass starts a fresh source view. Merely
// knowing that the original field erases to Object would admit Base<String>
// writes and introduce a cast that the original accessor never performed.
//
// This proof adds no static or anonymous-owner projection. Bounded
// generic parents and member-type owner arguments need separate substitution
// proofs. Failure leaves the existing accessor proof closed.
func nativeMemberInheritedFieldSourceSignature(root, owner *ClassObject, field *MemberInfo, descriptor string, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) (bool, bool) {
	if root == nil || owner == nil || field == nil {
		return false, false
	}
	if generic, known := nativeMemberInheritedFieldSignature(owner, field, descriptor, work); known {
		return generic, true
	}
	if field.AccessFlags&8 != 0 || resolve == nil {
		return false, false
	}
	for _, a := range root.Attributes {
		if sourceProofNil(a) || !nativeProofWork(work, 1) {
			return false, false
		}
	}
	if !nativeMemberTopLevelEvidence(root, work) {
		return false, false
	}
	sig, known := nativeMethodLocalOriginalSignature(owner, field.Attributes, work)
	classSig, ck := nativeMethodLocalOriginalSignature(owner, owner.Attributes, work)
	if !known || !ck || sig == "" || len(sig) > 4096 || len(classSig) > 4096 || !nativeProofWork(work, int64(len(sig)+len(classSig))*130+1) || work != nil && work.CheckAlloc(int64(len(sig)+len(classSig))*256) != nil {
		return false, false
	}
	physical := 0
	for _, candidate := range owner.Fields {
		if !nativeProofWork(work, 1) {
			return false, false
		}
		if candidate == field {
			physical++
		}
	}
	if physical != 1 {
		return false, false
	}
	for _, a := range field.Attributes {
		if !nativeProofWork(work, 1) || sourceProofNil(a) {
			return false, false
		}
		if _, bad := a.(*ConstantValueAttribute); bad {
			return false, false
		}
	}
	erased, throws, valid := types.EraseClassBoundMethodSignatureWithThrows(classSig, "("+sig+")V")
	if !valid || len(throws) != 0 || erased != "("+descriptor+")V" {
		return false, false
	}
	current := root
	seen := map[string]bool{}
	raw := false
	first := true
	for depth := 0; depth < 64; depth++ {
		if current == nil || current.AccessFlags&0x0200 != 0 || seen[current.GetClassName()] || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(len(seen)+1)*256) != nil {
			return false, false
		}
		seen[current.GetClassName()] = true
		for _, a := range current.Attributes {
			if sourceProofNil(a) || !nativeProofWork(work, 1) {
				return false, false
			}
		}
		signature, ok := nativeMethodLocalOriginalSignature(current, current.Attributes, work)
		if !ok || len(signature) > 4096 {
			return false, false
		}
		var formals []string
		if signature != "" {
			if !nativeProofWork(work, int64(len(signature))*130+1) || work != nil && work.CheckAlloc(int64(len(signature))*256) != nil {
				return false, false
			}
			if _, _, valid := types.EraseLexicalMethodSignatureWithThrows(signature, "()V"); !valid {
				return false, false
			}
			formals = types.ClassFormalTypeParamNames(signature)
		}
		if first {
			raw = len(formals) > 0
			first = false
		}
		parentName := current.GetSupperClassName()
		if parentName == "" {
			return false, false
		}
		parameterized := false
		var arguments []types.JavaType
		if signature != "" {
			parent, interfaces := types.ParseClassSignatureSupers(signature)
			name, known := types.RawClassFQN(parent)
			if !known || strings.ReplaceAll(name, ".", "/") != parentName || len(interfaces) != len(current.Interfaces) {
				return false, false
			}
			for i, iface := range interfaces {
				name, known := types.RawClassFQN(iface)
				expected, ok := sourceBridgeClassName(current, current.Interfaces[i])
				if !known || !ok || strings.ReplaceAll(name, ".", "/") != expected {
					return false, false
				}
			}
			if pt, yes := types.AsParameterizedType(parent); yes {
				parameterized = true
				arguments = pt.TypeArgs
				if len(pt.OwnerSegments) > 1 {
					return false, false
				}
			}
		}
		if current.GetClassName() == owner.GetClassName() {
			if current != owner || !raw {
				return false, false
			}
			return true, true
		}
		parent, ok := resolve(parentName)
		if !ok || parent == nil || parent.GetClassName() != parentName {
			return false, false
		}
		ps, ok := nativeMethodLocalOriginalSignature(parent, parent.Attributes, work)
		if !ok || len(ps) > 4096 || !nativeProofWork(work, int64(len(ps))*130+1) || work != nil && work.CheckAlloc(int64(len(ps))*256) != nil {
			return false, false
		}
		if ps != "" {
			if _, _, valid := types.EraseLexicalMethodSignatureWithThrows(ps, "()V"); !valid {
				return false, false
			}
		}
		pf := types.ClassFormalTypeParamNames(ps)
		if parameterized && len(arguments) != len(pf) {
			return false, false
		}
		for _, argument := range arguments {
			if argument == nil || types.IsWildcardType(argument) {
				return false, false
			}
		}
		// Bounded parent substitutions need their own argument/bound proof. This
		// proof currently covers unbounded generic parents.
		if len(pf) > 0 && len(types.ClassFormalTypeParamBounds(ps, nil)) != 0 {
			return false, false
		}
		raw = len(pf) > 0 && (raw || !parameterized)
		current = parent
	}
	return false, false
}
