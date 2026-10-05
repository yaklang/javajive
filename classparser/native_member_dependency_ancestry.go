package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

// An inherited member type is in the subclass's declaration namespace, but
// its hidden enclosing instance and private access still belong to the original
// declaring class. This proof licenses only consulting that separate completed
// source family for its type spelling; it never imports a child into this one.
func nativeMemberAncestorDeclarationDependency(root, member *ClassObject, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if root == nil || member == nil || resolve == nil || !nativeProofWork(work, 1) {
		return false
	}
	owner, _, flags, known := originalMemberOwner(member)
	if !known || flags&8 != 0 || flags&2 != 0 || root.GetClassName() == owner {
		return false
	}
	packageName := func(name string) string {
		if i := strings.LastIndexByte(name, '/'); i >= 0 {
			return name[:i]
		}
		return ""
	}
	rootPackage, ownerPackage := packageName(root.GetClassName()), packageName(owner)
	if rootPackage != ownerPackage && (ownerPackage == "" || flags&(1|4) == 0) {
		return false
	}
	// Require the complete original ordinary-class superclass chain, including
	// identity, cycle, missing metadata and shared work/memory/cancel guards.
	return nativeMemberOriginalClassWidening(root.GetClassName(), owner, resolve, work)
}
