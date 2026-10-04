package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strconv"
)

// Walk authoritative named-member edges, never dollar-delimited binary names.
// A static declaration starts a new instance/type-variable scope. The bound
// and cycle check apply equally to malformed archives and ordinary input.
func nativeMemberLexicalAncestors(enclosing *ClassObject, objects map[string]*ClassObject, work *workbudget.Budget) ([]*ClassObject, bool) {
	if enclosing == nil {
		return nil, false
	}
	var out []*ClassObject
	seen := map[string]bool{}
	for node := enclosing; node != nil; {
		name := node.GetClassName()
		if len(out) >= 64 || seen[name] || !nativeProofWork(work, 1) || objects[name] != node {
			return nil, false
		}
		seen[name] = true
		out = append(out, node)
		owner, _, flags, member := originalMemberOwner(node)
		if !member {
			if !nativeMemberTopLevelEvidence(node, work) {
				return nil, false
			}
			return out, true
		}
		if flags&8 != 0 {
			return out, true
		}
		node = objects[owner]
		if node == nil {
			return nil, false
		}
	}
	return nil, false
}

func nativeMemberLexicalCaptureField(enclosing *ClassObject, objects map[string]*ClassObject, work *workbudget.Budget) (string, bool) {
	ancestors, ok := nativeMemberLexicalAncestors(enclosing, objects, work)
	if !ok {
		return "", false
	}
	depth := 0
	for _, node := range ancestors {
		_, _, flags, member := originalMemberOwner(node)
		if !member || flags&8 != 0 {
			break
		}
		depth++
	}
	return "this$" + strconv.Itoa(depth), true
}

func nativeMemberLexicalTypeScope(enclosing *ClassObject, objects map[string]*ClassObject, work *workbudget.Budget) (map[string]bool, bool) {
	ancestors, ok := nativeMemberLexicalAncestors(enclosing, objects, work)
	if !ok {
		return nil, false
	}
	scope := map[string]bool{}
	// Validate outer declarations before descendants. A nearer declaration
	// shadows an equally spelled outer formal; erased identities are retained
	// separately by the original ClassContext lexical signatures.
	for i := len(ancestors) - 1; i >= 0; i-- {
		obj := ancestors[i]
		sig := ""
		found := false
		for _, a := range obj.Attributes {
			if s, ok := a.(*SignatureAttribute); ok {
				if found || s == nil {
					return nil, false
				}
				found = true
				var known bool
				sig, known = sourceBridgeUTF8(obj, s.SignatureIndex)
				if !known || !nativeProofWork(work, int64(len(sig))) {
					return nil, false
				}
			}
		}
		if !found {
			continue
		}
		own, refs, known := types.SignatureTypeVariableReferences(sig)
		if !known {
			return nil, false
		}
		for _, name := range own {
			scope[name] = true
		}
		for _, name := range refs {
			if !scope[name] {
				return nil, false
			}
		}
	}
	return scope, true
}

func (z *JarFS) nativeMemberOutermostNamedOwner(owner string, work *workbudget.Budget) (string, bool) {
	seen := map[string]bool{}
	for depth := 0; depth < 64; depth++ {
		if seen[owner] || !nativeProofWork(work, 1) {
			return "", false
		}
		seen[owner] = true
		raw, known := z.enumSiblingResolver()(owner)
		if !known {
			return "", false
		}
		obj, err := z.nativeMemberReader(nil).parseResolved(raw)
		if err != nil || obj.GetClassName() != owner {
			return "", false
		}
		next, _, _, member := originalMemberOwner(obj)
		if !member {
			return owner, nativeMemberTopLevelEvidence(obj, work)
		}
		owner = next
	}
	return "", false
}

// An invalid/ambiguous self row must not be reinterpreted as evidence that a
// declaration is top level. Local and anonymous owners need their own complete
// lexical plan; missing ownership metadata does not start a new named forest.
func nativeMemberTopLevelEvidence(obj *ClassObject, work *workbudget.Budget) bool {
	if obj == nil {
		return false
	}
	for _, attr := range obj.Attributes {
		if !nativeProofWork(work, 1) {
			return false
		}
		if a, ok := attr.(*UnparsedAttribute); ok && a.Name == "EnclosingMethod" {
			return false
		}
		if table, ok := attr.(*InnerClassesAttribute); ok {
			if table == nil {
				return false
			}
			for _, row := range table.Classes {
				if row == nil || !nativeProofWork(work, 1) {
					return false
				}
				name, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
				if !known || name == obj.GetClassName() {
					return false
				}
			}
		}
	}
	return true
}
