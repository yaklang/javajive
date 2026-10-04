package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// Source reads of constant variables are inlined by javac. An original
// GETSTATIC (also through an inherited symbolic owner) can initialize the
// declaring class and its superclass. Until that separate call-site protocol
// is proved, newly committed inner constants require no original field refs.
func (z *JarFS) nativeMemberStaticConstantsReferencesClosed(p *nativeMemberFamily, index *nativeMemberIndex, work *workbudget.Budget) bool {
	if p == nil || index == nil || !index.valid || !nativeProofWork(work, 1) {
		return false
	}
	objects := map[string]*ClassObject{}
	load := func(name string) (*ClassObject, bool) {
		if o := objects[name]; o != nil {
			return o, true
		}
		if child := p.children[name]; child != nil {
			objects[name] = child.object
			return child.object, true
		}
		raw, known := z.enumSiblingResolver()(name)
		if !known {
			return nil, false
		}
		reader := z.nativeMemberReader(nil)
		o, e := reader.parseResolved(raw)
		if e != nil || o.GetClassName() != name {
			return nil, false
		}
		objects[name] = o
		return o, true
	}
	for owner, child := range p.children {
		if child == nil || child.object == nil || !nativeProofWork(work, 1) {
			return false
		}
		if child.static {
			continue
		}
		constants := map[string]bool{}
		for _, field := range child.object.Fields {
			if field == nil {
				return false
			}
			if !nativeProofWork(work, 1) {
				return false
			}
			if field.AccessFlags&8 == 0 {
				continue
			}
			name, nok := sourceBridgeUTF8(child.object, field.NameIndex)
			descriptor, dok := sourceBridgeUTF8(child.object, field.DescriptorIndex)
			// The assertion flag is regenerated from its separate runtime protocol,
			// never treated as an inlinable ConstantValue.
			if nok && dok && child.assertions != nil && name == nativeAssertionField && descriptor == "Z" {
				continue
			}
			if !nok || !dok || !nativeMemberStaticConstantField(child.object, field, work) {
				return false
			}
			constants[name+"\x00"+descriptor] = true
		}
		if len(constants) == 0 {
			continue
		}
		queue := []string{owner}
		seen := map[string]bool{owner: true}
		for cursor := 0; cursor < len(queue); cursor++ {
			if !nativeProofWork(work, 1) || len(queue) > 16384 {
				return false
			}
			alias := queue[cursor]
			users := index.typeUsers[alias]
			// The own class may have no CP type edge to itself, so include it explicitly.
			check := func(user string) bool {
				object, known := load(user)
				if !known {
					return false
				}
				if object.GetSupperClassName() == alias && !seen[user] {
					seen[user] = true
					queue = append(queue, user)
				}
				for _, constant := range object.ConstantPool {
					if !nativeProofWork(work, 1) {
						return false
					}
					ref, ok := constant.(*ConstantFieldrefInfo)
					if !ok {
						continue
					}
					if ref == nil {
						return false
					}
					symbolic, known := sourceBridgeClassName(object, ref.ClassIndex)
					if !known {
						return false
					}
					if symbolic != alias {
						continue
					}
					if ref.NameAndTypeIndex < 1 || int(ref.NameAndTypeIndex) > len(object.ConstantPool) {
						return false
					}
					nt, ok := object.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
					if !ok || nt == nil {
						return false
					}
					name, nok := sourceBridgeUTF8(object, nt.NameIndex)
					descriptor, dok := sourceBridgeUTF8(object, nt.DescriptorIndex)
					if !nok || !dok || constants[name+"\x00"+descriptor] {
						return false
					}
				}
				return true
			}
			if !check(alias) {
				return false
			}
			for user := range users {
				if !nativeProofWork(work, 1) || !check(user) {
					return false
				}
			}
		}
	}
	return true
}
