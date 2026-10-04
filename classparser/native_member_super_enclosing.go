package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Traverse only actual enclosing captures in the jointly committed forest.
// The first SUPER operand must be a consecutive original slot1/GETFIELD chain
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
