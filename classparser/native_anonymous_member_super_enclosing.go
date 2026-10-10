package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
	"sort"
)

// Walk only committed original enclosing captures. A same-typed field, foreign
// receiver or computed parameter is not the lexical enclosing instance.
func nativeAnonymousMemberSuperEnclosingPath(object *ClassObject, owner string, parent *nativeMemberClass, members *nativeMemberFamily, forest *nativeAnonymousForest, ops []*core.OpCode, start int, work *workbudget.Budget) (*nativeMemberLexicalRead, int, bool) {
	if object == nil || members == nil || members.failed || parent == nil || parent.static || parent.object == nil || members.anonymousSuperClass(parent.object.GetClassName()) != parent || start < 0 || start >= len(ops) || !constructorMotionLoad(ops[start], "L"+owner+";") || core.GetRetrieveIdx(ops[start]) != 1 {
		return nil, start, false
	}
	root := members.lexicalObjects[members.owner]
	if root == nil || root.GetClassName() != members.owner || !nativeMemberTopLevelEvidence(root, work) {
		return nil, start, false
	}
	declaring, _, _, known := originalMemberOwner(parent.object)
	if !known || declaring != parent.owner {
		return nil, start, false
	}
	if forest != nil && (forest.members != members || forest.root != members.owner || forest.objects[members.owner] != root) {
		return nil, start, false
	}
	var path *nativeMemberLexicalRead
	current := owner
	seen := map[string]bool{}
	next := start + 1
	for current != parent.owner {
		// An inherited member's declaring outer is a superclass of the
		// original enclosing object. Widening changes no physical operand:
		// it must terminate this exact slot-1/capture path, never introduce
		// a qualified NEW, cast, null check or independently captured value.
		if members.children[parent.object.GetClassName()] != parent && nativeMemberOriginalClassWidening(current, parent.owner, members.anonymousSuperResolver, work) {
			break
		}
		if len(seen) >= 64 || seen[current] || next >= len(ops) || !nativeProofWork(work, 1) {
			return nil, start, false
		}
		seen[current] = true
		fieldName, enclosing := "", ""
		if child := members.children[current]; child != nil {
			if child.object == nil || child.static || child.object.GetClassName() != current || !nativeMemberSuperCaptureDeclaration(child, work) {
				return nil, start, false
			}
			fieldName, enclosing = child.field, child.owner
		} else if forest != nil && forest.members == members && forest.units[current] != nil {
			child := forest.units[current]
			if child.object == nil || forest.objects[current] != child.object || child.enclosingField == "" || !nativeAnonymousForestCaptureMetadata(child, work) {
				return nil, start, false
			}
			var known bool
			enclosing, _, known = originalAnonymousOwner(child.object)
			if !known {
				return nil, start, false
			}
			fieldName = child.enclosingField
			param, captured := child.fields[fieldName]
			params, result, err := callbinding.Descriptor(child.descriptor)
			if !captured || param != 0 || err != nil || result != "V" || len(params) == 0 || params[0] != "L"+enclosing+";" {
				return nil, start, false
			}
		} else {
			return nil, start, false
		}
		field := constructorMotionMember(object, ops[next], core.OP_GETFIELD)
		if field == nil || field.Name != current || field.Member != fieldName || field.Description != "L"+enclosing+";" {
			return nil, start, false
		}
		read := &nativeMemberLexicalRead{owner: current, field: fieldName, descriptor: field.Description, pc: int(ops[next].CurrentOffset), prior: path}
		if path == nil {
			read.parameterOwner = owner
			read.basePC = int(ops[start].CurrentOffset)
		}
		path = read
		current = enclosing
		next++
	}
	if members.children[parent.object.GetClassName()] == parent && (members.lexicalObjects[parent.owner] == nil || members.lexicalObjects[parent.owner].GetClassName() != parent.owner) {
		return nil, start, false
	}
	return path, next, true
}

// Rebuild the complete slot-1 occurrence before importing constructor reads
// into the body forest. Cached lexical roles cannot license a second read.
func nativeAnonymousMemberSuperRead(unit *nativeAnonymousClass, forest *nativeAnonymousForest, desc string, ops []*core.OpCode, entries []int, work *workbudget.Budget) (*nativeMemberLexicalRead, bool) {
	if unit == nil || unit.object == nil || forest == nil || forest.members == nil || forest.members.failed || unit.memberSuper == nil || unit.memberEnclosingPath == nil || unit.descriptor != desc || forest.units[unit.object.GetClassName()] != unit || forest.objects[unit.object.GetClassName()] != unit.object {
		return nil, false
	}
	first := unit.memberEnclosingPath
	for depth := 0; first.prior != nil; depth++ {
		if depth >= 64 || !nativeProofWork(work, 1) {
			return nil, false
		}
		first = first.prior
	}
	owner, _, known := originalAnonymousOwner(unit.object)
	if !known || first.parameterOwner != owner {
		return nil, false
	}
	start := -1
	for i, op := range ops {
		if op == nil || op.Instr == nil || core.GetStoreIdx(op) == 1 || !nativeProofWork(work, 1) {
			return nil, false
		}
		if int(op.CurrentOffset) == first.basePC {
			if start >= 0 {
				return nil, false
			}
			start = i
		}
	}
	params, result, err := callbinding.Descriptor(desc)
	if err != nil || result != "V" || len(ops) > 512 || start != 3*len(unit.fields)+1 || start < 1 || !constructorMotionLoad(ops[start-1], "Ljava/lang/Object;") || core.GetRetrieveIdx(ops[start-1]) != 0 {
		return nil, false
	}
	slots := constructorParameterSlots(params)
	seenFields := map[string]bool{}
	for i := 0; i < start-1; i += 3 {
		field := constructorMotionMember(unit.object, ops[i+2], core.OP_PUTFIELD)
		if field == nil || field.Name != unit.object.GetClassName() || !nativeProofWork(work, 1) || !constructorMotionLoad(ops[i], "Ljava/lang/Object;") || core.GetRetrieveIdx(ops[i]) != 0 {
			return nil, false
		}
		param, known := slots[core.GetRetrieveIdx(ops[i+1])]
		recorded, captured := unit.fields[field.Member]
		if !known || !captured || param != recorded || param < 0 || param >= len(params) || seenFields[field.Member] || field.Description != params[param] || !constructorMotionLoad(ops[i+1], params[param]) || unit.capturePCs[field.Member] != int(ops[i+2].CurrentOffset) {
			return nil, false
		}
		seenFields[field.Member] = true
	}
	path, next, known := nativeAnonymousMemberSuperEnclosingPath(unit.object, owner, unit.memberSuper, forest.members, forest, ops, start, work)
	if !known || path == nil || next >= len(ops) {
		return nil, false
	}
	actual, cached := path, unit.memberEnclosingPath
	for depth := 0; actual != nil; depth++ {
		if depth >= 64 || cached == nil || !nativeProofWork(work, 1) || actual.owner != cached.owner || actual.field != cached.field || actual.descriptor != cached.descriptor || actual.pc != cached.pc || actual.basePC != cached.basePC || actual.parameterOwner != cached.parameterOwner {
			return nil, false
		}
		preceding := actual.basePC
		if actual.prior != nil {
			preceding = actual.prior.pc
		}
		entry := sort.SearchInts(entries, preceding+1)
		if entry < len(entries) && entries[entry] <= actual.pc {
			return nil, false
		}
		actual, cached = actual.prior, cached.prior
	}
	if cached != nil {
		return nil, false
	}
	// The original representation proof already binds every remaining SUPER
	// argument and marker. Keep its exact first call and physical descriptor.
	for i := next; i < len(ops); i++ {
		op := ops[i]
		if int(op.CurrentOffset) == unit.superPC {
			call := constructorMotionMember(unit.object, op, core.OP_INVOKESPECIAL)
			return path, call != nil && call.Name == unit.memberSuper.object.GetClassName() && call.Member == "<init>" && call.Description == unit.superDescriptor
		}
		if op.Instr.OpCode == core.OP_INVOKESPECIAL {
			return nil, false
		}
	}
	return nil, false
}
