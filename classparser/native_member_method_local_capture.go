package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// A method-local declaration is a lexical participant, not a named member.
// Reconstruct its original declaration and physical capture packet before
// borrowing that scope. No binary-name prefix or cached source spelling grants
// ownership; placement and allocation closure remain mandatory at publication.
func nativeMemberJointMethodLocalOwner(p *nativeMemberFamily, obj *ClassObject, work *workbudget.Budget) (*nativeMethodLocalOwner, bool) {
	if p == nil || p.failed || obj == nil || len(p.methodLocals) > 64 || !nativeProofWork(work, 1) {
		return nil, false
	}
	if p.children[obj.GetClassName()] != nil {
		return nil, false
	}
	local := p.methodLocals[obj.GetClassName()]
	if local == nil || local.object != obj || local.owner == nil || local.constructor == nil || p.lexicalObjects[obj.GetClassName()] != obj {
		return nil, false
	}
	enclosing := p.lexicalObjects[local.owner.owner]
	if enclosing == nil || enclosing.GetClassName() != local.owner.owner || local.owner.owner != p.owner && (p.children[local.owner.owner] == nil || p.children[local.owner.owner].object != enclosing) {
		return nil, false
	}
	// Even a static local has a lexical access owner. Instance-depth evidence
	// alone stops at static declarations, so separately close the named chain
	// all the way to this compilation unit's actual root.
	current := enclosing.GetClassName()
	for depth := 0; current != p.owner; depth++ {
		if depth >= 64 || !nativeProofWork(work, 1) {
			return nil, false
		}
		node := p.children[current]
		if node == nil || node.object == nil || p.lexicalObjects[current] != node.object {
			return nil, false
		}
		parent, name, flags, known := originalMemberOwner(node.object)
		if !known || parent != node.owner || name != node.name || flags != node.flags || node.static != (flags&8 != 0) || p.lexicalObjects[parent] == nil {
			return nil, false
		}
		current = parent
	}
	root := p.lexicalObjects[p.owner]
	if root == nil || root.GetClassName() != p.owner || !nativeMemberTopLevelEvidence(root, work) {
		return nil, false
	}
	owner, known := originalMethodLocalOwner(obj, enclosing, work)
	if !known || *owner != *local.owner {
		return nil, false
	}
	actual, known := originalMethodLocalDefaultConstructor(obj, enclosing, work)
	cached := local.constructor
	if !known || actual.descriptor != cached.descriptor || actual.delegateOwner != cached.delegateOwner || actual.delegatePC != cached.delegatePC || actual.enclosingField != cached.enclosingField || len(actual.captures) != len(cached.captures) || len(actual.capturePCs) != len(cached.capturePCs) {
		return nil, false
	}
	for field, param := range actual.captures {
		index, found := cached.captures[field]
		pc, stored := cached.capturePCs[field]
		if !nativeProofWork(work, 1) || !found || !stored || index != param || pc != actual.capturePCs[field] {
			return nil, false
		}
	}
	if actual.enclosingField != "" {
		field, known := nativeMethodLocalEnclosingField(p, owner.owner, work)
		if !known || field != actual.enclosingField {
			return nil, false
		}
	}
	return owner, true
}
