package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Original named ownership and static boundaries prove where a flat source
// declaration's formals came from. Dependencies and natively nested units
// retain lexical owners and must never take this positional projection.
func (c *ClassObjectDumper) buildSiblingSourceClassFormals() func(string) ([]string, bool) {
	if c.foldSiblingResolver == nil || c.getenv("JDEC_INNER_TYPEVAR_OFF") != "" || c.getenv("JDEC_INNER_ENCLOSING_ARITY_OFF") != "" {
		return nil
	}
	type entry struct {
		names []string
		known bool
	}
	cache := map[string]entry{}
	return func(internal string) ([]string, bool) {
		if hit, ok := cache[internal]; ok {
			return hit.names, hit.known
		}
		cache[internal] = entry{}
		if c.nativeMemberLookup != nil && c.nativeMemberLookup(internal) != nil || c.nativeMemberRoot != nil && c.nativeMemberRoot.allocationClass(internal) != nil {
			return nil, false
		}
		if c.FuncCtx != nil && c.FuncCtx.DeclarationSourceName != nil {
			if _, known := c.FuncCtx.DeclarationSourceName(internal); known {
				return nil, false
			}
		}
		raw, ok := c.foldSiblingResolver(internal)
		if !ok {
			return nil, false
		}
		obj, err := c.parseResolved(raw)
		if err != nil || obj.GetClassName() != internal {
			return nil, false
		}
		sig, ok := nativeMethodLocalOriginalSignature(obj, obj.Attributes, c.Work)
		if !ok || len(types.ClassFormalTypeParamNames(sig)) != 0 {
			return nil, false
		}
		current := obj
		seen := map[string]bool{}
		var nearest []string
		var stack []string
		for depth := 0; depth < 64; depth++ {
			name := current.GetClassName()
			if seen[name] || !nativeProofWork(c.Work, 1) {
				return nil, false
			}
			seen[name] = true
			s, known := nativeMethodLocalOriginalSignature(current, current.Attributes, c.Work)
			if !known {
				return nil, false
			}
			stack = append(stack, s)
			if current != obj && len(nearest) == 0 {
				nearest = types.ClassFormalTypeParamNames(s)
			}
			owner, _, flags, member := originalMemberOwner(current)
			if current == obj && (!member || flags&8 != 0) {
				return nil, false
			}
			if !member || flags&8 != 0 {
				for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
					stack[i], stack[j] = stack[j], stack[i]
				}
				if len(nearest) == 0 || !nativeMethodLocalBindingBudget(stack, "()V", c.Work) {
					return nil, false
				}
				if _, _, valid := types.EraseLexicalOwnerMethodSignatureWithThrows(stack, "()V"); !valid {
					return nil, false
				}
				inherited := map[string]bool{}
				for _, n := range nearest {
					inherited[n] = true
				}
				// The flat header adopts the nearest full ordered set only when every
				// class/field/parameter free reference belongs to it. The original scope
				// check also covers method returns, own method formals and static cuts.
				free := flattenedFreeTypeVariables(obj, sig, c.getenv("JDEC_INNER_METHODPARAM_TYPEVAR_INJECT_OFF") == "")
				if !typeNamesSubset(free, nearest) || !nativeMemberTypeScope(obj, inherited, c.Work) {
					return nil, false
				}
				names := append([]string(nil), nearest...)
				cache[internal] = entry{names, true}
				return names, true
			}
			b, known := c.foldSiblingResolver(owner)
			if !known {
				return nil, false
			}
			parent, e := c.parseResolved(b)
			if e != nil || parent.GetClassName() != owner {
				return nil, false
			}
			current = parent
		}
		return nil, false
	}
}

// The same reference order drives the flat class header and call-site proof.
func flattenedFreeTypeVariables(obj *ClassObject, signature string, methodParameters bool) []string {
	seen := map[string]bool{}
	var free []string
	add := func(names []string) {
		for _, n := range names {
			if n != "" && !seen[n] {
				seen[n] = true
				free = append(free, n)
			}
		}
	}
	add(types.FreeTypeVarRefsInClassSig(signature))
	for _, f := range obj.Fields {
		for _, a := range f.Attributes {
			if s, ok := a.(*SignatureAttribute); ok {
				raw, e := obj.getUtf8(s.SignatureIndex)
				if e == nil {
					add(types.TypeVarRefsInFieldSig(raw))
				}
				break
			}
		}
	}
	if methodParameters {
		for _, m := range obj.Methods {
			for _, a := range m.Attributes {
				if s, ok := a.(*SignatureAttribute); ok {
					raw, e := obj.getUtf8(s.SignatureIndex)
					if e == nil {
						add(types.TypeVarRefsInMethodParams(raw))
					}
					break
				}
			}
		}
	}
	return free
}
