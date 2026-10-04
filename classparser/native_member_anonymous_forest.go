package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// The existing direct-scope proof remains authoritative. A failed direct plan
// cannot suppress children; retry only as one complete named/anonymous forest.
func (c *ClassObjectDumper) planNativeMemberAnonymousScopes(p *nativeMemberFamily) bool {
	reset := func() {
		p.anonymous = nil
		p.anonymousUnits = map[string]*nativeAnonymousFamily{}
		p.memberAnonymous = map[string]*nativeAnonymousFamily{}
		p.anonymousForest = nil
	}
	reset()
	add := func(group *nativeAnonymousFamily) bool {
		if group == nil {
			return true
		}
		for name := range group.children {
			if len(p.anonymousUnits) >= 64 || p.anonymousUnits[name] != nil || !nativeProofWork(c.Work, 1) {
				return false
			}
			p.anonymousUnits[name] = group
		}
		return true
	}
	p.anonymous = c.planNativeAnonymousFamilyWithinMembers(p)
	direct := nativeJointAnonymousAllocationsClosed(c.obj, p.anonymous, c.Work) && add(p.anonymous)
	for name, child := range p.children {
		reader := NewClassObjectDumper(child.object)
		reader.Work = c.Work
		reader.options = c.options
		reader.foldSiblingResolver = c.foldSiblingResolver
		reader.declarationResolver = c.declarationResolver
		group := reader.planNativeAnonymousFamilyWithinMembers(p)
		if !nativeJointAnonymousAllocationsClosed(child.object, group, c.Work) || !add(group) {
			direct = false
		}
		p.memberAnonymous[name] = group
	}
	if direct {
		return true
	}
	reset()
	forest := c.planNativeAnonymousLexicalForest(p)
	if forest == nil {
		return false
	}
	p.anonymousForest = forest
	p.anonymous = forest.groups[p.owner]
	for name := range p.children {
		p.memberAnonymous[name] = forest.groups[name]
	}
	for _, group := range forest.groups {
		if !add(group) {
			reset()
			return false
		}
	}
	return true
}

func nativeMemberJointAnonymousForestOwner(p *nativeMemberFamily, group *nativeAnonymousFamily, work *workbudget.Budget) bool {
	if p == nil || group == nil || group.forest == nil || group.forest.members != p || p.anonymousForest != group.forest || group.forest.groups[group.owner] != group {
		return false
	}
	seen := map[string]bool{}
	for owner := group.owner; len(seen) < 64; {
		if seen[owner] || !nativeProofWork(work, 1) {
			return false
		}
		seen[owner] = true
		if owner == p.owner || p.children[owner] != nil {
			return true
		}
		node := group.forest.units[owner]
		if node == nil {
			return false
		}
		parent, _, known := originalAnonymousOwner(node.object)
		if !known {
			return false
		}
		owner = parent
	}
	return false
}
