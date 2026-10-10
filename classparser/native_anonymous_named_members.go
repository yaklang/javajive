package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// An otherwise empty named plan can still bind the declarations used by its
// anonymous source scopes. Discover an actual original enclosing-method edge;
// no arbitrary reference row or binary-name suffix grants a source role. The
// joint constructor, symbol, archive and final-source proofs decide publication.
func (c *ClassObjectDumper) nativeMemberHasAnonymousDeclarations() bool {
	if c.foldSiblingResolver == nil || c.obj == nil {
		return false
	}
	resolved := map[string]bool{}
	for _, attribute := range c.obj.Attributes {
		table, ok := attribute.(*InnerClassesAttribute)
		if !ok {
			continue
		}
		if table == nil {
			return false
		}
		for _, row := range table.Classes {
			if row == nil || !nativeProofWork(c.Work, 1) {
				return false
			}
			if row.InnerNameIndex != 0 {
				continue
			}
			name, known := sourceBridgeClassName(c.obj, row.InnerClassInfoIndex)
			if !known {
				return false
			}
			if resolved[name] {
				continue
			}
			if len(resolved) >= 64 || c.Work != nil && c.Work.CheckAlloc(int64(len(resolved)+1)*512) != nil {
				return false
			}
			resolved[name] = true
			raw, known := c.foldSiblingResolver(name)
			if !known {
				return false
			}
			object, err := c.parseResolved(raw)
			if err != nil || object.GetClassName() != name {
				return false
			}
			owner, method, anonymous := originalAnonymousOwner(object)
			if !anonymous || owner != c.obj.GetClassName() {
				continue
			}
			_, known = nativeAnonymousOriginalContext(c.obj, object, method, c.Work)
			return known
		}
	}
	return false
}

// Discover reciprocal named declarations inside an already proved anonymous
// expression. The inheritance graph does not grant lexical ownership. These
// nodes join the same transaction; a failed closure restores the named plan.
func (c *ClassObjectDumper) nativeAnonymousForestNamedMembers(forest *nativeAnonymousForest, enclosing *ClassObject) ([]*ClassObject, bool) {
	if forest == nil || enclosing == nil || forest.objects[enclosing.GetClassName()] != enclosing {
		return nil, false
	}
	p := forest.members
	var added []*ClassObject
	for _, attribute := range enclosing.Attributes {
		table, ok := attribute.(*InnerClassesAttribute)
		if !ok {
			continue
		}
		if table == nil {
			return nil, false
		}
		seen := map[string]bool{}
		for _, row := range table.Classes {
			if row == nil || !nativeProofWork(c.Work, 1) {
				return nil, false
			}
			if row.OuterClassInfoIndex == 0 {
				continue
			}
			owner, known := sourceBridgeClassName(enclosing, row.OuterClassInfoIndex)
			if !known {
				return nil, false
			}
			if owner != enclosing.GetClassName() {
				continue
			}
			name, known := sourceBridgeClassName(enclosing, row.InnerClassInfoIndex)
			if !known || seen[name] || row.InnerNameIndex == 0 {
				return nil, false
			}
			seen[name] = true
			if p == nil {
				return nil, false
			}
			if existing := p.children[name]; existing != nil {
				if existing.owner != owner || existing.object != forest.objects[name] {
					return nil, false
				}
				continue
			}
			// Only a proved anonymous node, or its original named descendant,
			// can extend the mixed forest. Unrelated reference rows stay external.
			if forest.units[owner] == nil && p.children[owner] == nil || len(p.children) >= nativeMemberLayoutNodeLimit || p.lexicalObjects[name] != nil || c.Work != nil && c.Work.CheckAlloc(int64(len(p.children)+len(forest.objects)+2)*512) != nil {
				return nil, false
			}
			raw, known := c.foldSiblingResolver(name)
			if !known {
				return nil, false
			}
			object, err := c.parseResolved(raw)
			if err != nil || object.GetClassName() != name || !nativeAnonymousForestVersion(object, c.Work) {
				return nil, false
			}
			reader := NewClassObjectDumper(object)
			reader.options, reader.Work = c.options, c.Work
			reader.foldSiblingResolver, reader.declarationResolver = c.foldSiblingResolver, c.declarationResolver
			p.lexicalObjects[name] = object
			child := nativeMemberProofWithDeclarations(object, enclosing, c.Work, reader.originalNativeConstructorAccessBridges(), p.lexicalObjects, c.nativeAnnotationDeclarationResolver(), reader.buildInvocationMetadata())
			simple, known := sourceBridgeUTF8(enclosing, row.InnerNameIndex)
			if child == nil || !reader.nativeMemberAnnotationTablesRepresentable() || !known || child.owner != owner || child.name != simple || child.flags != row.InnerClassAccessFlags {
				return nil, false
			}
			p.children[name] = child
			forest.objects[name] = object
			added = append(added, object)
		}
	}
	return added, true
}

// The anonymous owner cannot be named in Java source. A named descendant has
// a relative spelling only inside that exact original anonymous source scope.
func (p *nativeMemberFamily) anonymousNamedAnchor(binary string) string {
	seen := map[string]bool{}
	for child := p.children[strings.ReplaceAll(binary, ".", "/")]; child != nil; child = p.children[child.owner] {
		if child.sourceAnonymousOwner != "" {
			return child.sourceAnonymousOwner
		}
		if seen[child.owner] || len(seen) >= 64 {
			return ""
		}
		seen[child.owner] = true
		if object := p.lexicalObjects[child.owner]; object != nil {
			if _, _, anonymous := originalAnonymousOwner(object); anonymous {
				return child.owner
			}
		}
	}
	return ""
}

func (p *nativeMemberFamily) anonymousNamedScopeContains(anchor, caller string) bool {
	seen := map[string]bool{}
	for len(seen) < 64 && caller != "" && !seen[caller] {
		object := p.lexicalObjects[caller]
		if object == nil || object.GetClassName() != caller {
			return false
		}
		if caller == anchor {
			_, _, anonymous := originalAnonymousOwner(object)
			return anonymous
		}
		seen[caller] = true
		if owner, _, _, member := originalMemberOwner(object); member {
			caller = owner
		} else if owner, _, anonymous := originalAnonymousOwner(object); anonymous {
			caller = owner
		} else {
			return false
		}
	}
	return false
}

func nativeAnonymousForestNamedChild(forest *nativeAnonymousForest, object *ClassObject) *nativeMemberClass {
	if forest == nil || forest.members == nil || object == nil || forest.objects[object.GetClassName()] != object {
		return nil
	}
	child := forest.members.children[object.GetClassName()]
	if child == nil || child.object != object || child.static || forest.units[child.owner] == nil || forest.units[child.owner].object != forest.objects[child.owner] {
		return nil
	}
	return child
}

func nativeAnonymousForestNamedEnclosingDeclaration(forest *nativeAnonymousForest, object *ClassObject, member *MemberInfo, work *workbudget.Budget) bool {
	child := nativeAnonymousForestNamedChild(forest, object)
	if child == nil || member == nil || !nativeProofWork(work, 1) {
		return false
	}
	matches := 0
	for _, declaration := range append(append([]*MemberInfo{}, object.Fields...), object.Methods...) {
		if !nativeProofWork(work, 1) {
			return false
		}
		if declaration == member {
			matches++
		}
	}
	if matches != 1 {
		return false
	}
	name, nk := sourceBridgeUTF8(object, member.NameIndex)
	desc, dk := sourceBridgeUTF8(object, member.DescriptorIndex)
	if !nk || !dk {
		return false
	}
	if name == "<init>" {
		packet := child.constructors[desc]
		return packet != nil && packet.descriptor == desc && member.AccessFlags&0x1000 == 0
	}
	return name == child.field && member.AccessFlags == 0x1010 && desc == "L"+child.owner+";" && nativeMemberSuperCaptureDeclaration(child, work)
}

func nativeAnonymousForestNamedEnclosingNameType(forest *nativeAnonymousForest, object *ClassObject, index int, work *workbudget.Budget) bool {
	if forest == nil || forest.members == nil || object == nil || forest.objects[object.GetClassName()] != object || index <= 0 || index > len(object.ConstantPool) {
		return false
	}
	nt, ok := object.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
	if !ok || nt == nil {
		return false
	}
	name, nk := sourceBridgeUTF8(object, nt.NameIndex)
	desc, dk := sourceBridgeUTF8(object, nt.DescriptorIndex)
	if !nk || !dk {
		return false
	}
	if name == "<init>" {
		if !nativeProofWork(work, int64(len(desc))+1) {
			return false
		}
		params, result, err := callbinding.Descriptor(desc)
		if err != nil || result != "V" || len(params) == 0 || len(params[0]) < 3 || params[0][0] != 'L' || forest.units[params[0][1:len(params[0])-1]] == nil {
			return false
		}
	} else {
		child := nativeAnonymousForestNamedChild(forest, object)
		if child == nil || name != child.field || desc != "L"+child.owner+";" {
			return false
		}
	}
	used := false
	for _, constant := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		if ref := nativeConstantMember(constant); ref != nil && int(ref.NameAndTypeIndex) == index {
			owner, known := sourceBridgeClassName(object, ref.ClassIndex)
			child := nativeAnonymousForestNamedChild(forest, forest.objects[owner])
			if !known || child == nil {
				return false
			}
			if name == "<init>" {
				_, method := constant.(*ConstantMethodrefInfo)
				if !method || child.constructors[desc] == nil || !forest.members.anonymousNamedScopeContains(child.owner, object.GetClassName()) {
					return false
				}
			} else {
				_, field := constant.(*ConstantFieldrefInfo)
				if !field || object != child.object || name != child.field || desc != "L"+child.owner+";" || !nativeMemberSuperCaptureDeclaration(child, work) {
					return false
				}
			}
			used = true
		}
		switch entry := constant.(type) {
		case *ConstantInvokeDynamicInfo:
			if int(entry.NameAndTypeIndex) == index {
				return false
			}
		case *ConstantDynamicInfo:
			if int(entry.NameAndTypeIndex) == index {
				return false
			}
		}
	}
	return used
}

func nativeAnonymousForestNamedRow(forest *nativeAnonymousForest, object *ClassObject, row *InnerClassInfo) bool {
	if forest == nil || forest.members == nil || object == nil || row == nil {
		return false
	}
	name, nk := sourceBridgeClassName(object, row.InnerClassInfoIndex)
	owner, ok := sourceBridgeClassName(object, row.OuterClassInfoIndex)
	simple, sk := sourceBridgeUTF8(object, row.InnerNameIndex)
	child := forest.members.children[name]
	return nk && ok && sk && child != nil && child.owner == owner && child.name == simple && child.flags == row.InnerClassAccessFlags && forest.objects[name] == child.object && forest.units[owner] != nil
}
