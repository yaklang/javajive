package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

// Every anonymous source scope is committed with its original lexical parent.
// Anonymous THIS stays unnameable. A read can only end at a proved lexical
// capture whose complete original dereference chain javac regenerates.
type nativeAnonymousForest struct {
	root              string
	groups            map[string]*nativeAnonymousFamily
	units             map[string]*nativeAnonymousClass
	objects           map[string]*ClassObject
	reads             map[string]map[string]map[int]*nativeMemberLexicalRead
	readPCs           map[string]map[string]map[int]bool
	anonymousTypes    map[string]bool
	captureReferences map[string]map[int]*nativeMemberLexicalRead
	members           *nativeMemberFamily
	lexicalThis       map[string]map[string]map[int]bool
}

func (c *ClassObjectDumper) planNativeAnonymousForest() *nativeAnonymousFamily {
	forest := c.planNativeAnonymousLexicalForest(nil)
	if forest == nil {
		return nil
	}
	return forest.groups[forest.root]
}

func (c *ClassObjectDumper) planNativeAnonymousLexicalForest(members *nativeMemberFamily) *nativeAnonymousForest {
	if c.foldSiblingResolver == nil || !nativeAnonymousForestVersion(c.obj, c.Work) || !nativeMemberTopLevelEvidence(c.obj, c.Work) {
		return nil
	}
	if _, _, anon := originalAnonymousOwner(c.obj); anon {
		return nil
	}
	modernNest, nestKnown := c.nativeModernNestOriginalScope()
	if !nestKnown {
		return nil
	}
	forest := &nativeAnonymousForest{root: c.obj.GetClassName(), groups: map[string]*nativeAnonymousFamily{}, units: map[string]*nativeAnonymousClass{}, objects: map[string]*ClassObject{c.obj.GetClassName(): c.obj}, reads: map[string]map[string]map[int]*nativeMemberLexicalRead{}, readPCs: map[string]map[string]map[int]bool{}, anonymousTypes: map[string]bool{}, captureReferences: map[string]map[int]*nativeMemberLexicalRead{}, members: members, lexicalThis: map[string]map[string]map[int]bool{}}
	queue := []*ClassObject{c.obj}
	if members != nil {
		if members.owner != forest.root {
			return nil
		}
		for name, child := range members.children {
			if !nativeAnonymousForestVersion(child.object, c.Work) || !nativeProofWork(c.Work, 1) {
				return nil
			}
			forest.objects[name] = child.object
			queue = append(queue, child.object)
		}
		// Enum bodies participate in the same source closure (including
		// private-constructor marker descriptors). Their allocations already
		// have a distinct enum certificate; include their original objects for
		// symbol/opcode/user closure without inventing an expression group.
		for name, body := range members.enumConstants {
			if body == nil || body.object == nil || members.lexicalObjects[name] != body.object || !nativeEnumConstantConstructorOwned(members, body.object, body.descriptor) || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(int64(len(forest.objects)+1)*512) != nil {
				return nil
			}
			if forest.objects[name] != nil {
				return nil
			}
			forest.objects[name] = body.object
		}
	}
	for cursor := 0; cursor < len(queue); cursor++ {
		object := queue[cursor]
		for _, attr := range object.Attributes {
			if table, ok := attr.(*InnerClassesAttribute); ok {
				if table == nil {
					return nil
				}
				for _, row := range table.Classes {
					if row == nil || !nativeProofWork(c.Work, 1) {
						return nil
					}
					if row.InnerNameIndex == 0 {
						name, known := sourceBridgeClassName(object, row.InnerClassInfoIndex)
						if !known {
							return nil
						}
						forest.anonymousTypes[name] = true
					}
				}
			}
		}
		reader := c
		if cursor != 0 {
			reader = NewClassObjectDumper(object)
			reader.options = c.options
			reader.Work = c.Work
			reader.foldSiblingResolver = c.foldSiblingResolver
			reader.declarationResolver = c.declarationResolver
		}
		has, closed := reader.nativeAnonymousForestHasChildren(members)
		if !closed {
			return nil
		}
		if !has {
			continue
		}
		group := reader.planNativeAnonymousGroup(members, forest)
		if group == nil {
			return nil
		}
		forest.groups[object.GetClassName()] = group
		for name, child := range group.children {
			if len(forest.units) >= 64 || forest.units[name] != nil || !nativeProofWork(c.Work, 1) || !nativeAnonymousForestVersion(child.object, c.Work) || !nativeAnonymousForestCaptureMetadata(child, c.Work) {
				return nil
			}
			forest.units[name] = child
			forest.objects[name] = child.object
			queue = append(queue, child.object)
		}
	}
	// Direct groups already tried the local proof. The complete forest also
	// proves reads that traverse named enclosing scopes, even without a nested
	// anonymous child. An empty forest still has nothing to commit.
	if len(forest.units) == 0 {
		return nil
	}
	if !nativeAnonymousForestCaptureReads(forest, c.Work, c.buildInvocationMetadata()) || !nativeAnonymousForestSymbolClosure(forest, c.Work) {
		return nil
	}
	for owner, group := range forest.groups {
		reader := NewClassObjectDumper(forest.objects[owner])
		reader.options = c.options
		reader.Work = c.Work
		reader.foldSiblingResolver = c.foldSiblingResolver
		reader.declarationResolver = c.declarationResolver
		if reader.validateNativeAnonymousGroup(group, members, forest) == nil {
			return nil
		}
	}
	if !nativeAnonymousForestOpcodeClosure(forest, c.Work) {
		return nil
	}
	if !nativeModernNestSourceScopeClosed(modernNest, forest.objects, c.Work) {
		return nil
	}
	return forest
}

func (c *ClassObjectDumper) nativeAnonymousForestHasChildren(members *nativeMemberFamily) (bool, bool) {
	found := false
	for _, a := range c.obj.Attributes {
		if table, ok := a.(*InnerClassesAttribute); ok {
			if table == nil {
				return false, false
			}
			for _, row := range table.Classes {
				if row == nil || !nativeProofWork(c.Work, 1) {
					return false, false
				}
				if row.InnerNameIndex != 0 {
					continue
				}
				name, known := sourceBridgeClassName(c.obj, row.InnerClassInfoIndex)
				if !known {
					return false, false
				}
				raw, known := c.foldSiblingResolver(name)
				if !known {
					continue
				}
				object, e := c.parseResolved(raw)
				if e != nil || object.GetClassName() != name {
					return false, false
				}
				owner, _, anon := originalAnonymousOwner(object)
				if anon && owner == c.obj.GetClassName() {
					// Discovery and grouping must agree on semantic ownership.
					// A proved private-access marker has no source anonymous
					// body; the joint constructor bridge regenerates its binary
					// identity. Revalidate the original object, not a synthetic
					// flag/name alone, before excluding this compiler artifact.
					// Real or unknown anonymous objects still require a group.
					if members != nil && members.emptyMarkers[name] != nil && nativeMemberEmptyAccessMarker(object, members.owner, c.Work) {
						continue
					}
					// Constant-specific enum classes also have unnamed rows and
					// EnclosingMethod metadata. Their source role belongs to the
					// independently proved enum allocation, not to an anonymous
					// expression group. Discovery must preserve that single owner.
					if c.nativeMemberEnumConstantAnonymousRole(members, object) {
						continue
					}
					found = true
				}
			}
		}
	}
	return found, true
}

// Only an exact owned child constructor can license its anonymous-parent type
// in a CP descriptor. Handles/dynamic/interface reuse of the same NT is refused.
func nativeAnonymousForestConstructorNameType(object *ClassObject, index int, forest *nativeAnonymousForest, work *workbudget.Budget) bool {
	if forest == nil || object == nil || index <= 0 || index > len(object.ConstantPool) {
		return false
	}
	nt, ok := object.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
	if !ok || nt == nil {
		return false
	}
	name, nok := sourceBridgeUTF8(object, nt.NameIndex)
	desc, dok := sourceBridgeUTF8(object, nt.DescriptorIndex)
	if !nok || !dok || name != "<init>" {
		return false
	}
	matched := false
	for _, constant := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		if member := nativeConstantMember(constant); member != nil && int(member.NameAndTypeIndex) == index {
			ref, normal := constant.(*ConstantMethodrefInfo)
			if !normal {
				return false
			}
			owner, known := sourceBridgeClassName(object, ref.ClassIndex)
			child := forest.units[owner]
			if !known || child == nil || child.descriptor != desc {
				return false
			}
			actual, _, known := originalAnonymousOwner(child.object)
			if !known || actual != object.GetClassName() {
				return false
			}
			matched = true
		}
		switch dynamic := constant.(type) {
		case *ConstantDynamicInfo:
			if int(dynamic.NameAndTypeIndex) == index {
				return false
			}
		case *ConstantInvokeDynamicInfo:
			if int(dynamic.NameAndTypeIndex) == index {
				return false
			}
		}
	}
	return matched
}

func nativeAnonymousForestEnclosingDeclaration(object *ClassObject, member *MemberInfo, forest *nativeAnonymousForest, work *workbudget.Budget) bool {
	if forest == nil || object == nil || member == nil || !nativeProofWork(work, 1) {
		return false
	}
	child := forest.units[object.GetClassName()]
	if child == nil || !child.parentAnonymous {
		return false
	}
	name, nok := sourceBridgeUTF8(object, member.NameIndex)
	desc, dok := sourceBridgeUTF8(object, member.DescriptorIndex)
	if !nok || !dok {
		return false
	}
	if name == "<init>" {
		return desc == child.descriptor && member.AccessFlags&0x1000 == 0
	}
	owner, _, known := originalAnonymousOwner(object)
	return known && name == child.enclosingField && member.AccessFlags == 0x1010 && desc == "L"+owner+";"
}

func nativeAnonymousForestOpcodeClosure(forest *nativeAnonymousForest, work *workbudget.Budget) bool {
	for owner, object := range forest.objects {
		for _, method := range object.Methods {
			if method == nil {
				return false
			}
			mn, nok := sourceBridgeUTF8(object, method.NameIndex)
			md, dok := sourceBridgeUTF8(object, method.DescriptorIndex)
			if !nok || !dok {
				return false
			}
			for _, a := range method.Attributes {
				code, ok := a.(*CodeAttribute)
				if !ok {
					continue
				}
				if !nativeProofWork(work, int64(len(code.Code))) {
					return false
				}
				decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
				decoder.Work = work
				if decoder.ParseOpcode() != nil {
					return false
				}
				for _, op := range decoder.Opcodes() {
					if op.Instr.OpCode == core.OP_NEW {
						if len(op.Data) != 2 {
							return false
						}
						name, known := sourceBridgeClassName(object, binary.BigEndian.Uint16(op.Data))
						if !known {
							return false
						}
						if forest.anonymousTypes[name] && forest.units[name] == nil && !nativeEnumConstantAllocationOwned(forest.members, object, method, int(op.CurrentOffset), name) {
							// A certified independent tail keeps its original flat
							// allocation. It contributes no lexical unit, capture
							// spelling or child suppression to this forest. Enum
							// bodies instead use their separately sealed <clinit>
							// allocation above; no other NEW inherits that role.
							group := forest.groups[owner]
							if group == nil || group.owner != owner || group.standalone[name] == nil || group.standalone[name].GetClassName() != name {
								return false
							}
						}
						if child := forest.units[name]; child != nil {
							group := forest.groups[owner]
							if group == nil || group.children[name] != child || child.newPC != int(op.CurrentOffset) || !nativeAnonymousAllocationScope(object, child, mn, md, work) {
								return false
							}
						}
					}
					if op.Instr.OpCode == core.OP_LDC || op.Instr.OpCode == core.OP_LDC_W || op.Instr.OpCode == core.OP_CHECKCAST || op.Instr.OpCode == core.OP_INSTANCEOF || op.Instr.OpCode == core.OP_ANEWARRAY || op.Instr.OpCode == core.OP_MULTIANEWARRAY {
						idx := uint16(0)
						if len(op.Data) == 1 && op.Instr.OpCode == core.OP_LDC {
							idx = uint16(op.Data[0])
						} else if len(op.Data) >= 2 {
							idx = binary.BigEndian.Uint16(op.Data)
						} else {
							return false
						}
						if typ, known := sourceBridgeClassName(object, idx); known {
							for name := range forest.units {
								if typ == name || strings.Contains(typ, "L"+name+";") {
									return false
								}
							}
						}
					}
					for _, kind := range []int{core.OP_INVOKEVIRTUAL, core.OP_INVOKESPECIAL, core.OP_INVOKESTATIC, core.OP_INVOKEINTERFACE, core.OP_GETFIELD, core.OP_PUTFIELD, core.OP_GETSTATIC, core.OP_PUTSTATIC} {
						symbol := constructorMotionMember(object, op, kind)
						if symbol == nil || forest.units[symbol.Name] == nil {
							continue
						}
						if symbol.Name == owner && symbol.Member != "<init>" {
							continue
						}
						child := forest.units[symbol.Name]
						group := forest.groups[owner]
						if kind == core.OP_GETFIELD && forest.readPCs[owner][mn+md][int(op.CurrentOffset)] {
							continue
						}
						if kind == core.OP_INVOKEVIRTUAL && (nativeAnonymousInheritedCall(forest, object, op, work) || nativeAnonymousDeclaredCall(forest, object, mn+md, op, work)) {
							continue
						}
						if kind != core.OP_INVOKESPECIAL || symbol.Member != "<init>" || group == nil || group.children[symbol.Name] != child || symbol.Description != child.descriptor || child.invokePC != int(op.CurrentOffset) {
							return false
						}
					}
				}
			}
		}
	}
	return true
}

func nativeAnonymousForestArchiveClosed(forest *nativeAnonymousForest, index *nativeMemberIndex, work *workbudget.Budget) bool {
	if forest == nil || index == nil || !index.valid {
		return false
	}
	for name, child := range forest.units {
		if !nativeProofWork(work, 1) || index.handles[name] {
			return false
		}
		for user := range index.typeUsers[name] {
			if forest.objects[user] == nil {
				return false
			}
		}
		if child.parentAnonymous {
			for user := range index.captureUsers[nativeMemberCaptureIndexKey(name, child.enclosingField)] {
				if forest.objects[user] == nil {
					return false
				}
			}
		}
	}
	return true
}

func (forest *nativeAnonymousForest) scopeSourceComplete(source string) bool {
	if forest == nil {
		return false
	}
	scopes := map[string]bool{}
	for owner := range forest.groups {
		scopes[owner] = true
	}
	if _, known := nativeAnonymousOrdinalsWithinOwner(source, "", scopes); !known {
		return false
	}
	for _, group := range forest.groups {
		if !group.completeOwnSource(source) {
			return false
		}
	}
	return true
}

func nativeAnonymousForestOwnChild(forest *nativeAnonymousForest, parent, name string) bool {
	if forest == nil {
		return false
	}
	group := forest.groups[parent]
	return group != nil && group.children[strings.ReplaceAll(name, ".", "/")] != nil
}

// Scope suppression changes all source type spellings together. An owned
// anonymous type may occur only in its own proved enclosing capture/constructor
// transfer, never in an ordinary source declaration, dynamic or foreign member.
func nativeAnonymousForestSymbolClosure(forest *nativeAnonymousForest, work *workbudget.Budget) bool {
	for _, object := range forest.objects {
		bridgeNameTypes := nativeMemberJointBridgeNameTypes(forest.members, object, work)
		for _, member := range append(append([]*MemberInfo{}, object.Fields...), object.Methods...) {
			if member == nil || !nativeProofWork(work, 1) {
				return false
			}
			descriptor, known := sourceBridgeUTF8(object, member.DescriptorIndex)
			if !known {
				return false
			}
			for name := range forest.units {
				if strings.Contains(descriptor, "L"+name+";") && !nativeAnonymousForestEnclosingDeclaration(object, member, forest, work) &&
					!(nativeAnonymousForestBridgeMarker(forest, name) && nativeMemberJointBridgeDeclaration(forest.members, object, member, name, work)) {
					return false
				}
			}
		}
		for index, constant := range object.ConstantPool {
			if !nativeProofWork(work, 1) {
				return false
			}
			if nt, ok := constant.(*ConstantNameAndTypeInfo); ok {
				if nt == nil {
					return false
				}
				descriptor, known := sourceBridgeUTF8(object, nt.DescriptorIndex)
				if !known {
					return false
				}
				for name := range forest.units {
					if strings.Contains(descriptor, "L"+name+";") && !nativeAnonymousForestConstructorNameType(object, index+1, forest, work) && !nativeAnonymousForestEnclosingNameType(object, index+1, forest, work) && !nativeAnonymousForestCaptureNameType(forest, object, index+1, work) &&
						!(nativeAnonymousForestBridgeMarker(forest, name) && bridgeNameTypes[index+1]) {
						return false
					}
				}
			}
		}
	}
	return true
}

// javac reuses the first root anonymous class as the unused private-constructor
// marker. Only the separately proved constructor packet may mention that type;
// ordinary declarations, method handles and other descriptor uses stay closed.
func nativeAnonymousForestBridgeMarker(forest *nativeAnonymousForest, name string) bool {
	if forest == nil || forest.members == nil || forest.members.owner != forest.root {
		return false
	}
	group := forest.groups[forest.root]
	if group == nil || group.owner != forest.root {
		return false
	}
	child := group.children[name]
	return child != nil && child.ordinal == 1 && forest.units[name] == child && child.object.GetClassName() == name
}

func nativeAnonymousForestEnclosingNameType(object *ClassObject, index int, forest *nativeAnonymousForest, work *workbudget.Budget) bool {
	if forest == nil || object == nil || index <= 0 || index > len(object.ConstantPool) {
		return false
	}
	child := forest.units[object.GetClassName()]
	if child == nil || !child.parentAnonymous {
		return false
	}
	nt, valid := object.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
	if !valid || nt == nil {
		return false
	}
	name, nok := sourceBridgeUTF8(object, nt.NameIndex)
	desc, dok := sourceBridgeUTF8(object, nt.DescriptorIndex)
	owner, _, known := originalAnonymousOwner(object)
	if !nok || !dok || !known || name != child.enclosingField || desc != "L"+owner+";" {
		return false
	}
	found := false
	for _, constant := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return false
		}
		if member := nativeConstantMember(constant); member != nil && int(member.NameAndTypeIndex) == index {
			field, valid := constant.(*ConstantFieldrefInfo)
			if !valid {
				return false
			}
			actual, known := sourceBridgeClassName(object, field.ClassIndex)
			if !known || actual != object.GetClassName() {
				return false
			}
			found = true
		}
		switch dynamic := constant.(type) {
		case *ConstantDynamicInfo:
			if int(dynamic.NameAndTypeIndex) == index {
				return false
			}
		case *ConstantInvokeDynamicInfo:
			if int(dynamic.NameAndTypeIndex) == index {
				return false
			}
		}
	}
	return found
}
