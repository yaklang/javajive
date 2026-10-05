package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Traverse only actual enclosing captures in the jointly committed forest.
// The first SUPER operand is either the same original slot-1 object through
// a proved class widening, or a consecutive original slot1/GETFIELD chain
// from the child's immediate enclosing object to the parent's declaring object.
func nativeMemberSuperEnclosingPath(child, parent *nativeMemberClass, p *nativeMemberFamily, ops []*core.OpCode, start int, work *workbudget.Budget) (*nativeMemberLexicalRead, bool) {
	if child == nil || parent == nil || p == nil || child.static || parent.static || start < 0 || start+1 >= len(ops) {
		return nil, false
	}
	if child.object == nil || parent.object == nil || p.children[child.object.GetClassName()] != child || p.children[parent.object.GetClassName()] != parent {
		return nil, false
	}
	enclosing := p.lexicalObjects[parent.owner]
	if enclosing == nil || enclosing.GetClassName() != parent.owner {
		return nil, false
	}
	if child.owner == parent.owner {
		return nil, true
	}
	if !constructorMotionLoad(ops[start], "Ljava/lang/Object;") || core.GetRetrieveIdx(ops[start]) != 0 ||
		!constructorMotionLoad(ops[start+1], "L"+child.owner+";") || core.GetRetrieveIdx(ops[start+1]) != 1 {
		return nil, false
	}
	// The parent's enclosing object can be the very same parameter viewed
	// through a superclass. Lexical capture traversal and class widening are
	// different relations; an inherited member does not require GETFIELD.
	// The downstream delegation proof still binds this exact slot-1 origin.
	if start+2 < len(ops) && ops[start+2] != nil && ops[start+2].Instr != nil && ops[start+2].Instr.OpCode != core.OP_GETFIELD && nativeMemberEnclosingClassWidening(child.owner, parent.owner, p, work) {
		return nil, true
	}
	owner := child.owner
	var path *nativeMemberLexicalRead
	seen := map[string]bool{}
	for i := start + 2; owner != parent.owner; i++ {
		if i >= len(ops) || len(seen) >= 64 || seen[owner] || !nativeProofWork(work, 1) {
			return nil, false
		}
		seen[owner] = true
		current := p.children[owner]
		if current == nil || current.static || current.object == nil || current.object.GetClassName() != owner {
			return nil, false
		}
		if !nativeMemberSuperCaptureDeclaration(current, work) {
			return nil, false
		}
		field := constructorMotionMember(child.object, ops[i], core.OP_GETFIELD)
		if field == nil || field.Name != owner || field.Member != current.field || field.Description != "L"+current.owner+";" {
			return nil, false
		}
		read := &nativeMemberLexicalRead{owner: field.Name, field: field.Member, descriptor: field.Description, pc: int(ops[i].CurrentOffset), prior: path}
		if path == nil {
			read.parameterOwner = child.owner
			read.basePC = int(ops[start+1].CurrentOffset)
		}
		path = read
		owner = current.owner
	}
	return path, path != nil
}

func nativeMemberSuperCaptureDeclaration(child *nativeMemberClass, work *workbudget.Budget) bool {
	if child == nil || child.object == nil || child.static || child.field == "" {
		return false
	}
	count := 0
	for _, field := range child.object.Fields {
		if field == nil || !nativeProofWork(work, 1) {
			return false
		}
		name, nk := sourceBridgeUTF8(child.object, field.NameIndex)
		desc, dk := sourceBridgeUTF8(child.object, field.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		if name != child.field {
			continue
		}
		flags, only, known := nativeMemberEffectiveFieldFlags(field, work)
		if !known || !only || flags != 0x1010 || desc != "L"+child.owner+";" {
			return false
		}
		count++
	}
	return count == 1
}

// Only original ordinary classes already in the joint lexical forest may
// establish this widening. Missing ancestors/interfaces/cycles cannot prove
// that the same enclosing object is valid for the parent's declaration.
func nativeMemberEnclosingClassWidening(from, to string, p *nativeMemberFamily, work *workbudget.Budget) bool {
	if p == nil {
		return false
	}
	return nativeMemberOriginalClassWidening(from, to, func(name string) (*ClassObject, bool) {
		obj := p.lexicalObjects[name]
		return obj, obj != nil
	}, work)
}

func nativeMemberOriginalClassWidening(from, to string, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if resolve == nil || from == "" || to == "" {
		return false
	}
	seen := map[string]bool{}
	for depth := 0; depth < 64; depth++ {
		if seen[from] || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(len(seen)+1)*128) != nil {
			return false
		}
		seen[from] = true
		obj, known := resolve(from)
		if !known || obj == nil || obj.GetClassName() != from || obj.AccessFlags&0x0200 != 0 {
			return false
		}
		if from == to {
			return true
		}
		from = obj.GetSupperClassName()
		if from == "" {
			return false
		}
	}
	return false
}
