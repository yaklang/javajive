package javaclassparser

import "bytes"

// A static source root owns its private fields without importing its original
// outer's private scope. Keep their declarations and every original store in
// place. Existing family/accessor/archive closure still governs publication;
// this query grants no initialization motion or source access to the outer.
//
// Pre-nestmate children must use the original compiler's access protocols. A
// direct child CP reference to the root's private field is not such a protocol:
// turning it into Java lexical access would replace an original access failure
// with a new accessor. Walk original owner rows, never dollar-name prefixes.
func (c *ClassObjectDumper) nativeIndependentPrivateFieldsClosed(root *ClassObject) bool {
	if c == nil || root == nil || root != c.obj || len(root.Fields) > 4096 {
		return false
	}
	fields := map[string]bool{}
	for _, field := range root.Fields {
		if field == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		if field.AccessFlags&2 != 0 {
			name, named := sourceBridgeUTF8(root, field.NameIndex)
			descriptor, typed := sourceBridgeUTF8(root, field.DescriptorIndex)
			if !named || !typed || name == "" || descriptor == "" || fields[name+"\x00"+descriptor] {
				return false
			}
			fields[name+"\x00"+descriptor] = true
		}
	}
	if len(fields) == 0 {
		return true
	}
	if root.MinorVersion != 0 || root.MajorVersion < 49 || root.MajorVersion > 52 || c.foldSiblingResolver == nil {
		return false
	}
	before, known := constructorConfinedRootSnapshot(root)
	if !known || len(before) > 2<<20 || !nativeProofWork(c.Work, int64(len(before))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(before))*2+256) != nil {
		return false
	}
	snapshots := map[*ClassObject][]byte{root: before}
	totalSnapshotBytes := int64(len(before))
	owner := root.GetClassName()
	queue := []*ClassObject{root}
	seen := map[string]bool{owner: true}
	examined := map[string]*ClassObject{owner: root}
	for cursor := 0; cursor < len(queue); cursor++ {
		object := queue[cursor]
		if len(queue) > 256 || len(examined) > 1024 || !nativeProofWork(c.Work, 1) {
			return false
		}
		if object != root {
			for _, constant := range object.ConstantPool {
				if !nativeProofWork(c.Work, 1) {
					return false
				}
				ref, field := constant.(*ConstantFieldrefInfo)
				if !field {
					continue
				}
				if ref == nil {
					return false
				}
				declaring, known := sourceBridgeClassName(object, ref.ClassIndex)
				if !known {
					return false
				}
				if declaring != owner {
					continue
				}
				index := int(ref.NameAndTypeIndex)
				if index <= 0 || index > len(object.ConstantPool) {
					return false
				}
				nt, known := object.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
				if !known || nt == nil {
					return false
				}
				name, named := sourceBridgeUTF8(object, nt.NameIndex)
				descriptor, typed := sourceBridgeUTF8(object, nt.DescriptorIndex)
				if !named || !typed || fields[name+"\x00"+descriptor] {
					return false
				}
			}
		}
		current := object.GetClassName()
		for _, attribute := range object.Attributes {
			table, isTable := attribute.(*InnerClassesAttribute)
			if !isTable {
				continue
			}
			if table == nil || len(table.Classes) > 4096 {
				return false
			}
			for _, row := range table.Classes {
				if row == nil || !nativeProofWork(c.Work, 1) {
					return false
				}
				name, known := sourceBridgeClassName(object, row.InnerClassInfoIndex)
				if !known {
					return false
				}
				if name == current {
					continue
				}
				direct := false
				if row.OuterClassInfoIndex != 0 {
					enclosing, known := sourceBridgeClassName(object, row.OuterClassInfoIndex)
					if !known {
						return false
					}
					if enclosing != current {
						continue
					}
					direct = true
				}
				child := examined[name]
				if child == nil {
					raw, known := c.foldSiblingResolver(name)
					if !known || len(raw) > 2<<20 || !nativeProofWork(c.Work, int64(len(raw))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(raw))*2+128) != nil {
						return false
					}
					var err error
					child, err = c.parseResolved(raw)
					if err != nil || child == nil || child.GetClassName() != name {
						return false
					}
					examined[name] = child
				}
				if !direct {
					enclosing, _, known := originalAnonymousOwner(child)
					if !known || enclosing != current {
						continue
					}
				}
				if child.MinorVersion != 0 || child.MajorVersion != root.MajorVersion {
					return false
				}
				if !seen[name] {
					snapshot, valid := constructorConfinedRootSnapshot(child)
					if !valid || int64(len(snapshot)) > (8<<20)-totalSnapshotBytes || !nativeProofWork(c.Work, int64(len(snapshot))+1) {
						return false
					}
					totalSnapshotBytes += int64(len(snapshot))
					if c.Work != nil && c.Work.CheckAlloc(totalSnapshotBytes*2+int64(len(snapshots)+1)*64) != nil {
						return false
					}
					snapshots[child] = snapshot
					seen[name] = true
					queue = append(queue, child)
				}
			}
		}
	}
	// Resolving a later original may mutate an earlier model supplied to this
	// query. A component proof must retain all inspected inputs, not just root.
	for object, before := range snapshots {
		after, known := constructorConfinedRootSnapshot(object)
		if !known || !nativeProofWork(c.Work, int64(len(after))+1) || c.Work != nil && c.Work.CheckAlloc(totalSnapshotBytes+int64(len(after))) != nil || !bytes.Equal(before, after) {
			return false
		}
	}
	return true
}
