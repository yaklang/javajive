package javaclassparser

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

type nativeAnonymousClass struct {
	enclosingField        string
	parentAnonymous       bool
	capturePCs            map[string]int
	object                *ClassObject
	descriptor            string
	method                string
	ordinal               int
	fields                map[string]int
	superParams           []int
	superDescriptor       string
	sourceSuperDescriptor string
	superPC               int
	newPC                 int
	invokePC              int
	memberSuper           *nativeMemberClass
	memberEnclosingReadPC int
}
type nativeAnonymousFamily struct {
	forest   *nativeAnonymousForest
	owner    string
	children map[string]*nativeAnonymousClass
	failed   bool
	bridges  map[string]*nativeConstructorAccessBridge
}

// Anonymous ownership is an original attribute fact; binary spelling only
// checks that javac can regenerate the original ordinal after source layout.
func originalAnonymousOwner(obj *ClassObject) (string, string, bool) {
	if obj == nil {
		return "", "", false
	}
	self, selfRows, anonymous := obj.GetClassName(), 0, false
	for _, a := range obj.Attributes {
		if inner, ok := a.(*InnerClassesAttribute); ok && inner != nil {
			for _, row := range inner.Classes {
				if row == nil {
					return "", "", false
				}
				name, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
				if !known {
					return "", "", false
				}
				if name == self {
					selfRows++
					anonymous = row.InnerNameIndex == 0 && row.OuterClassInfoIndex == 0
				}
			}
		}
	}
	if selfRows != 1 || !anonymous {
		return "", "", false
	}
	owner, method, found := "", "", false
	for _, a := range obj.Attributes {
		if raw, ok := a.(*UnparsedAttribute); ok && raw != nil && raw.Name == "EnclosingMethod" {
			if found || raw.Length != 4 || len(raw.Info) != 4 {
				return "", "", false
			}
			found = true
			var known bool
			owner, known = sourceBridgeClassName(obj, binary.BigEndian.Uint16(raw.Info[:2]))
			if !known {
				return "", "", false
			}
			idx := int(binary.BigEndian.Uint16(raw.Info[2:]))
			if idx == 0 {
				continue
			}
			if idx <= 0 || idx > len(obj.ConstantPool) {
				return "", "", false
			}
			nt, ok := obj.ConstantPool[idx-1].(*ConstantNameAndTypeInfo)
			if !ok || nt == nil {
				return "", "", false
			}
			n, nok := sourceBridgeUTF8(obj, nt.NameIndex)
			d, dok := sourceBridgeUTF8(obj, nt.DescriptorIndex)
			if !nok || !dok {
				return "", "", false
			}
			method = n + d
		}
	}
	return owner, method, found && owner != ""
}

func nativeAnonymousConstructor(obj *ClassObject, owner string, method string, work *workbudget.Budget, access ...map[string]*nativeConstructorAccessBridge) *nativeAnonymousClass {
	return nativeAnonymousConstructorWithinMembers(obj, owner, method, work, nil, access...)
}

func nativeAnonymousConstructorWithinMembers(obj *ClassObject, owner string, method string, work *workbudget.Budget, members *nativeMemberFamily, access ...map[string]*nativeConstructorAccessBridge) *nativeAnonymousClass {
	return nativeAnonymousConstructorWithinForest(obj, owner, method, work, members, nil, access...)
}

func nativeAnonymousConstructorWithinForest(obj *ClassObject, owner string, method string, work *workbudget.Budget, members *nativeMemberFamily, forest *nativeAnonymousForest, access ...map[string]*nativeConstructorAccessBridge) *nativeAnonymousClass {
	if obj == nil || obj.AccessFlags&(0x0200|0x0400|0x4000) != 0 || len(obj.Interfaces) > 1 || len(obj.Interfaces) == 1 && obj.GetSupperClassName() != "java/lang/Object" {
		return nil
	}
	// These class-level declarations cannot be moved into an anonymous body.
	// Type-use annotations need a separate allocation/type-path reconstruction.
	for _, attribute := range obj.Attributes {
		switch a := attribute.(type) {
		case *RuntimeVisibleAnnotationsAttribute, *RuntimeVisibleTypeAnnotationsAttribute, *DeprecatedAttribute, *SyntheticAttribute:
			return nil
		case *UnparsedAttribute:
			if strings.Contains(a.Name, "Annotation") {
				return nil
			}
		}
	}
	if obj.AccessFlags != 0x0020 {
		return nil
	}
	c := &nativeAnonymousClass{object: obj, fields: map[string]int{}, capturePCs: map[string]int{}, method: method}
	prefix := owner + "$"
	suffix, ok := strings.CutPrefix(obj.GetClassName(), prefix)
	if !ok {
		return nil
	}
	n, e := strconv.Atoi(suffix)
	if e != nil || n <= 0 || strconv.Itoa(n) != suffix {
		return nil
	}
	c.ordinal = n
	var code *CodeAttribute
	matches := 0
	for _, m := range obj.Methods {
		name, _ := obj.getUtf8(m.NameIndex)
		if m.AccessFlags&0x0008 != 0 {
			return nil
		}
		if name == "<clinit>" {
			return nil
		}
		if name != "<init>" {
			continue
		}
		matches++
		c.descriptor, _ = obj.getUtf8(m.DescriptorIndex)
		for _, a := range m.Attributes {
			if ca, ok := a.(*CodeAttribute); ok {
				if code != nil {
					return nil
				}
				code = ca
			}
		}
	}
	if matches != 1 || code == nil || len(code.ExceptionTable) != 0 || !nativeProofWork(work, int64(len(code.Code))) {
		return nil
	}
	ps, ret, e := callbinding.Descriptor(c.descriptor)
	if e != nil || ret != "V" {
		return nil
	}
	slots := constructorParameterSlots(ps)
	d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	d.Work = work
	if d.ParseOpcode() != nil {
		return nil
	}
	ops := constructorMotionOps(d)
	if len(ops) > 512 {
		return nil
	}
	i := 0
	for i+2 < len(ops) {
		mem := constructorMotionMember(obj, ops[i+2], core.OP_PUTFIELD)
		if mem == nil {
			break
		}
		slot := core.GetRetrieveIdx(ops[i+1])
		param, known := slots[slot]
		if core.GetRetrieveIdx(ops[i]) != 0 || !constructorMotionLoad(ops[i], "Ljava/lang/Object;") || !known || !constructorMotionLoad(ops[i+1], ps[param]) || mem.Name != obj.GetClassName() || mem.Description != ps[param] || !constructorMotionField(obj, mem, true) {
			return nil
		}
		if _, dup := c.fields[mem.Member]; dup {
			return nil
		}
		c.fields[mem.Member] = param
		c.capturePCs[mem.Member] = int(ops[i+2].CurrentOffset)
		i += 3
	}
	if i >= len(ops) || core.GetRetrieveIdx(ops[i]) != 0 || !constructorMotionLoad(ops[i], "Ljava/lang/Object;") {
		return nil
	}
	i++
	outerField := "this$0"
	currentMember := (*nativeMemberClass)(nil)
	if members != nil {
		currentMember = members.children[owner]
		if currentMember != nil && !currentMember.static {
			suffix, known := strings.CutPrefix(currentMember.field, "this$")
			depth, err := strconv.Atoi(suffix)
			if !known || err != nil || depth < 0 || depth >= 64 {
				return nil
			}
			outerField = "this$" + strconv.Itoa(depth+1)
		}
	}
	if forest != nil && forest.units[owner] != nil {
		parent := forest.units[owner]
		depth := 0
		if parent.enclosingField != "" {
			var err error
			depth, err = strconv.Atoi(strings.TrimPrefix(parent.enclosingField, "this$"))
			if err != nil {
				return nil
			}
			depth++
		}
		outerField = "this$" + strconv.Itoa(depth)
		c.parentAnonymous = true
	}
	// javac's enclosing operand for an anonymous subclass of a sibling member
	// is this member's original capture. The same joint plan proves both
	// declarations and javac regenerates this exact read before super(...).
	if members != nil && currentMember != nil && !currentMember.static && i+1 < len(ops) {
		parent := members.children[obj.GetSupperClassName()]
		field := constructorMotionMember(obj, ops[i+1], core.OP_GETFIELD)
		outerIndex, outerCaptured := c.fields[outerField]
		if parent != nil && !parent.static && parent.owner == currentMember.owner && parent.owner == members.owner &&
			len(ps) > 0 && ps[0] == "L"+owner+";" && outerCaptured && outerIndex == 0 &&
			core.GetRetrieveIdx(ops[i]) == 1 && constructorMotionLoad(ops[i], ps[0]) &&
			field != nil && field.Name == owner && field.Member == currentMember.field && field.Description == "L"+parent.owner+";" {
			c.memberSuper = parent
			c.memberEnclosingReadPC = int(ops[i+1].CurrentOffset)
			i += 2
		}
	}
	nullTail := false
	for i < len(ops) {
		if ops[i].Instr.OpCode == core.OP_INVOKESPECIAL {
			break
		}
		if ops[i].Instr.OpCode == core.OP_ACONST_NULL && len(ops[i].Data) == 0 && i+1 < len(ops) && ops[i+1].Instr.OpCode == core.OP_INVOKESPECIAL {
			nullTail = true
			i++
			break
		}
		slot := core.GetRetrieveIdx(ops[i])
		param, known := slots[slot]
		if !known || !constructorMotionLoad(ops[i], ps[param]) {
			return nil
		}
		c.superParams = append(c.superParams, param)
		i++
	}
	if i+2 != len(ops) || ops[i+1].Instr.OpCode != core.OP_RETURN || len(ops[i+1].Data) != 0 {
		return nil
	}
	mem := constructorMotionMember(obj, ops[i], core.OP_INVOKESPECIAL)
	if mem == nil || mem.Name != obj.GetSupperClassName() || mem.Member != "<init>" {
		return nil
	}
	c.superDescriptor = mem.Description
	c.sourceSuperDescriptor = mem.Description
	if c.memberSuper != nil {
		ctor := c.memberSuper.constructors[mem.Description]
		if ctor == nil || c.memberSuper.object.GetClassName() != mem.Name || nullTail {
			return nil
		}
		c.sourceSuperDescriptor = ctor.sourceDescriptor
	}
	var bridge *nativeConstructorAccessBridge
	if nullTail {
		if mem.Name != owner || len(access) != 1 {
			return nil
		}
		bridge = access[0][mem.Description]
		if bridge == nil {
			return nil
		}
		c.sourceSuperDescriptor = bridge.target
	}
	c.superPC = int(ops[i].CurrentOffset)
	extra := 0
	if nullTail {
		extra = 1
	}
	if c.memberSuper != nil {
		extra++
	}
	ds, ret, e := callbinding.Descriptor(mem.Description)
	if e != nil || ret != "V" || len(ds) != len(c.superParams)+extra {
		return nil
	}
	for j, p := range c.superParams {
		index := j
		if c.memberSuper != nil {
			index++
		}
		if ps[p] != ds[index] {
			return nil
		}
	}
	if c.memberSuper != nil && ds[0] != "L"+c.memberSuper.owner+";" {
		return nil
	}
	// Nothing can silently disappear: every constructor argument is either a
	// real superclass argument or a compiler capture, and never both.
	used := map[int]bool{}
	for _, p := range c.fields {
		if used[p] {
			return nil
		}
		used[p] = true
	}
	for _, p := range c.superParams {
		if used[p] {
			return nil
		}
		used[p] = true
	}
	if len(used) != len(ps) {
		return nil
	}
	outer := -1
	captureOrder := []int{}
	for _, f := range obj.Fields {
		name, _ := obj.getUtf8(f.NameIndex)
		if index, captured := c.fields[name]; captured {
			if f.AccessFlags != 0x1010 {
				return nil
			}
			if name == outerField {
				if outer >= 0 || ps[index] != "L"+owner+";" {
					return nil
				}
				outer = index
			} else {
				local, known := strings.CutPrefix(name, "val$")
				if !known || local == "" || class_context.SafeIdentifier(local) != local {
					return nil
				}
				captureOrder = append(captureOrder, index)
			}
		}
		if f.AccessFlags&0x0008 != 0 && (f.AccessFlags&0x0010 == 0 || !fieldHasConstantValue(f)) {
			return nil
		}
		if f.AccessFlags&0x1000 != 0 && f.AccessFlags&0x0008 == 0 {
			n, _ := obj.getUtf8(f.NameIndex)
			if _, known := c.fields[n]; !known {
				return nil
			}
		}
	}
	if c.parentAnonymous && outer < 0 {
		return nil
	}
	if outer >= 0 {
		c.enclosingField = outerField
	}
	next := 0
	if outer >= 0 {
		if outer != next {
			return nil
		}
		next++
	}
	for _, index := range c.superParams {
		if index != next {
			return nil
		}
		next++
	}
	for _, index := range captureOrder {
		if index != next {
			return nil
		}
		next++
	}
	return c
}

func fieldHasConstantValue(field *MemberInfo) bool {
	for _, a := range field.Attributes {
		if _, ok := a.(*ConstantValueAttribute); ok {
			return true
		}
	}
	return false
}

func (c *ClassObjectDumper) planNativeAnonymousFamily() *nativeAnonymousFamily {
	if existing := c.planNativeAnonymousFamilyWithinMembers(nil); existing != nil {
		return existing
	}
	return c.planNativeAnonymousForest()
}

func (c *ClassObjectDumper) planNativeAnonymousFamilyWithinMembers(members *nativeMemberFamily) *nativeAnonymousFamily {
	p := c.planNativeAnonymousGroup(members, nil)
	if p == nil {
		return nil
	}
	return c.validateNativeAnonymousGroup(p, members, nil)
}
func (c *ClassObjectDumper) planNativeAnonymousGroup(members *nativeMemberFamily, forest *nativeAnonymousForest) *nativeAnonymousFamily {
	if c.foldSiblingResolver == nil || isGenuineEnum(c.obj) || c.getenv("JDEC_NATIVE_ANONYMOUS_OFF") != "" || !nativeSourceBinaryName(c.obj.GetClassName()) {
		return nil
	}
	if _, _, anon := originalAnonymousOwner(c.obj); anon && (forest == nil || forest.units[c.obj.GetClassName()] == nil) {
		return nil
	}
	p := &nativeAnonymousFamily{forest: forest, owner: c.obj.GetClassName(), children: map[string]*nativeAnonymousClass{}, bridges: map[string]*nativeConstructorAccessBridge{}}
	access := c.nativeConstructorAccessBridges()
	names := map[string]bool{}
	for _, a := range c.obj.Attributes {
		if inner, ok := a.(*InnerClassesAttribute); ok {
			for _, row := range inner.Classes {
				if row != nil && row.InnerNameIndex == 0 {
					n, known := sourceBridgeClassName(c.obj, row.InnerClassInfoIndex)
					if known {
						names[n] = true
					}
				}
			}
		}
	}
	if len(names) > 64 {
		return nil
	}
	for name := range names {
		raw, known := c.foldSiblingResolver(name)
		if !known {
			continue
		}
		obj, e := c.parseResolved(raw)
		if e != nil || obj.GetClassName() != name {
			return nil
		}
		owner, method, anon := originalAnonymousOwner(obj)
		if !anon || owner != p.owner {
			continue
		}
		if members != nil && members.emptyMarkers[name] != nil && nativeMemberEmptyAccessMarker(obj, members.owner, c.Work) {
			continue
		}
		if !nativeSourceBinaryName(obj.GetSupperClassName()) {
			return nil
		}
		for _, index := range obj.Interfaces {
			name, known := sourceBridgeClassName(obj, index)
			if !known || !nativeSourceBinaryName(name) {
				return nil
			}
		}
		child := nativeAnonymousConstructorWithinForest(obj, owner, method, c.Work, members, forest, access)
		if child == nil {
			return nil
		}
		p.children[name] = child
	}
	for _, child := range p.children {
		if child.sourceSuperDescriptor != child.superDescriptor && child.memberSuper == nil {
			bridge := access[child.superDescriptor]
			// javac8 regenerates access markers using its first owned anonymous type.
			marker := p.children[bridge.marker]
			if marker == nil || marker.ordinal != 1 {
				return nil
			}
			p.bridges[bridge.descriptor] = bridge
		}
	}
	if len(p.children) == 0 {
		return nil
	}
	for i := 1; i <= len(p.children); i++ {
		if p.children[p.owner+"$"+strconv.Itoa(i)] == nil {
			return nil
		}
	}
	return p
}
func (c *ClassObjectDumper) validateNativeAnonymousGroup(p *nativeAnonymousFamily, members *nativeMemberFamily, forest *nativeAnonymousForest) *nativeAnonymousFamily {
	allNames := map[string]bool{}
	for _, a := range c.obj.Attributes {
		if inner, ok := a.(*InnerClassesAttribute); ok && inner != nil {
			for _, row := range inner.Classes {
				if row == nil {
					return nil
				}
				name, known := sourceBridgeClassName(c.obj, row.InnerClassInfoIndex)
				if !known {
					return nil
				}
				allNames[name] = true
			}
		}
	}
	if len(allNames) > 256 {
		return nil
	}
	// InnerClasses lists direct references, not the complete lexical family.
	// A deeper named sibling may use this anonymous type as a javac-8 private
	// constructor access parameter. Traverse original owned nesting to closure
	// before suppressing any source unit; unresolved external types stay external.
	queue := make([]string, 0, len(allNames))
	for name := range allNames {
		queue = append(queue, name)
	}
	for cursor := 0; cursor < len(queue); cursor++ {
		name := queue[cursor]
		if name == p.owner || p.children[name] != nil {
			continue
		}
		raw, found := c.foldSiblingResolver(name)
		if !found {
			continue
		}
		sibling, err := c.parseResolved(raw)
		if err != nil || sibling.GetClassName() != name {
			return nil
		}
		for _, attribute := range sibling.Attributes {
			if inner, ok := attribute.(*InnerClassesAttribute); ok && inner != nil {
				for _, row := range inner.Classes {
					if row == nil {
						return nil
					}
					nested, known := sourceBridgeClassName(sibling, row.InnerClassInfoIndex)
					if !known {
						return nil
					}
					if !allNames[nested] {
						if len(allNames) >= 256 || !nativeProofWork(c.Work, 1) {
							return nil
						}
						allNames[nested] = true
						queue = append(queue, nested)
					}
				}
			}
		}
		for _, member := range append(append([]*MemberInfo{}, sibling.Fields...), sibling.Methods...) {
			descriptor, known := sourceBridgeUTF8(sibling, member.DescriptorIndex)
			if !known {
				return nil
			}
			for child := range p.children {
				if strings.Contains(descriptor, "L"+child+";") && !nativeAnonymousForestEnclosingDeclaration(sibling, member, forest, c.Work) && !nativeMemberJointBridgeDeclaration(members, sibling, member, child, c.Work) {
					return nil
				}
			}
		}
		jointBridgeTypes := nativeMemberJointBridgeNameTypes(members, sibling, c.Work)
		for constantIndex, constant := range sibling.ConstantPool {
			if nt, ok := constant.(*ConstantNameAndTypeInfo); ok {
				desc, known := sourceBridgeUTF8(sibling, nt.DescriptorIndex)
				if !known {
					return nil
				}
				for child := range p.children {
					if strings.Contains(desc, "L"+child+";") && !nativeAnonymousForestConstructorNameType(sibling, constantIndex+1, forest, c.Work) && !nativeAnonymousForestEnclosingNameType(sibling, constantIndex+1, forest, c.Work) && !nativeAnonymousForestCaptureNameType(forest, sibling, constantIndex+1, c.Work) && !jointBridgeTypes[constantIndex+1] {
						return nil
					}
				}
			}
			if member := nativeConstantMember(constant); member != nil {
				owner, known := sourceBridgeClassName(sibling, member.ClassIndex)
				if !known || p.children[owner] != nil && !nativeAnonymousForestCaptureReference(forest, sibling, constantIndex+1, c.Work) {
					return nil
				}
			}
		}
	}
	// A native anonymous class has no source-level binary type name. Families
	// referenced by declarations or by another nested owner require a joint
	// ownership plan; leaving those other units flat would lose their bindings.
	objects := []*ClassObject{c.obj}
	for _, child := range p.children {
		objects = append(objects, child.object)
	}
	for _, object := range objects {
		if !c.nativeAccessBridgeCalls(p, object) {
			return nil
		}
		for _, a := range object.Attributes {
			if inner, ok := a.(*InnerClassesAttribute); ok && inner != nil {
				for _, row := range inner.Classes {
					if row == nil {
						return nil
					}
					if row.OuterClassInfoIndex != 0 {
						outer, known := sourceBridgeClassName(object, row.OuterClassInfoIndex)
						if !known || p.children[outer] != nil {
							return nil
						}
					}
					name, known := sourceBridgeClassName(object, row.InnerClassInfoIndex)
					if !known {
						return nil
					}
					if row.InnerNameIndex == 0 && p.children[name] == nil {
						if raw, found := c.foldSiblingResolver(name); found {
							nested, err := c.parseResolved(raw)
							if err != nil {
								return nil
							}
							owner, _, anonymous := originalAnonymousOwner(nested)
							if anonymous && p.children[owner] != nil && !nativeAnonymousForestOwnChild(forest, owner, name) {
								return nil
							}
						}
					}
				}
			}
		}
		bridgeNameTypes := nativeMemberJointBridgeNameTypes(members, object, c.Work)
		if len(p.bridges) > 0 {
			for index, valid := range p.accessBridgeNameTypes(object, c.Work) {
				if valid {
					bridgeNameTypes[index] = true
				}
			}
		}
		for constantIndex, constant := range object.ConstantPool {
			if nt, ok := constant.(*ConstantNameAndTypeInfo); ok && nt != nil {
				descriptor, known := sourceBridgeUTF8(object, nt.DescriptorIndex)
				if !known {
					return nil
				}
				for child := range p.children {
					if strings.Contains(descriptor, "L"+child+";") && !nativeAnonymousForestConstructorNameType(object, constantIndex+1, forest, c.Work) && !nativeAnonymousForestEnclosingNameType(object, constantIndex+1, forest, c.Work) && !nativeAnonymousForestCaptureNameType(forest, object, constantIndex+1, c.Work) && !bridgeNameTypes[constantIndex+1] {
						return nil
					}
				}
			}
		}
		for _, member := range append(append([]*MemberInfo{}, object.Fields...), object.Methods...) {
			descriptor, known := sourceBridgeUTF8(object, member.DescriptorIndex)
			if !known {
				return nil
			}
			for child := range p.children {
				name, _ := object.getUtf8(member.NameIndex)
				if strings.Contains(descriptor, "L"+child+";") && !nativeAnonymousForestEnclosingDeclaration(object, member, forest, c.Work) && !p.accessBridgeDescriptor(object, name, descriptor) {
					return nil
				}
			}
		}
	}
	// Every class has exactly one witnessed allocation in its declared lexical
	// method. Repeated field initializers across constructors need a distinct plan.
	counts := map[string]int{}
	invokes := map[string]int{}
	for _, m := range c.obj.Methods {
		mn, _ := c.obj.getUtf8(m.NameIndex)
		md, _ := c.obj.getUtf8(m.DescriptorIndex)
		for _, a := range m.Attributes {
			code, ok := a.(*CodeAttribute)
			if !ok {
				continue
			}
			if !nativeProofWork(c.Work, int64(len(code.Code))) {
				return nil
			}
			d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.ConstantPool, i) })
			d.Work = c.Work
			if d.ParseOpcode() != nil {
				return nil
			}
			for _, op := range d.Opcodes() {
				if op == nil || op.Instr == nil {
					continue
				}
				if op.Instr.OpCode == core.OP_NEW {
					n, known := sourceBridgeClassName(c.obj, core.Convert2bytesToInt(op.Data))
					if child := p.children[n]; known && child != nil {
						counts[n]++
						child.newPC = int(op.CurrentOffset)
						if child.method != "" && child.method != mn+md {
							return nil
						}
					}
				}
				if member := constructorMotionMember(c.obj, op, core.OP_INVOKESPECIAL); member != nil && member.Member == "<init>" {
					if member.Name == p.owner && p.bridges[member.Description] != nil {
						return nil
					}
					if child := p.children[member.Name]; child != nil {
						if member.Description != child.descriptor || child.method != "" && child.method != mn+md {
							return nil
						}
						child.invokePC = int(op.CurrentOffset)
						invokes[member.Name]++
					}
				}
				for _, opcode := range []int{core.OP_GETFIELD, core.OP_PUTFIELD, core.OP_GETSTATIC, core.OP_PUTSTATIC} {
					if member := constructorMotionMember(c.obj, op, opcode); member != nil && p.children[member.Name] != nil {
						return nil
					}
				}
				if op.Instr.OpCode == core.OP_LDC || op.Instr.OpCode == core.OP_LDC_W {
					idx := uint16(0)
					if len(op.Data) == 1 {
						idx = uint16(op.Data[0])
					} else if len(op.Data) == 2 {
						idx = core.Convert2bytesToInt(op.Data)
					} else {
						return nil
					}
					if n, known := sourceBridgeClassName(c.obj, idx); known && p.children[n] != nil {
						return nil
					}
				}
				if op.Instr.OpCode == core.OP_CHECKCAST || op.Instr.OpCode == core.OP_INSTANCEOF || op.Instr.OpCode == core.OP_ANEWARRAY || op.Instr.OpCode == core.OP_MULTIANEWARRAY {
					n, known := sourceBridgeClassName(c.obj, core.Convert2bytesToInt(op.Data))
					if known && p.children[n] != nil {
						return nil
					}
				}
			}
		}
	}
	for n := range p.children {
		if counts[n] != 1 || invokes[n] != 1 {
			return nil
		}
	}
	return p
}

func (c *ClassObjectDumper) wireNativeAnonymousSource() {
	ctx := c.FuncCtx
	if c.nativeCaptureFields != nil {
		ctx.SourceCapturedFieldType = func(pc int, owner, name, descriptor string) any {
			if owner != c.obj.GetClassName() || c.nativeCapturedReads[ctx.FunctionName+ctx.CurrentMethodDesc][pc] != name {
				return nil
			}
			view := c.nativeCaptureTypes[name]
			if c.nativeMemberCurrent != nil && view != nil {
				// An enclosing formal has a different declaration identity from a
				// same-spelled current class/method formal. It cannot be named as
				// that shadowing variable in a materialized outer qualifier.
				if !nativeMemberEnclosingTypeDenotable(c, view) {
					return nil
				}
			}
			return view
		}
		ctx.SourceCapturedField = func(pc int, name string, receiver bool) (string, bool) {
			if _, captured := c.nativeCaptureFields[name]; !captured {
				return "", false
			}
			key := ctx.FunctionName + ctx.CurrentMethodDesc
			field := c.nativeCapturedReads[key][pc]
			text, known := c.nativeCaptureFields[field]
			if !known || field != name || !receiver {
				c.nativeCaptureFailed = true
				return "", false
			}
			return text, true
		}
	}
	c.wireNativeAnonymousForestCaptures(ctx)
	p := c.nativeAnonymousRoot
	if p == nil {
		return
	}
	prior := ctx.DeclarationSourceName
	ctx.DeclarationSourceName = func(name string) (string, bool) {
		if child := p.children[strings.ReplaceAll(name, ".", "/")]; child != nil {
			return nativeAnonymousParentType(child.object, ctx), true
		}
		if prior != nil {
			return prior(name)
		}
		return "", false
	}
	ctx.SourceAnonymousCandidate = func(owner string) bool { return p.children[strings.ReplaceAll(owner, ".", "/")] != nil }
	ctx.SourceAnonymousAllocation = func(owner, descriptor string, newPC, pc int, args []class_context.SourceCaptureOperand) (string, bool) {
		child := p.children[strings.ReplaceAll(owner, ".", "/")]
		if child == nil {
			return "", false
		}
		if c.Work != nil {
			if c.Work.Enter(workbudget.CounterASTDepth) != nil {
				p.failed = true
				return "", false
			}
			defer c.Work.Leave(workbudget.CounterASTDepth)
		}
		fail := func() (string, bool) { p.failed = true; return "", false }
		if descriptor != child.descriptor || newPC != child.newPC || pc != child.invokePC {
			return fail()
		}
		ps, _, e := callbinding.Descriptor(descriptor)
		if e != nil || len(ps) != len(args) {
			return fail()
		}
		bindings := map[string]string{}
		captureTypes := map[string]types.JavaType{}
		for field, index := range child.fields {
			if index < 0 || index >= len(args) {
				return fail()
			}
			arg := args[index]
			if !arg.Local && !arg.Receiver {
				return fail()
			}
			if (field == child.enclosingField) && !arg.Receiver {
				return fail()
			}
			if arg.Receiver {
				arg.Text = ctx.ShortTypeName(ctx.ClassName) + ".this"
			}
			bindings[field] = arg.Text
			if raw, ok := arg.Value.(values.JavaValue); ok && raw.Type() != nil {
				if primitive, ok := raw.Type().RawType().(*types.JavaPrimer); ok && primitive.Name != descriptorPrimitiveName(ps[index]) {
					return fail()
				}
				if parameterized, ok := types.AsParameterizedType(raw.Type()); ok && ps[index] == "L"+strings.ReplaceAll(parameterized.RawClassName, ".", "/")+";" {
					captureTypes[field] = raw.Type().Copy()
				}
			}
		}
		sub := NewClassObjectDumper(child.object)
		sub.options = c.options
		sub.Work = c.Work
		sub.foldSiblingResolver = c.foldSiblingResolver
		sub.declarationResolver = c.declarationResolver
		sub.nativeMemberLookup = c.nativeMemberLookup
		if c.obj.GetClassName() == p.owner && nativeMemberJointAnonymousAccess(c.nativeMemberRoot, child.object.GetClassName(), c.Work) {
			// This body is being emitted inside the same joint lexical commit.
			// Reuse its proved member names; looking up the owner's incomplete
			// cache entry would recurse, and a flat binary name loses private scope.
			sub.nativeMemberRoot = c.nativeMemberRoot
		}
		if p.forest != nil {
			sub.nativeAnonymousRoot = p.forest.groups[child.object.GetClassName()]
			sub.nativeAnonymousForest = p.forest
			sub.nativeAnonymousBindings = map[string]string{}
			for key, text := range c.nativeAnonymousBindings {
				sub.nativeAnonymousBindings[key] = text
			}
			for field, text := range bindings {
				sub.nativeAnonymousBindings[nativeMemberCaptureIndexKey(child.object.GetClassName(), field)] = text
			}
		}
		sub.nativeCaptureFields = bindings
		sub.nativeCaptureTypes = captureTypes
		sub.nativeOuterContext = ctx
		sub.nativeTypeParams = append([]string(nil), ctx.TypeParams...)
		sub.nativeCapturedReads = map[string]map[int]string{}
		for _, m := range child.object.Methods {
			for _, attribute := range m.Attributes {
				if signature, ok := attribute.(*SignatureAttribute); ok {
					text, _ := child.object.getUtf8(signature.SignatureIndex)
					for _, formal := range types.MethodFormalTypeParamNames(text) {
						if ctx.IsTypeParam(formal) {
							return fail()
						}
					}
				}
			}
			n, _ := child.object.getUtf8(m.NameIndex)
			desc, _ := child.object.getUtf8(m.DescriptorIndex)
			key := n + desc
			sub.nativeCapturedReads[key] = map[int]string{}
			for _, a := range m.Attributes {
				code, ok := a.(*CodeAttribute)
				if !ok {
					continue
				}
				if !nativeProofWork(c.Work, int64(len(code.Code))) {
					return fail()
				}
				d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(child.object.ConstantPool, i) })
				d.Work = c.Work
				if d.ParseOpcode() != nil {
					return fail()
				}
				for _, op := range d.Opcodes() {
					mem := constructorMotionMember(child.object, op, core.OP_GETFIELD)
					if mem != nil && mem.Name == child.object.GetClassName() {
						if _, captured := bindings[mem.Member]; captured {
							sub.nativeCapturedReads[key][int(op.CurrentOffset)] = mem.Member
						}
					}
				}
			}
		}
		src, e := sub.DumpClass()
		if e != nil || sub.nativeCaptureFailed || len(sub.constructorBoundaryHelpers) != 0 ||
			sub.privateNestOwnPlan != nil && len(sub.privateNestOwnPlan.bridges) != 0 {
			return fail()
		}
		// Java 8 anonymous bodies cannot host the static escape helper. A
		// checked escape needs an enclosing helper plan, not an illegal method.
		for _, method := range sub.dumpedMethodsSet {
			if method != nil && method.checkedEscape {
				return fail()
			}
		}
		if group := sub.nativeAnonymousRoot; group != nil && !group.completeOwnSource(src) {
			return fail()
		}
		body := javaClassBodyContent(src)
		if body == "" {
			return fail()
		}
		for _, imp := range javaExtractImports(src) {
			ctx.Import(imp)
		}
		parent := nativeAnonymousParentType(child.object, ctx)
		tuple := []string{}
		originalTypes, e := types.ParseMethodDescriptor(child.descriptor)
		if e != nil {
			return fail()
		}
		invoke := &values.FunctionCallExpression{ClassName: strings.ReplaceAll(child.object.GetSupperClassName(), "/", "."), FunctionName: "<init>", Descriptor: child.sourceSuperDescriptor, Kind: values.InvokeSpecial, IsSpecialInvoke: true, Object: &values.JavaRef{IsThis: true}, OriginPC: child.superPC, HasOriginPC: true}
		for _, index := range child.superParams {
			text, typ := args[index].Text, originalTypes.FunctionType().ParamTypes[index]
			if operand, ok := args[index].Value.(values.JavaValue); ok && operand.Type() != nil {
				typ = operand.Type().Copy()
				if literal, ok := values.UnpackSoltValue(operand).(*values.JavaLiteral); ok {
					copy := *literal
					copy.JavaType = typ
					copy.Units = append([]uint16(nil), literal.Units...)
					invoke.Arguments = append(invoke.Arguments, &copy)
					continue
				}
			}
			invoke.Arguments = append(invoke.Arguments, values.NewCustomValue(func(*class_context.ClassContext) string { return text }, func() types.JavaType { return typ }))
		}
		delegateType, e := types.ParseMethodDescriptor(child.sourceSuperDescriptor)
		if e != nil {
			return fail()
		}
		invoke.FuncType = delegateType.FunctionType()
		binding := *sub.FuncCtx
		if child.sourceSuperDescriptor != child.superDescriptor {
			binding = *ctx // The source allocation has the original enclosing private access.
		}
		if child.memberSuper != nil {
			if c.nativeMemberRoot == nil || c.nativeMemberRoot.children[child.memberSuper.object.GetClassName()] != child.memberSuper || !nativeMemberJointAnonymousAccess(c.nativeMemberRoot, child.object.GetClassName(), c.Work) {
				return fail()
			}
			binding = *nativeMemberBinding(ctx, c.nativeMemberRoot, c.Work)
		}
		binding.FunctionName = "<init>"
		binding.CurrentMethodDesc = child.descriptor
		tuple = invoke.ArgumentStrings(&binding)
		size := int64(len(body)) + int64(len(parent)) + int64(len(p.owner)) + 80
		for _, argument := range tuple {
			size += int64(len(argument)) + 1
		}
		if ctx.ChargeOutput(size) != nil {
			return fail()
		}
		return fmt.Sprintf("/*jdec-owned-anonymous-ordinal:%d:%s*/new %s(%s) {%s}", child.ordinal, p.owner, parent, strings.Join(tuple, ","), body), true
	}
}

// Bind a caller identity before parameter declarations and the body render.
// This is a lexical source name, not a replacement value or a heap alias.

// Layout is checked on actual emitted comments, skipping literals and other
// comments. Anonymous numbering is a source-layout property, not just metadata.
func nativeAnonymousOrdinals(source string) ([]int, bool) {
	return nativeAnonymousOrdinalsWithinOwner(source, "")
}

func nativeAnonymousOrdinalsWithinOwner(source, owner string, scopes ...map[string]bool) ([]int, bool) {
	result := []int{}
	prefix := "jdec-owned-anonymous-ordinal:"
	for i := 0; i < len(source); {
		ch := source[i]
		if ch == '\'' || ch == '"' {
			quote := ch
			i++
			closed := false
			for i < len(source) {
				if source[i] == '\\' {
					i += 2
					continue
				}
				if source[i] == quote {
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, false
			}
			continue
		}
		if i+1 < len(source) && source[i:i+2] == "//" {
			end := strings.IndexByte(source[i:], '\n')
			if end < 0 {
				break
			}
			i += end + 1
			continue
		}
		if i+1 < len(source) && source[i:i+2] == "/*" {
			end := strings.Index(source[i+2:], "*/")
			if end < 0 {
				return nil, false
			}
			comment := source[i+2 : i+2+end]
			if strings.HasPrefix(comment, prefix) {
				ordinal, scope, scoped := strings.Cut(strings.TrimPrefix(comment, prefix), ":")
				n, e := strconv.Atoi(ordinal)
				if e != nil {
					return nil, false
				}
				if scoped && !nativeSourceBinaryName(scope) || len(scopes) > 0 && (!scoped || !scopes[0][scope]) {
					return nil, false
				}
				if owner == "" || scoped && owner == scope {
					result = append(result, n)
				}
			}
			i += end + 4
			continue
		}
		i++
	}
	return result, true
}
func (p *nativeAnonymousFamily) completeSource(source string) bool {
	if p == nil {
		return false
	}
	if p.forest != nil {
		return p.forest.scopeSourceComplete(source)
	}
	return p.completeOwnSource(source)
}

func (p *nativeAnonymousFamily) completeOwnSource(source string) bool {
	if p == nil || p.failed {
		return false
	}
	ordinals, known := nativeAnonymousOrdinalsWithinOwner(source, p.owner)
	if !known || len(ordinals) != len(p.children) {
		return false
	}
	for i, n := range ordinals {
		if n != i+1 {
			return false
		}
	}
	return true
}
func (c *ClassObjectDumper) nativeLexicalCaptures() map[string]bool {
	result := map[string]bool{}
	for _, name := range c.nativeAnonymousBindings {
		if name != "" && class_context.SafeIdentifier(name) == name {
			result[name] = true
		}
	}
	for _, name := range c.nativeCaptureFields {
		if name != "" && class_context.SafeIdentifier(name) == name {
			result[name] = true
		}
	}
	return result
}

// Allocate lexical names over identities, reserving captures before renaming
// other locals. Both caller parameters and child method parameters may collide
// with an original val$ name; spelling alone must never merge those identities.
func (c *ClassObjectDumper) prepareNativeLocalShadowing(body []statements.Statement, params []values.JavaValue) {
	if c.nativeCaptureFields == nil {
		return
	}
	var protected map[*coreutils.VariableId]bool
	if child := c.nativeMemberCurrent; child != nil && c.FuncCtx.FunctionName == "<init>" && len(params) > 0 {
		if outer, ok := params[0].(*values.JavaRef); ok && outer.Id != nil && c.FuncCtx.LocalNames[outer.Id] == c.FuncCtx.ShortTypeName(strings.ReplaceAll(child.owner, "/", "."))+".this" {
			protected = map[*coreutils.VariableId]bool{outer.Id: true}
		}
	}
	c.prepareNativeSourceNames(body, params, c.nativeLexicalCaptures(), protected)
}
func (c *ClassObjectDumper) prepareNativeSourceNames(body []statements.Statement, params []values.JavaValue, reserved map[string]bool, protected map[*coreutils.VariableId]bool) {
	ctx := c.FuncCtx
	if ctx.LocalNames == nil {
		ctx.LocalNames = map[*coreutils.VariableId]string{}
	}
	refs := []*values.JavaRef{}
	seen := map[*coreutils.VariableId]bool{}
	occupied := map[string]bool{}
	for name := range reserved {
		occupied[name] = true
	}
	remaining := 16384
	valid := true
	activeValue := map[values.JavaValue]bool{}
	activeStatement := map[statements.Statement]bool{}
	var value func(values.JavaValue)
	value = func(v values.JavaValue) {
		if c.Work != nil {
			if c.Work.Enter(workbudget.CounterASTDepth) != nil {
				valid = false
				return
			}
			defer c.Work.Leave(workbudget.CounterASTDepth)
		}
		remaining--
		if remaining < 0 || !nativeProofWork(c.Work, 1) || activeValue[v] {
			valid = false
			return
		}
		if sourceProofNil(v) {
			return
		}
		activeValue[v] = true
		defer delete(activeValue, v)
		if ref, ok := v.(*values.JavaRef); ok && ref.Id != nil && !ref.IsThis && ref.CustomValue == nil && ref.StackVar == nil {
			if !seen[ref.Id] {
				seen[ref.Id] = true
				refs = append(refs, ref)
				occupied[ref.String(ctx)] = true
			}
			return
		}
		if call, ok := v.(*values.FunctionCallExpression); ok {
			if !call.IsStatic {
				value(call.Object)
			}
			for _, arg := range call.Arguments {
				value(arg)
			}
			return
		}
		children, known := values.Children(v)
		if !known {
			valid = false
			return
		}
		for _, v := range children {
			value(v)
		}
	}
	for _, v := range params {
		value(v)
	}
	var walk func([]statements.Statement)
	walk = func(ss []statements.Statement) {
		if c.Work != nil {
			if c.Work.Enter(workbudget.CounterASTDepth) != nil {
				valid = false
				return
			}
			defer c.Work.Leave(workbudget.CounterASTDepth)
		}
		for _, st := range ss {
			remaining--
			if remaining < 0 || !nativeProofWork(c.Work, 1) || sourceProofNil(st) || activeStatement[st] {
				valid = false
				return
			}
			activeStatement[st] = true
			roots, children, known := catchSourceChildren(st)
			if !known {
				valid = false
				return
			}
			for _, v := range roots {
				value(v)
			}
			for _, ss := range children {
				walk(ss)
			}
			delete(activeStatement, st)
		}
	}
	walk(body)
	if !valid {
		c.nativeCaptureFailed = true
		if c.nativeAnonymousRoot != nil {
			c.nativeAnonymousRoot.failed = true
		}
		return
	}
	for _, ref := range refs {
		current := ref.String(ctx)
		if reserved[current] && !protected[ref.Id] {
			candidate := "jdec$local$" + current
			for occupied[candidate] {
				candidate += "$"
			}
			ctx.LocalNames[ref.Id] = candidate
			occupied[candidate] = true
		}
	}
}
func nativeAnonymousParentType(object *ClassObject, ctx *class_context.ClassContext) string {
	raw := object.GetSupperClassName()
	iface := len(object.Interfaces) == 1
	if iface {
		raw, _ = sourceBridgeClassName(object, object.Interfaces[0])
	}
	for _, a := range object.Attributes {
		if sig, ok := a.(*SignatureAttribute); ok {
			signature, known := sourceBridgeUTF8(object, sig.SignatureIndex)
			if !known {
				break
			}
			parent, interfaces := types.ParseClassSignatureSupers(signature)
			if iface && len(interfaces) == 1 {
				return interfaces[0].String(ctx)
			}
			if !iface && parent != nil {
				return parent.String(ctx)
			}
		}
	}
	return ctx.ShortTypeName(strings.ReplaceAll(raw, "/", "."))
}
func nativeCaptureDeclaration(body []statements.Statement, ref *values.JavaRef, parameter bool, work *workbudget.Budget) (*statements.AssignStatement, bool) {
	if ref == nil || ref.Id == nil {
		return nil, false
	}
	var declaration *statements.AssignStatement
	valid := true
	remaining := 4096
	active := map[statements.Statement]bool{}
	var walk func([]statements.Statement)
	walk = func(ss []statements.Statement) {
		if work != nil {
			if work.Enter(workbudget.CounterASTDepth) != nil {
				valid = false
				return
			}
			defer work.Leave(workbudget.CounterASTDepth)
		}
		for _, st := range ss {
			remaining--
			if remaining < 0 || !nativeProofWork(work, 1) || sourceProofNil(st) || active[st] {
				valid = false
				return
			}
			active[st] = true
			if assign, ok := st.(*statements.AssignStatement); ok && assign.ArrayMember == nil {
				if target, ok := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef); ok && target != nil && target.Id == ref.Id {
					if parameter || !(assign.IsDeclare || assign.IsFirst) || declaration != nil {
						valid = false
						return
					}
					declaration = assign
				}
			}
			_, children, known := catchSourceChildren(st)
			if !known {
				valid = false
				return
			}
			for _, ss := range children {
				walk(ss)
			}
			delete(active, st)
		}
	}
	walk(body)
	if !valid || !parameter && declaration == nil {
		return nil, false
	}
	remaining = 8192
	if parameter {
		return nil, catchParameterUnwritten(body, ref, &remaining)
	}
	return declaration, catchParameterUnwritten(body, ref, &remaining, declaration)
}
func (c *ClassObjectDumper) prepareNativeCaptureBindings(body []statements.Statement, params []values.JavaValue) {
	p := c.nativeAnonymousRoot
	if p == nil {
		return
	}
	ctx := c.FuncCtx
	relevant := false
	for _, child := range p.children {
		if child.method == ctx.FunctionName+ctx.CurrentMethodDesc || child.method == "" && (ctx.FunctionName == "<init>" || ctx.FunctionName == "<clinit>") {
			relevant = true
		}
	}
	if !relevant {
		return
	}
	if ctx.LocalNames == nil {
		ctx.LocalNames = map[*coreutils.VariableId]string{}
	}
	parameterIDs := map[*coreutils.VariableId]bool{}
	for _, v := range params {
		if ref, ok := v.(*values.JavaRef); ok && ref.Id != nil {
			parameterIDs[ref.Id] = true
		}
	}
	allowed := map[int]map[*coreutils.VariableId]bool{}
	assigned := map[string]*coreutils.VariableId{}
	remaining := 16384
	activeValue := map[values.JavaValue]bool{}
	activeStatement := map[statements.Statement]bool{}
	var value func(values.JavaValue, map[*coreutils.VariableId]bool)
	value = func(v values.JavaValue, visible map[*coreutils.VariableId]bool) {
		if c.Work != nil {
			if c.Work.Enter(workbudget.CounterASTDepth) != nil {
				p.failed = true
				return
			}
			defer c.Work.Leave(workbudget.CounterASTDepth)
		}
		remaining--
		if remaining < 0 || !nativeProofWork(c.Work, 1) || sourceProofNil(v) || activeValue[v] {
			p.failed = true
			return
		}
		activeValue[v] = true
		defer delete(activeValue, v)
		if alloc, ok := v.(*values.NewExpression); ok && alloc.ConstructorCall != nil {
			call := alloc.ConstructorCall
			child := p.children[strings.ReplaceAll(call.ClassName, ".", "/")]
			if child != nil {
				site := map[*coreutils.VariableId]bool{}
				allowed[call.OriginPC] = site
				for field, index := range child.fields {
					if index >= len(call.Arguments) {
						p.failed = true
						continue
					}
					ref, ok := values.UnpackSoltValue(call.Arguments[index]).(*values.JavaRef)
					if !ok || ref == nil || ref.Id == nil {
						p.failed = true
						continue
					}
					if ref.IsThis {
						continue
					}
					declaration, stable := nativeCaptureDeclaration(body, ref, parameterIDs[ref.Id], c.Work)
					captureParams, _, err := callbinding.Descriptor(child.descriptor)
					if err != nil || index >= len(captureParams) {
						p.failed = true
						continue
					}
					declaredType := ref.Type()
					if declaration != nil && declaration.JavaValue != nil {
						declaredType = declaration.JavaValue.Type()
						if ref.WebDeclType != nil {
							declaredType = ref.WebDeclType
						}
					}
					erasure, known := values.SourceTypeErasure(declaredType, ctx)
					if !known || erasure != captureParams[index] {
						p.failed = true
						continue
					}
					if !visible[ref.Id] || !stable {
						p.failed = true
						continue
					}
					name, ok := strings.CutPrefix(field, "val$")
					if !ok || name == "" || class_context.SafeIdentifier(name) != name {
						p.failed = true
						continue
					}
					if old, known := ctx.LocalNames[ref.Id]; known && old != name {
						p.failed = true
						continue
					}
					if old := assigned[name]; old != nil && old != ref.Id {
						p.failed = true
						continue
					}
					ctx.LocalNames[ref.Id] = name
					assigned[name] = ref.Id
					site[ref.Id] = true
				}
			}
		}
		// Static invocations have no receiver node. Treat only their real operands
		// as dependencies, rather than interpreting the absent receiver as opaque IR.
		if call, ok := v.(*values.FunctionCallExpression); ok {
			if !call.IsStatic {
				value(call.Object, visible)
			}
			for _, arg := range call.Arguments {
				value(arg, visible)
			}
			return
		}
		children, known := values.Children(v)
		if !known {
			p.failed = true
			return
		}
		for _, v := range children {
			value(v, visible)
		}
	}
	var walk func([]statements.Statement, map[*coreutils.VariableId]bool)
	walk = func(ss []statements.Statement, visible map[*coreutils.VariableId]bool) {
		if c.Work != nil {
			if c.Work.Enter(workbudget.CounterASTDepth) != nil {
				p.failed = true
				return
			}
			defer c.Work.Leave(workbudget.CounterASTDepth)
		}
		for _, st := range ss {
			remaining--
			if remaining < 0 || !nativeProofWork(c.Work, 1) || sourceProofNil(st) || activeStatement[st] {
				p.failed = true
				return
			}
			activeStatement[st] = true
			roots, children, known := catchSourceChildren(st)
			if !known {
				p.failed = true
				delete(activeStatement, st)
				continue
			}
			for _, v := range roots {
				value(v, visible)
			}
			if assign, ok := st.(*statements.AssignStatement); ok && (assign.IsDeclare || assign.IsFirst) && assign.ArrayMember == nil {
				if ref, ok := values.UnpackSoltValue(assign.LeftValue).(*values.JavaRef); ok && ref != nil && ref.Id != nil {
					visible[ref.Id] = true
				}
			}
			for _, ss := range children {
				copy := map[*coreutils.VariableId]bool{}
				for id, v := range visible {
					copy[id] = v
				}
				walk(ss, copy)
			}
			delete(activeStatement, st)
		}
	}
	visible := map[*coreutils.VariableId]bool{}
	for _, v := range params {
		if ref, ok := v.(*values.JavaRef); ok && ref.Id != nil {
			visible[ref.Id] = true
		}
	}
	walk(body, visible)
	reserved := map[string]bool{}
	protected := map[*coreutils.VariableId]bool{}
	for name, id := range assigned {
		reserved[name] = true
		protected[id] = true
	}
	c.prepareNativeSourceNames(body, params, reserved, protected)
	ctx.SourceCaptureStable = func(pc int, id *coreutils.VariableId) bool { return allowed[pc][id] }
}

func descriptorPrimitiveName(descriptor string) string {
	p, err := types.ParseDescriptor(descriptor)
	if err != nil {
		return ""
	}
	if primitive, ok := p.RawType().(*types.JavaPrimer); ok {
		return primitive.Name
	}
	return ""
}

func nativeConstantMember(constant ConstantInfo) *ConstantMemberrefInfo {
	switch x := constant.(type) {
	case *ConstantFieldrefInfo:
		if x != nil {
			return &x.ConstantMemberrefInfo
		}
	case *ConstantMethodrefInfo:
		if x != nil {
			return &x.ConstantMemberrefInfo
		}
	case *ConstantInterfaceMethodrefInfo:
		if x != nil {
			return &x.ConstantMemberrefInfo
		}
	}
	return nil
}

func nativeProofWork(work *workbudget.Budget, count int64) bool {
	return work == nil || work.Charge(workbudget.CounterGraphScans, count) == nil
}

func nativeSourceBinaryName(name string) bool {
	if name == "" {
		return false
	}
	for _, component := range strings.Split(name, "/") {
		if component == "" || class_context.SafeIdentifier(component) != component {
			return false
		}
	}
	return true
}
