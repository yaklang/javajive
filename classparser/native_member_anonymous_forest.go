package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// Direct scope proofs establish capture ownership within each group. Recreating
// private accessors across named and anonymous scopes additionally needs one
// complete lexical forest so that every original call has a committed owner.
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
	direct := nativeJointAnonymousAllocationsClosed(c.obj, p.anonymous, c.Work, p) && add(p.anonymous)
	for name, child := range p.children {
		reader := NewClassObjectDumper(child.object)
		reader.Work = c.Work
		reader.options = c.options
		reader.foldSiblingResolver = c.foldSiblingResolver
		reader.declarationResolver = c.declarationResolver
		group := reader.planNativeAnonymousFamilyWithinMembers(p)
		if !nativeJointAnonymousAllocationsClosed(child.object, group, c.Work, p) || !add(group) {
			direct = false
		}
		p.memberAnonymous[name] = group
	}
	if direct && nativeMemberDirectAnonymousCapturesClosed(p, c.Work) &&
		(len(p.getters) == 0 || len(p.anonymousUnits) == 0) {
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

// A direct anonymous plan proves only its own captures and the separately
// supported enclosing SUPER operand. Body reads of named enclosing captures
// require the complete mixed forest, even when anonymous nesting is absent.
func nativeMemberDirectAnonymousCapturesClosed(p *nativeMemberFamily, work *workbudget.Budget) bool {
	if p == nil {
		return false
	}
	groups := map[*nativeAnonymousFamily]bool{}
	for _, group := range p.anonymousUnits {
		if group == nil || !nativeProofWork(work, 1) {
			return false
		}
		if groups[group] {
			continue
		}
		groups[group] = true
		for _, anonymous := range group.children {
			if anonymous == nil || anonymous.object == nil {
				return false
			}
			checked := map[string]bool{}
			for _, constant := range anonymous.object.ConstantPool {
				if !nativeProofWork(work, 1) {
					return false
				}
				field, ok := constant.(*ConstantFieldrefInfo)
				if !ok {
					continue
				}
				if field == nil || field.NameAndTypeIndex == 0 || int(field.NameAndTypeIndex) > len(anonymous.object.ConstantPool) {
					return false
				}
				owner, known := sourceBridgeClassName(anonymous.object, field.ClassIndex)
				nt, valid := anonymous.object.ConstantPool[field.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				if !known || !valid || nt == nil {
					return false
				}
				name, known := sourceBridgeUTF8(anonymous.object, nt.NameIndex)
				if !known {
					return false
				}
				if named := p.children[owner]; named != nil && !named.static && name == named.field && !checked[owner] {
					if !nativeMemberProjectedAnonymousCaptureRead(p, anonymous, owner, work) {
						return false
					}
					checked[owner] = true
				}
			}
		}
	}
	return true
}
