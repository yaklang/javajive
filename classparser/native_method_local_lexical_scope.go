package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Original named ownership and static flags delimit lexical type scope. Keep
// signatures ordered by declaration instead of flattening equal-spelled names.
func nativeMethodLocalLexicalSignatures(enclosing *ClassObject, owner *nativeMethodLocalOwner, family *nativeMemberFamily, work *workbudget.Budget) ([]string, bool) {
	if enclosing == nil || owner == nil || owner.declaration == nil || owner.owner != enclosing.GetClassName() {
		return nil, false
	}
	if family != nil && (family.owner == "" || family.lexicalObjects[enclosing.GetClassName()] != enclosing) {
		return nil, false
	}
	found := false
	for _, m := range enclosing.Methods {
		if !nativeProofWork(work, 1) {
			return nil, false
		}
		if m == owner.declaration {
			found = true
		}
	}
	if !found {
		return nil, false
	}
	name, named := sourceBridgeUTF8(enclosing, owner.declaration.NameIndex)
	desc, typed := sourceBridgeUTF8(enclosing, owner.declaration.DescriptorIndex)
	if !named || !typed || name != owner.method || desc != owner.descriptor {
		return nil, false
	}

	if owner.declaration.AccessFlags&8 != 0 {
		return nil, true
	}
	signatures := []string{}
	current := enclosing
	seen := map[*ClassObject]bool{}
	for depth := 0; depth < 64; depth++ {
		if current == nil || seen[current] || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(depth+1)*128) != nil {
			return nil, false
		}
		if family != nil && family.lexicalObjects[current.GetClassName()] != current {
			return nil, false
		}
		seen[current] = true
		sig, ok := nativeMethodLocalOriginalSignature(current, current.Attributes, work)
		if !ok {
			return nil, false
		}
		signatures = append(signatures, sig)
		if family == nil || current.GetClassName() == family.owner {
			for i, j := 0, len(signatures)-1; i < j; i, j = i+1, j-1 {
				signatures[i], signatures[j] = signatures[j], signatures[i]
			}
			return signatures, true
		}
		child := family.children[current.GetClassName()]
		if child == nil || child.object != current || family.lexicalObjects[current.GetClassName()] != current {
			return nil, false
		}
		actual, name, flags, known := originalMemberOwner(current)
		if !known || actual != child.owner || name != child.name || flags != child.flags || child.static != (flags&8 != 0) {
			return nil, false
		}
		if child.static {
			for i, j := 0, len(signatures)-1; i < j; i, j = i+1, j-1 {
				signatures[i], signatures[j] = signatures[j], signatures[i]
			}
			return signatures, true
		}
		current = family.lexicalObjects[actual]
	}
	return nil, false
}

func nativeMethodLocalOriginalSignature(obj *ClassObject, attrs []AttributeInfo, work *workbudget.Budget) (string, bool) {
	result := ""
	seen := false
	for _, a := range attrs {
		if !nativeProofWork(work, 1) {
			return "", false
		}
		if s, ok := a.(*SignatureAttribute); ok {
			if seen || s == nil {
				return "", false
			}
			seen = true
			var valid bool
			result, valid = sourceBridgeUTF8(obj, s.SignatureIndex)
			if !valid || result == "" || !nativeProofWork(work, int64(len(result))) || work != nil && work.CheckAlloc(int64(len(result))*32) != nil {
				return "", false
			}
		}
	}
	return result, true
}

// Only the physically closed method owner can supply erasures for source
// captures. Existing renderer-only erasure remains conservative elsewhere.
func (c *ClassObjectDumper) nativeMethodLocalSourceErasure(t types.JavaType, local *nativeMethodLocalClass) (string, bool) {
	if c == nil || c.FuncCtx == nil || c.obj == nil || local == nil || local.owner == nil || c.obj.GetClassName() != local.owner.owner || c.FuncCtx.FunctionName != local.owner.method || c.FuncCtx.CurrentMethodDesc != local.owner.descriptor {
		return "", false
	}
	prefix := ""
	for depth := 0; depth < 32; depth++ {
		if t == nil || !nativeProofWork(c.Work, 1) {
			return "", false
		}
		if t.IsArray() {
			prefix += "["
			t = t.ElementType()
			continue
		}
		name, formal := types.RawClassFQN(t)
		if !formal || !c.FuncCtx.IsTypeParam(name) {
			d, ok := values.SourceTypeErasure(t, c.FuncCtx)
			return prefix + d, ok
		}
		classes, ok := nativeMethodLocalLexicalSignatures(c.obj, local.owner, c.nativeMemberRoot, c.Work)
		if !ok {
			return "", false
		}
		signature, ok := nativeMethodLocalOriginalSignature(c.obj, local.owner.declaration.Attributes, c.Work)
		if !ok {
			return "", false
		}
		if signature == "" {
			signature = local.owner.descriptor
		}
		if !nativeMethodLocalBindingBudget(classes, signature, c.Work) {
			return "", false
		}
		d, descriptor, ok := types.LexicalOwnerTypeVariableErasure(classes, signature, name)
		return prefix + d, ok && descriptor == local.owner.descriptor
	}
	return "", false
}

// Bound the derived environment work/storage before the grammar resolver
// snapshots declaration maps. The original UTF8 length alone understates a
// deep stack of inherited formals retained by successive owner declarations.
func nativeMethodLocalBindingBudget(classes []string, method string, work *workbudget.Budget) bool {
	if len(classes) > 64 {
		return false
	}
	bytes, names, cells := int64(len(method)), int64(0), int64(0)
	for _, sig := range classes {
		if len(sig) > 65535 || !nativeProofWork(work, int64(len(sig))+1) {
			return false
		}
		bytes += int64(len(sig))
		names += int64(len(types.ClassFormalTypeParamNames(sig)))
		cells += names
	}
	names += int64(len(types.MethodFormalTypeParamNames(method)))
	cells += names
	if bytes > 1<<20 || names > 512 {
		return false
	}
	return nativeProofWork(work, bytes*4+cells*4+names*128) && (work == nil || work.CheckAlloc(bytes*16+cells*96+names*128) == nil)
}
