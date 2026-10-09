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

// Type accessibility belongs to the referencing declaration and its proved
// lexical enclosing declarations, not just the compilation unit's root. A
// nested subclass can inherit a protected member type even when its outer root
// is unrelated; declarations nested inside that subclass share its type scope.
// Consult only original ancestry and the already prepared named scope chain.
// This contributes a type spelling, never foreign private/constructor ownership.
func nativeMemberLexicalAncestorDeclarationDependency(prepared *nativeMemberPrepared, object, member *ClassObject, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if prepared == nil || prepared.root == nil || prepared.family == nil || prepared.family.failed || object == nil || prepared.objects[object.GetClassName()] != object || prepared.objects[prepared.family.owner] != prepared.root || prepared.root.GetClassName() != prepared.family.owner || !nativeProofWork(work, 1) {
		return false
	}
	if nativeMemberAncestorDeclarationDependency(object, member, resolve, work) || object != prepared.root && nativeMemberAncestorDeclarationDependency(prepared.root, member, resolve, work) {
		return true
	}
	if prepared.family.children[object.GetClassName()] == nil {
		return false
	}
	scopes, known := nativeMemberJointNamedScope(prepared, object.GetClassName(), work)
	if !known {
		return false
	}
	// Walk nearest-to-farthest in lexical order, not map iteration order, so
	// the shared proof budget has deterministic behavior.
	for name := prepared.family.children[object.GetClassName()].owner; name != prepared.family.owner; {
		node := prepared.family.children[name]
		if !scopes[name] || node == nil || !nativeProofWork(work, 1) {
			return false
		}
		if nativeMemberAncestorDeclarationDependency(prepared.objects[name], member, resolve, work) {
			return true
		}
		name = node.owner
	}
	return false
}
