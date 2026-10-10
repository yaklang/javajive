package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

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
	allocationForest, allocationKnown := c.nativeMemberAnonymousAllocationsRequireLexicalForest(p)
	if !allocationKnown {
		return false
	}
	if direct && !allocationForest && nativeMemberDirectAnonymousCapturesClosed(p, c.Work) &&
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

// A direct anonymous scope has no certificate for the enclosing operand of a
// named member created in its body. Such an allocation joins two source scopes
// and needs the same complete lexical forest as a private accessor or a read
// of another scope's capture. Discover this dependency from original NEWs.
func (c *ClassObjectDumper) nativeMemberAnonymousAllocationsRequireLexicalForest(p *nativeMemberFamily) (bool, bool) {
	work := c.Work
	if p == nil || !nativeProofWork(work, 1) {
		return false, false
	}
	for name, group := range p.anonymousUnits {
		if group == nil || group.children[name] == nil || group.children[name].object == nil || !nativeProofWork(work, 1) {
			return false, false
		}
		object := group.children[name].object
		reader := NewClassObjectDumper(object)
		reader.Work = work
		reader.foldSiblingResolver = c.foldSiblingResolver
		// A nested anonymous source scope can carry this same dependency.
		// Discover the original ownership edge before selecting a direct plan;
		// looking only at the first group's NEWs would miss its child's body.
		if nested, known := reader.nativeAnonymousForestHasChildren(p); !known {
			return false, false
		} else if nested {
			return true, true
		}
		for _, method := range object.Methods {
			if method == nil || !nativeProofWork(work, 1) {
				return false, false
			}
			for _, attribute := range method.Attributes {
				code, ok := attribute.(*CodeAttribute)
				if !ok {
					continue
				}
				if code == nil || !nativeProofWork(work, int64(len(code.Code))) {
					return false, false
				}
				decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
				decoder.Work = work
				if decoder.ParseOpcode() != nil {
					return false, false
				}
				for _, op := range decoder.Opcodes() {
					if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
						return false, false
					}
					if op.Instr.OpCode != core.OP_NEW {
						continue
					}
					if len(op.Data) != 2 {
						return false, false
					}
					owner, known := sourceBridgeClassName(object, core.Convert2bytesToInt(op.Data))
					if !known {
						return false, false
					}
					if child := p.children[owner]; child != nil && !child.static {
						return true, true
					}
				}
			}
		}
	}
	return false, true
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
