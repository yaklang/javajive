package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A nonstatic named member of a raw enclosing type is itself raw (JLS 4.8).
// Resolve free class variables in the exact original lexical declarations,
// retaining each formal's declaration environment through inner shadowing.
// The returned path forces a raw OUTER qualifier: a simple inner cast within
// a parameterized outer would leave the outer variables parameterized.
func nativeMemberRawLexicalPrivateCall(owner *ClassObject, signature, descriptor string, target *MemberInfo, objects map[string]*ClassObject, work *workbudget.Budget) ([]string, bool) {
	if owner == nil || target == nil || target.AccessFlags&0x000a != 2 || objects[owner.GetClassName()] != owner || len(signature) > 4096 {
		return nil, false
	}
	classes, path, known := nativeMemberOriginalLexicalScope(owner, objects, work)
	if !known || len(classes) < 2 || !nativeMethodLocalBindingBudget(classes, signature, work) {
		return nil, false
	}
	erased, throws, known := types.EraseLexicalOwnerMethodSignatureWithThrows(classes, signature)
	if !known || erased != descriptor || !nativeOriginalSignatureThrows(owner, target, throws, work) {
		return nil, false
	}
	return path, true
}

// Source type binding and raw-call projection share the same original scope
// certificate: pointer-owned named edges, reciprocal rows and physical supers.
// The signatures and source path are returned outer-to-inner after static cuts.
func nativeMemberOriginalLexicalScope(owner *ClassObject, objects map[string]*ClassObject, work *workbudget.Budget) ([]string, []string, bool) {
	if owner == nil || objects[owner.GetClassName()] != owner {
		return nil, nil, false
	}
	ancestors, known := nativeMemberLexicalAncestors(owner, objects, work)
	if !known || len(ancestors) == 0 || work != nil && work.CheckAlloc(int64(len(ancestors))*128) != nil {
		return nil, nil, false
	}
	classes := make([]string, 0, len(ancestors))
	path := make([]string, 0, len(ancestors))
	for i := len(ancestors) - 1; i >= 0; i-- {
		obj := ancestors[i]
		sig, valid := nativeMethodLocalOriginalSignature(obj, obj.Attributes, work)
		if !valid || len(sig) > 4096 {
			return nil, nil, false
		}
		if sig != "" {
			// Bound nested-signature parsing before constructing its type trees.
			if !nativeProofWork(work, int64(len(sig))*130+1) || work != nil && work.CheckAlloc(int64(len(sig))*256) != nil {
				return nil, nil, false
			}
			super, interfaces := types.ParseClassSignatureSupers(sig)
			raw, valid := types.RawClassFQN(super)
			if !valid || strings.ReplaceAll(raw, ".", "/") != obj.GetSupperClassName() || len(interfaces) != len(obj.Interfaces) {
				return nil, nil, false
			}
			for j, it := range interfaces {
				raw, valid := types.RawClassFQN(it)
				physical, known := sourceBridgeClassName(obj, obj.Interfaces[j])
				if !valid || !known || strings.ReplaceAll(raw, ".", "/") != physical || !nativeProofWork(work, 1) {
					return nil, nil, false
				}
			}
		}
		classes = append(classes, sig)
		if i == len(ancestors)-1 {
			path = append(path, obj.GetClassName())
			continue
		}
		outer, name, flags, valid := originalMemberOwner(obj)
		if !valid || flags&8 != 0 || outer != ancestors[i+1].GetClassName() {
			return nil, nil, false
		}
		// Reciprocal original rows certify the edge; a copied name or self-only row
		// cannot import an unrelated declaration's formals into this method.
		parent := ancestors[i+1]
		matches := 0
		for _, a := range parent.Attributes {
			if table, ok := a.(*InnerClassesAttribute); ok {
				if table == nil {
					return nil, nil, false
				}
				for _, row := range table.Classes {
					if row == nil || !nativeProofWork(work, 1) {
						return nil, nil, false
					}
					binary, known := sourceBridgeClassName(parent, row.InnerClassInfoIndex)
					if !known {
						return nil, nil, false
					}
					if binary != obj.GetClassName() {
						continue
					}
					parentName, known := sourceBridgeClassName(parent, row.OuterClassInfoIndex)
					childName, named := sourceBridgeUTF8(parent, row.InnerNameIndex)
					if !known || !named || parentName != outer || childName != name || row.InnerClassAccessFlags != flags {
						return nil, nil, false
					}
					matches++
				}
			}
		}
		if matches != 1 {
			return nil, nil, false
		}
		path = append(path, name)
	}
	return classes, path, true
}
