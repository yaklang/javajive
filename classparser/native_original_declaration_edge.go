package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// A reciprocal original edge establishes identity, not lexical accessibility.
func nativeOriginalDeclarationEdge(parent, child *ClassObject, work *workbudget.Budget) bool {
	if parent == nil || child == nil || !nativeMemberDeclarationMetadataBounded(parent, work) || !nativeMemberDeclarationMetadataBounded(child, work) {
		return false
	}
	owner, name, flags, named := originalMemberOwner(child)
	if !named || parent.GetClassName() != owner {
		return false
	}
	matches := 0
	for _, attribute := range parent.Attributes {
		if table, ok := attribute.(*InnerClassesAttribute); ok {
			for _, row := range table.Classes {
				if row == nil || !nativeProofWork(work, 1) {
					return false
				}
				binary, known := sourceBridgeClassName(parent, row.InnerClassInfoIndex)
				if !known || !nativeProofWork(work, int64(len(binary))) {
					return false
				}
				if binary != child.GetClassName() {
					continue
				}
				outer, ok := sourceBridgeClassName(parent, row.OuterClassInfoIndex)
				simple, valid := sourceBridgeUTF8(parent, row.InnerNameIndex)
				if !ok || !valid || outer != owner || simple != name || row.InnerClassAccessFlags != flags {
					return false
				}
				matches++
			}
		}
	}
	return matches == 1
}
