package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

// A member type can be named from an unrelated source family. This establishes
// only an accessible original declaration path: the separate dependency must
// still complete its source transaction. No field, enclosing instance, private
// constructor, accessor or registration ordinal becomes owned by the caller.
func nativeMemberIndependentDeclarationDependency(root, member *ClassObject, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if root == nil || member == nil || !nativeMemberDeclarationMetadataBounded(root, work) || !nativeMemberDeclarationMetadataBounded(member, work) {
		return false
	}
	owner, _, flags, named := originalMemberOwner(member)
	if !named || flags&8 != 0 || owner == root.GetClassName() {
		return false
	}
	return nativeMemberOriginalDeclarationPath(root, member, resolve, work)
}

// Validate every named declaration edge, including a root's own member.
// Whether a caller may import an independent dependency is a separate rule.
func nativeMemberOriginalDeclarationPath(root, member *ClassObject, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if root == nil || member == nil || resolve == nil || !nativeMemberDeclarationMetadataBounded(root, work) || !nativeMemberDeclarationMetadataBounded(member, work) || !nativeMemberTopLevelEvidence(root, work) {
		return false
	}
	pkg := func(name string) string {
		if i := strings.LastIndexByte(name, '/'); i >= 0 {
			return name[:i]
		}
		return ""
	}
	rootPackage := pkg(root.GetClassName())
	seen := map[string]bool{}
	for node := member; node != nil; {
		name := node.GetClassName()
		if len(seen) >= 64 || seen[name] || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(len(seen)+1)*128) != nil {
			return false
		}
		seen[name] = true
		// A named package cannot refer to a declaration in the unnamed
		// package, even when every enclosing declaration is public.
		if rootPackage != "" && pkg(name) == "" {
			return false
		}
		if !nativeMemberDeclarationMetadataBounded(node, work) {
			return false
		}
		parent, local, access, named := originalMemberOwner(node)
		if !named {
			return nativeMemberTopLevelEvidence(node, work) && (pkg(name) == rootPackage || node.AccessFlags&1 != 0)
		}
		// Cross-package protected names require the distinct original ancestry
		// proof. An unrelated declaration cannot acquire that lexical privilege.
		if access&2 != 0 || pkg(name) != rootPackage && access&1 == 0 || len(node.Attributes) > 65535 || !nativeProofWork(work, int64(len(node.Attributes))) {
			return false
		}
		original, known := resolve(parent)
		if !known || original == nil || original.GetClassName() != parent || len(original.Attributes) > 65535 || !nativeProofWork(work, int64(len(original.Attributes))) {
			return false
		}
		// Both original declarations must agree on this very edge. A forged
		// self row or a same-spelled foreign object grants no source spelling.
		matches := 0
		for _, attribute := range original.Attributes {
			if table, ok := attribute.(*InnerClassesAttribute); ok {
				if table == nil || len(table.Classes) > 65535 || !nativeProofWork(work, int64(len(table.Classes))) {
					return false
				}
				for _, row := range table.Classes {
					if row == nil {
						return false
					}
					child, known := sourceBridgeClassName(original, row.InnerClassInfoIndex)
					if !known {
						return false
					}
					if child != name {
						continue
					}
					outer, ok := sourceBridgeClassName(original, row.OuterClassInfoIndex)
					inner, valid := sourceBridgeUTF8(original, row.InnerNameIndex)
					if !ok || !valid || outer != parent || inner != local || row.InnerClassAccessFlags != access {
						return false
					}
					matches++
				}
			}
		}
		if matches != 1 {
			return false
		}
		node = original
	}
	return false
}

func nativeMemberDeclarationMetadataBounded(obj *ClassObject, work *workbudget.Budget) bool {
	if obj == nil || len(obj.Attributes) > 65535 || !nativeProofWork(work, int64(len(obj.Attributes))+1) {
		return false
	}
	for _, attribute := range obj.Attributes {
		if table, ok := attribute.(*InnerClassesAttribute); ok {
			if table == nil || len(table.Classes) > 65535 || !nativeProofWork(work, int64(len(table.Classes))) {
				return false
			}
		}
	}
	return true
}
