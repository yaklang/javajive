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
	// This view carries only original lambda implementation evidence. It
	// supplies no named membership, constructor or foreign private scope.
	lambdaImplementation  *nativeMemberClass
	assertions            *nativeMemberAssertion
	initializers          []nativeAnonymousInitializer
	expressionInitializer *nativeAnonymousExpressionInitializer
	enclosingField        string
	unusedEnclosing       bool
	parentAnonymous       bool
	capturePCs            map[string]int
	object                *ClassObject
	descriptor            string
	method                string
	ordinal               int
	fields                map[string]int
	superParams           []int
	sharedParameterRoles  bool
	superDescriptor       string
	sourceSuperDescriptor string
	superPC               int
	newPC                 int
	invokePC              int
	memberSuper           *nativeMemberClass
	memberEnclosingReadPC int
	memberEnclosingPath   *nativeMemberLexicalRead
}
type nativeAnonymousFamily struct {
	independentRoot *nativeAnonymousIndependentRoot
	forest          *nativeAnonymousForest
	owner           string
	children        map[string]*nativeAnonymousClass
	failed          bool
	bridges         map[string]*nativeConstructorAccessBridge
	standalone      map[string]*ClassObject
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
	return nativeAnonymousConstructorWithinSourceRoot(obj, owner, method, "", work, members, forest, access...)
}
func nativeAnonymousConstructorWithinSourceRoot(obj *ClassObject, owner, method, assertionRoot string, work *workbudget.Budget, members *nativeMemberFamily, forest *nativeAnonymousForest, access ...map[string]*nativeConstructorAccessBridge) *nativeAnonymousClass {
	return nativeAnonymousConstructorWithDeclarations(obj, owner, method, assertionRoot, work, members, forest, nil, access...)
}

func nativeAnonymousConstructorWithDeclarations(obj *ClassObject, owner, method, assertionRoot string, work *workbudget.Budget, members *nativeMemberFamily, forest *nativeAnonymousForest, metadata callbinding.Provider, access ...map[string]*nativeConstructorAccessBridge) *nativeAnonymousClass {
	return nativeAnonymousConstructorRepresentationProof(obj, owner, method, assertionRoot, work, members, forest, metadata, false, access...)
}

func nativeAnonymousConstructorRepresentationProof(obj *ClassObject, owner, method, assertionRoot string, work *workbudget.Budget, members *nativeMemberFamily, forest *nativeAnonymousForest, metadata callbinding.Provider, standalone bool, access ...map[string]*nativeConstructorAccessBridge) *nativeAnonymousClass {
	return nativeAnonymousConstructorRepresentationWithLambdas(obj, owner, method, assertionRoot, work, members, forest, metadata, standalone, nil, access...)
}

func nativeAnonymousConstructorRepresentationWithLambdas(obj *ClassObject, owner, method, assertionRoot string, work *workbudget.Budget, members *nativeMemberFamily, forest *nativeAnonymousForest, metadata callbinding.Provider, standalone bool, lambda *nativeMemberClass, access ...map[string]*nativeConstructorAccessBridge) *nativeAnonymousClass {
	flags := uint16(0x0020)
	if !standalone && !nativeAnonymousSelfAccessFlags(obj, 0, work) {
		return nil
	}
	if standalone {
		flags = 0x0030
	}
	return nativeAnonymousConstructorRolesWithLambdas(obj, owner, method, assertionRoot, work, members, forest, metadata, flags, false, lambda, access...)
}

func nativeAnonymousConstructorWithFlags(obj *ClassObject, owner, method, assertionRoot string, work *workbudget.Budget, members *nativeMemberFamily, forest *nativeAnonymousForest, metadata callbinding.Provider, flags uint16, access ...map[string]*nativeConstructorAccessBridge) *nativeAnonymousClass {
	return nativeAnonymousConstructorWithRoles(obj, owner, method, assertionRoot, work, members, forest, metadata, flags, false, access...)
}

func nativeAnonymousConstructorWithRoles(obj *ClassObject, owner, method, assertionRoot string, work *workbudget.Budget, members *nativeMemberFamily, forest *nativeAnonymousForest, metadata callbinding.Provider, flags uint16, omitUnusedEnclosing bool, access ...map[string]*nativeConstructorAccessBridge) *nativeAnonymousClass {
	return nativeAnonymousConstructorRolesWithLambdas(obj, owner, method, assertionRoot, work, members, forest, metadata, flags, omitUnusedEnclosing, nil, access...)
}

func nativeAnonymousConstructorRolesWithLambdas(obj *ClassObject, owner, method, assertionRoot string, work *workbudget.Budget, members *nativeMemberFamily, forest *nativeAnonymousForest, metadata callbinding.Provider, flags uint16, omitUnusedEnclosing bool, lambda *nativeMemberClass, access ...map[string]*nativeConstructorAccessBridge) *nativeAnonymousClass {
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
	if obj.AccessFlags != flags {
		return nil
	}
	c := &nativeAnonymousClass{object: obj, fields: map[string]int{}, capturePCs: map[string]int{}, method: method}
	if assertionRoot != "" {
		var known bool
		c.assertions, known = nativeMemberAssertionProof(obj, assertionRoot, work)
		if !known {
			return nil
		}
	}
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
	var lambdaMethods []*MemberInfo
	matches := 0
	for _, m := range obj.Methods {
		name, _ := obj.getUtf8(m.NameIndex)
		if c.assertions != nil && c.assertions.initializer == m {
			continue
		}
		// An accessor is a separate original packet role; it never receives
		// metafactory permission. Complete forest/source closure is still required.
		if members != nil && lambda != nil && nativeMemberPrivateAccessProofWithDeclarations(obj, m, lambda.lambdaContext.resolve, work, members.lexicalObjects) != nil {
			continue
		}
		if m.AccessFlags&0x0008 != 0 || m.AccessFlags&0x1002 == 0x1002 {
			// Anonymous source cannot declare an ordinary static method in
			// Java 8. Both static and bound private synthetic targets are
			// instead regenerated by their
			// exclusively witnessed original metafactory sites. Archive and
			// actual source consumption remain independent publication gates.
			if lambda == nil || lambda.object != obj || len(lambdaMethods) >= 64 {
				return nil
			}
			lambdaMethods = append(lambdaMethods, m)
			continue
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
		if core.GetRetrieveIdx(ops[i]) != 0 || !constructorMotionLoad(ops[i], "Ljava/lang/Object;") || !known || !constructorMotionLoad(ops[i+1], ps[param]) || mem.Name != obj.GetClassName() || mem.Description != ps[param] || !constructorMotionField(obj, mem, true, work) {
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
	// The anonymous constructor passes its unchanged enclosing word (slot 1)
	// through the original lexical capture graph to the named member SUPER.
	// Source nesting, class inheritance and ordinary captured values are
	// different relations; only the first relation can omit this operand.
	if members != nil && i < len(ops) {
		parent := members.anonymousSuperClass(obj.GetSupperClassName())
		capture, captured := c.fields[outerField]
		if parent != nil && !parent.static && captured && capture == 0 &&
			len(ps) > 0 && ps[0] == "L"+owner+";" {
			path, next, known := nativeAnonymousMemberSuperEnclosingPath(obj, owner, parent, members, forest, ops, i, work)
			if known {
				c.memberSuper, c.memberEnclosingPath = parent, path
				c.memberEnclosingReadPC = -1
				if path != nil {
					c.memberEnclosingReadPC = path.pc
				}
				i = next
			}
		}
	}
	// A captured word passed to SUPER is not by itself an enclosing-instance
	// certificate. Original InnerClasses metadata distinguishes a nonstatic
	// member superclass from an ordinary constructor argument. Without the
	// complete named-parent proof, never fall back to the ordinary SUPER path.
	if c.memberSuper == nil {
		for _, attribute := range obj.Attributes {
			if table, ok := attribute.(*InnerClassesAttribute); ok {
				if table == nil {
					return nil
				}
				for _, row := range table.Classes {
					if row == nil || !nativeProofWork(work, 1) {
						return nil
					}
					name, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
					if !known {
						return nil
					}
					if name == obj.GetSupperClassName() && row.OuterClassInfoIndex != 0 && row.InnerClassAccessFlags&8 == 0 {
						return nil
					}
				}
			}
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
	if i+1 >= len(ops) {
		return nil
	}
	mem := constructorMotionMember(obj, ops[i], core.OP_INVOKESPECIAL)
	if mem == nil || mem.Name != obj.GetSupperClassName() || mem.Member != "<init>" {
		return nil
	}
	c.superDescriptor = mem.Description
	c.sourceSuperDescriptor = mem.Description
	if c.memberSuper == nil && nullTail && members != nil {
		// A static named parent has no lexical enclosing operand, but its
		// private access bridge belongs to the same complete member family.
		// Record that parent so the physical marker projects through its
		// independently proved constructor instead of the root's bridge map.
		if parent := members.children[mem.Name]; parent != nil && parent.static {
			c.memberSuper = parent
		}
	}
	var bridge *nativeConstructorAccessBridge
	if c.memberSuper != nil {
		// Project the two independent compiler operands in their original
		// order: the leading enclosing instance and optional trailing unused
		// private-access marker. Both are backed by the named parent packet.
		parent := c.memberSuper
		// A foreign declaration supplies no private bridge or constructor
		// privilege. Its source transaction is committed independently.
		if members != nil && members.children[mem.Name] != parent && !nativeAnonymousForeignSuperConstructorAccessible(parent, owner, mem.Description, work) {
			return nil
		}
		target := mem.Description
		if nullTail {
			bridge = parent.accessBridges[target]
			if bridge == nil {
				return nil
			}
			target = bridge.target
		}
		if parent.object == nil || parent.object.GetClassName() != mem.Name {
			return nil
		}
		if parent.static {
			var known bool
			c.sourceSuperDescriptor, known = nativeMemberBridgeTargetSourceDescriptor(parent, bridge, work)
			if !known {
				return nil
			}
		} else {
			ctor := parent.constructors[target]
			if ctor == nil || ctor.descriptor != target {
				return nil
			}
			c.sourceSuperDescriptor = ctor.sourceDescriptor
		}
	} else if nullTail {
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
	if c.memberSuper != nil && !c.memberSuper.static {
		extra++
	}
	ds, ret, e := callbinding.Descriptor(mem.Description)
	if e != nil || ret != "V" || len(ds) != len(c.superParams)+extra {
		return nil
	}
	// javac can give the anonymous constructor a more specific physical
	// parameter than the erased SUPER declaration (e.g. T instantiated with
	// String). A plain original load is unchanged under reference widening;
	// narrowing, primitive conversion and unknown hierarchy facts are not.
	// Keep both descriptors and the actual parameter operands: the source
	// allocation still performs its independent exact overload/type binding.
	widening := newConstructorWideningQuery(func(name string) (callbinding.Class, bool) {
		if metadata == nil || !nativeProofWork(work, 1) {
			return callbinding.Class{}, false
		}
		return metadata(name)
	})
	for j, p := range c.superParams {
		index := j
		if c.memberSuper != nil && !c.memberSuper.static {
			index++
		}
		if !nativeProofWork(work, 1) || !widening.assignable(ps[p], ds[index]) {
			return nil
		}
	}
	if c.memberSuper != nil && !c.memberSuper.static && ds[0] != "L"+c.memberSuper.owner+";" {
		return nil
	}
	// Physical parameter identity is independent of its uses. The same unchanged
	// word may be stored in one capture and passed once to SUPER. A capture's
	// source operand is subsequently required to be stable at this allocation;
	// emitting both roles therefore cannot evaluate an effectful value twice.
	// Every physical parameter still needs an original witnessed role, and two
	// capture fields or repeated SUPER arguments need a separate capability.
	used := map[int]bool{}
	for _, p := range c.fields {
		if used[p] {
			return nil
		}
		used[p] = true
	}
	superUsed := map[int]bool{}
	for _, p := range c.superParams {
		if superUsed[p] {
			return nil
		}
		if used[p] {
			c.sharedParameterRoles = true
		}
		superUsed[p] = true
		used[p] = true
	}
	if omitUnusedEnclosing && len(ps) > 0 && ps[0] == "L"+owner+";" && !used[0] && len(used)+1 == len(ps) {
		// A lexical enclosing word remains a physical constructor parameter
		// after javac omits its unused field. It must be completely unread and
		// unwritten, not merely absent from the capture-store prefix.
		for _, op := range ops {
			if !nativeProofWork(work, 1) || core.GetRetrieveIdx(op) == 1 {
				return nil
			}
		}
		if !nativeEnumSelectorParametersUnchanged(ops, map[int]bool{1: true}) {
			return nil
		}
		c.unusedEnclosing = true
		used[0] = true
	}
	if len(used) != len(ps) {
		return nil
	}
	outer := -1
	captureOrder := []int{}
	for _, f := range obj.Fields {
		flags, _, known := nativeMemberEffectiveFieldFlags(f, work)
		if !known {
			return nil
		}
		name, _ := obj.getUtf8(f.NameIndex)
		if c.assertions != nil && name == nativeAssertionField {
			continue
		}
		if index, captured := c.fields[name]; captured {
			if flags != 0x1010 {
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
		if flags&0x0008 != 0 && (flags&0x0010 == 0 || !fieldHasConstantValue(f)) {
			return nil
		}
		if flags&0x1000 != 0 && flags&0x0008 == 0 {
			n, _ := obj.getUtf8(f.NameIndex)
			if _, known := c.fields[n]; !known {
				return nil
			}
		}
	}
	if c.parentAnonymous && outer < 0 && !c.unusedEnclosing {
		return nil
	}
	if outer >= 0 {
		c.enclosingField = outerField
	}
	next := 0
	ordered := map[int]bool{}
	if c.unusedEnclosing {
		next = 1
		ordered[0] = true
	}
	if outer >= 0 {
		if outer != next {
			return nil
		}
		next++
		ordered[outer] = true
	}
	for _, index := range c.superParams {
		if ordered[index] {
			continue
		}
		if index != next {
			return nil
		}
		next++
		ordered[index] = true
	}
	for _, index := range captureOrder {
		if ordered[index] {
			continue
		}
		if index != next {
			return nil
		}
		next++
		ordered[index] = true
	}
	if next != len(ps) || c.sharedParameterRoles && !nativeAnonymousInitializerStack(c, ps, code) {
		return nil
	}
	var initialized bool
	c.initializers, initialized = nativeAnonymousInitializerPackets(obj, ops, i+1, c, ps, work)
	if !initialized || len(c.initializers) > 0 && !nativeAnonymousInitializerStack(c, ps, code) {
		c.initializers = nil
		c.expressionInitializer = nativeAnonymousExpressionInitializerProof(obj, code, ops, i+1, c, work)
		if c.expressionInitializer == nil {
			return nil
		}
	}
	if len(lambdaMethods) != 0 {
		// Never borrow a prior packet's implementation cache. The complete
		// constructor and capture declarations above establish this context.
		context := lambda.lambdaContext
		context.localCaptures = nil
		context.factorySites = nil
		context.anonymousCaptures = c
		view := &nativeMemberClass{object: obj, lambdaContext: context}
		for _, implementation := range lambdaMethods {
			if !nativeMemberLambdaImplementation(view, implementation, work) {
				return nil
			}
		}
		c.lambdaImplementation = view
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
	return c.planNativeAnonymousOwnedGroup(members, forest, nil)
}

func (c *ClassObjectDumper) planNativeAnonymousOwnedGroup(members *nativeMemberFamily, forest *nativeAnonymousForest, root *nativeAnonymousIndependentRoot) *nativeAnonymousFamily {
	if c.foldSiblingResolver == nil || isGenuineEnum(c.obj) || c.getenv("JDEC_NATIVE_ANONYMOUS_OFF") != "" || !nativeSourceBinaryName(c.obj.GetClassName()) {
		return nil
	}
	if _, _, anon := originalAnonymousOwner(c.obj); anon && (forest == nil || forest.units[c.obj.GetClassName()] == nil) && !root.validFor(c) {
		return nil
	}
	p := &nativeAnonymousFamily{independentRoot: root, forest: forest, owner: c.obj.GetClassName(), children: map[string]*nativeAnonymousClass{}, bridges: map[string]*nativeConstructorAccessBridge{}, standalone: map[string]*ClassObject{}}
	assertionRoot := ""
	if forest != nil && forest.objects[forest.root] != nil && nativeMemberTopLevelEvidence(forest.objects[forest.root], c.Work) {
		assertionRoot = forest.root
	} else if members != nil && members.lexicalObjects[members.owner] != nil && nativeMemberTopLevelEvidence(members.lexicalObjects[members.owner], c.Work) {
		assertionRoot = members.owner
	} else if nativeMemberTopLevelEvidence(c.obj, c.Work) {
		assertionRoot = c.obj.GetClassName()
	}
	access := c.originalNativeConstructorAccessBridges()
	metadata := c.buildInvocationMetadata()
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
		if members != nil && members.enumSwitchTables[name] != nil {
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
		child := c.nativeAnonymousConstructorForCompiler(obj, owner, method, assertionRoot, members, forest, metadata, access)
		if child == nil || child.assertions != nil && c.options.TargetSourceVersion != 0 && c.options.TargetSourceVersion != 8 {
			p.standalone[name] = obj
			continue
		}
		p.children[name] = child
	}
	// javac numbers actual anonymous expressions consecutively. Only the
	// representable leading prefix can be reconstructed; leaving an earlier
	// declaration flat would renumber every subsequent anonymous expression.
	for ordinal := 1; ordinal <= len(names); ordinal++ {
		name := p.owner + "$" + strconv.Itoa(ordinal)
		if p.children[name] != nil {
			continue
		}
		for name, child := range p.children {
			if child.ordinal > ordinal {
				p.standalone[name] = child.object
				delete(p.children, name)
			}
		}
		break
	}
	if !c.nativeAnonymousStandaloneTailClosed(p, members, forest, metadata, access) {
		return nil
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
	if len(p.children) == 0 && (members == nil || len(p.standalone) == 0) {
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
	if p == nil || p.independentRoot != nil && !p.independentRoot.validFor(c) || !c.nativeAnonymousStandaloneTailClosed(p, members, forest, c.buildInvocationMetadata(), c.originalNativeConstructorAccessBridges()) {
		return nil
	}
	if c.obj.MajorVersion >= 55 && members == nil && forest == nil {
		modernNest, known := c.nativeModernNestOriginalScope()
		if !known {
			return nil
		}
		source := map[string]*ClassObject{c.obj.GetClassName(): c.obj}
		for name, child := range p.children {
			source[name] = child.object
		}
		if !nativeModernNestSourceScopeClosed(modernNest, source, c.Work) {
			return nil
		}
	}
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
		var sibling *ClassObject
		if forest != nil {
			sibling = forest.objects[name]
		}
		if sibling == nil {
			var err error
			sibling, err = c.parseResolved(raw)
			if err != nil || sibling.GetClassName() != name {
				return nil
			}
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
				if strings.Contains(descriptor, "L"+child+";") && !nativeAnonymousForestEnclosingDeclaration(sibling, member, forest, c.Work) && !nativeAnonymousAccessorDeclaration(forest, sibling, member, c.Work) && !nativeMemberJointBridgeDeclaration(members, sibling, member, child, c.Work) {
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
					if strings.Contains(desc, "L"+child+";") && !nativeAnonymousForestConstructorNameType(sibling, constantIndex+1, forest, c.Work) && !nativeAnonymousForestEnclosingNameType(sibling, constantIndex+1, forest, c.Work) && !nativeAnonymousForestCaptureNameType(forest, sibling, constantIndex+1, c.Work) && !nativeAnonymousAccessorNameType(forest, sibling, constantIndex+1, c.Work) && !jointBridgeTypes[constantIndex+1] {
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
						if !known || p.children[outer] != nil && !nativeAnonymousForestNamedRow(forest, object, row) {
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
		if bridgeNameTypes == nil {
			return nil
		}
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
					if strings.Contains(descriptor, "L"+child+";") && !nativeAnonymousForestConstructorNameType(object, constantIndex+1, forest, c.Work) && !nativeAnonymousForestEnclosingNameType(object, constantIndex+1, forest, c.Work) && !nativeAnonymousForestCaptureNameType(forest, object, constantIndex+1, c.Work) && !nativeAnonymousForestLambdaNameType(forest, object, constantIndex+1, c.Work) && !nativeAnonymousAccessorNameType(forest, object, constantIndex+1, c.Work) && !bridgeNameTypes[constantIndex+1] {
						return nil
					}
				}
			}
		}
		// Root declarations can use the same original anonymous constructor
		// marker as siblings. Compose the joint declaration certificate here
		// too; an anonymous name in an arbitrary descriptor remains unproved.
		for _, member := range append(append([]*MemberInfo{}, object.Fields...), object.Methods...) {
			descriptor, known := sourceBridgeUTF8(object, member.DescriptorIndex)
			if !known {
				return nil
			}
			for child := range p.children {
				name, _ := object.getUtf8(member.NameIndex)
				if strings.Contains(descriptor, "L"+child+";") && !nativeAnonymousForestEnclosingDeclaration(object, member, forest, c.Work) && !nativeAnonymousAccessorDeclaration(forest, object, member, c.Work) && !p.accessBridgeDescriptor(object, name, descriptor) && !nativeMemberJointBridgeDeclaration(members, object, member, child, c.Work) {
					return nil
				}
			}
		}
	}
	// A partial transaction must remain in its original source method. A
	// method-handle implementation may be lifted into a lambda, where its
	// parameter words no longer identify the enclosing source captures. That
	// transfer needs a separate capture-binding certificate before promotion.
	if len(p.standalone) != 0 && !nativeAnonymousAllocationMethodsStayLexical(c.obj, p, c.Work) {
		return nil
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
						if !nativeAnonymousAllocationScope(c.obj, child, mn, md, c.Work) {
							return nil
						}
					}
				}
				if member := constructorMotionMember(c.obj, op, core.OP_INVOKESPECIAL); member != nil && member.Member == "<init>" {
					if member.Name == p.owner && p.bridges[member.Description] != nil {
						return nil
					}
					if child := p.children[member.Name]; child != nil {
						if member.Description != child.descriptor || !nativeAnonymousAllocationScope(c.obj, child, mn, md, c.Work) {
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
	if forest := c.nativeAnonymousForest; forest != nil {
		prior := ctx.SourceClassDenotable
		ctx.SourceClassDenotable = func(name string) (bool, bool) {
			name = strings.ReplaceAll(name, ".", "/")
			if child := forest.units[name]; child != nil && child.object != nil && child.object.GetClassName() == name && forest.objects[name] == child.object {
				owner, method, anonymous := originalAnonymousOwner(child.object)
				if group := forest.groups[owner]; anonymous && group != nil && group.owner == owner && group.forest == forest && group.children[name] == child && method == child.method && nativeProofWork(c.Work, 1) {
					return false, true
				}
			}
			if prior != nil {
				return prior(name)
			}
			return false, false
		}
	}
	if c.nativeCaptureFields != nil {
		ctx.SourceCapturedFieldType = func(pc int, owner, name, descriptor string) any {
			if owner != c.obj.GetClassName() || c.nativeCapturedReads[ctx.FunctionName+ctx.CurrentMethodDesc][pc] != name {
				return nil
			}
			view := c.nativeCaptureTypes[name]
			shadowed, closed := nativeAnonymousMethodLexicalShadow(c)
			if !closed {
				c.nativeCaptureFailed = true
				return nil
			}
			if shadowed {
				// The captured variable retains its declaration outside this method.
				// Its same-spelled outer formal cannot name that declaration here.
				// Keep the authoritative original field descriptor as the IR view;
				// the source variable still binds to its original lexical declaration.
				return nil
			}
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
			if c.nativeMethodLocalCurrent != nil {
				text = "(/*jdec-owned-local-capture:" + c.obj.GetClassName() + ":" + field + "*/" + text + ")"
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
	ctx.SourceBranchSwap = func(left, right string) bool { return nativeAnonymousBranchSwap(p, left, right, c.Work) }
	ctx.SourceAnonymousCandidate = func(owner string) bool {
		// IR analysis may ask values for provisional text before source-local
		// identities are proved. Such a query cannot commit or reject ownership.
		return ctx.SourceCaptureStable != nil && p.children[strings.ReplaceAll(owner, ".", "/")] != nil
	}
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
		if child.unusedEnclosing && (len(args) == 0 || !args[0].Receiver) {
			return fail()
		}
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
		sub.nativeSourceAssertions = child.assertions
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
			if p.forest.members != nil {
				for owner, member := range p.forest.members.children {
					if !nativeProofWork(c.Work, 1) {
						return fail()
					}
					if !member.static {
						// Bind a lexical THIS in the actual expression's scope.
						// Its parent renderer may be outside a named declaration
						// owned by this anonymous object. Anonymous THIS itself
						// has no spelling: proved hidden SUPER operands are
						// consumed by delegation, never rendered as a qualifier.
						if p.forest.units[member.owner] != nil {
							continue
						}
						anchor := p.forest.members.anonymousNamedAnchor(member.owner)
						if anchor != "" {
							if !p.forest.members.anonymousNamedScopeContains(anchor, child.object.GetClassName()) {
								continue
							}
							source, known := p.forest.members.sourceName(member.owner)
							if !known {
								return fail()
							}
							sub.nativeAnonymousBindings[nativeMemberCaptureIndexKey(owner, member.field)] = source + ".this"
						} else {
							sub.nativeAnonymousBindings[nativeMemberCaptureIndexKey(owner, member.field)] = ctx.ShortTypeName(strings.ReplaceAll(member.owner, "/", ".")) + ".this"
						}
					}
				}
			}
			for field, text := range bindings {
				sub.nativeAnonymousBindings[nativeMemberCaptureIndexKey(child.object.GetClassName(), field)] = text
			}
		}
		sub.nativeCaptureFields = bindings
		sub.nativeAnonymousLambdaCurrent = child.lambdaImplementation
		sub.nativeCaptureTypes = captureTypes
		sub.nativeOuterContext = ctx
		sub.nativeTypeParams = append([]string(nil), ctx.TypeParams...)
		sub.nativeCapturedReads = map[string]map[int]string{}
		for _, m := range child.object.Methods {
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
		if e != nil || sub.nativeCaptureFailed || child.lambdaImplementation != nil && !nativeMemberLambdaSourceClosed(child.lambdaImplementation, sub, c.Work) || len(sub.constructorBoundaryHelpers) != 0 ||
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
		body, bodyKnown := javaClassBodyContentKnown(src)
		if !bodyKnown {
			return fail()
		}
		initialization, known := nativeAnonymousInitializerSource(child, bindings, sub.FuncCtx, sub.nativeAnnotationDeclarationResolver(), c.Work)
		if child.expressionInitializer != nil {
			initialization, known = sub.nativeAnonymousExpressionInitializerSource(child, bindings)
		}
		if !known {
			return fail()
		}
		body = initialization + body
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
				typ = nativeAnonymousArgumentSourceType(operand, ctx, c.Work)
				if typ == nil {
					return fail()
				}
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
			if c.nativeMemberRoot == nil || c.nativeMemberRoot.allocationClass(child.memberSuper.object.GetClassName()) != child.memberSuper || !nativeMemberJointAnonymousAccess(c.nativeMemberRoot, child.object.GetClassName(), c.Work) {
				return fail()
			}
			// This is the original anonymous constructor's SUPER binding.
			// Its instantiated superclass Signature supplies the leaf and
			// enclosing arguments; the allocation caller's unrelated extends
			// clause must never stand in for that declaration environment.
			binding = *nativeMemberBinding(sub.FuncCtx, c.nativeMemberRoot, c.Work)
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
		registration := ""
		if c.nativeMemberRoot != nil && c.nativeMemberRoot.anonymousUnits[child.object.GetClassName()] != nil && c.nativeMemberRoot.constructorBridges(child.object.GetSupperClassName())[child.superDescriptor] != nil {
			if !nativeAnonymousBridgeSuperOwned(c.nativeMemberRoot, child.object, "<init>", child.descriptor, child.object.GetSupperClassName(), child.superDescriptor, child.superPC, c.Work) {
				return fail()
			}
			// javac registers this private constructor before lowering the
			// anonymous body. Keep that original event before its getters.
			registration = nativeMemberConstructorRegistration(c.nativeMemberRoot, child.object.GetSupperClassName(), child.superDescriptor)
		}
		if child.sharedParameterRoles {
			c.appendDiagnostic(DecompileDiagnostic{Code: "anonymous_shared_parameter_metadata", Method: child.object.GetClassName() + ".<init>" + child.descriptor, Message: "The original constructor reuses an unchanged physical parameter for a proved capture store and SUPER argument. Both source uses read the same stable local or enclosing receiver. Recompilation may generate separate hidden capture parameters, changing this anonymous constructor's physical descriptor and parameter metadata; executable values, capture-before-SUPER order and source overload binding are preserved."})
		}
		return fmt.Sprintf("/*jdec-owned-anonymous-ordinal:%d:%s*/%snew %s(%s) {%s}", child.ordinal, p.owner, registration, parent, strings.Join(tuple, ","), body), true
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
	if f := c.nativeAnonymousForest; f != nil {
		for _, method := range f.consumers[c.obj.GetClassName()] {
			for _, consumer := range method {
				if consumer != nil && consumer.getter == nil && consumer.declaredField != nil {
					result[consumer.name] = true
				}
			}
		}
	}
	if p := c.nativeMemberRoot; p != nil {
		// Own static fields may be printed without a qualifier. Their original
		// declarations are field bindings, never missing generated JVM locals.
		// Reserve the names over actual local IDs as well as the source fallback.
		for _, field := range c.obj.Fields {
			if field != nil && field.AccessFlags&8 != 0 {
				if name, known := sourceBridgeUTF8(c.obj, field.NameIndex); known && class_context.SafeIdentifier(name) == name {
					result[name] = true
				}
			}
		}
		for _, getter := range p.getters {
			if c.nativeAnonymousForest != nil && c.nativeAnonymousForest.units[getter.owner] != nil {
				result[getter.field] = true
			}
			if getter.staticField && nativeStaticAccessorQualifierShadowed(c.FuncCtx.ShortTypeName(strings.ReplaceAll(getter.owner, "/", ".")), c.FuncCtx) {
				result[getter.field] = true
			}
		}
	}
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
	reserved := c.nativeLexicalCaptures()
	if c.nativeCaptureFields == nil && len(reserved) == 0 {
		return
	}
	var protected map[*coreutils.VariableId]bool
	if child := c.nativeMemberCurrent; child != nil && c.FuncCtx.FunctionName == "<init>" && len(params) > 0 {
		if outer, ok := params[0].(*values.JavaRef); ok && outer.Id != nil && c.FuncCtx.LocalNames[outer.Id] == c.FuncCtx.ShortTypeName(strings.ReplaceAll(child.owner, "/", "."))+".this" {
			protected = map[*coreutils.VariableId]bool{outer.Id: true}
		}
	}
	c.prepareNativeSourceNames(body, params, reserved, protected)
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
			roots, children, known := nativeSourceNameChildren(st)
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
		if c.nativeMemberRoot != nil {
			c.nativeMemberRoot.failed = true
		}
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
				left, bounded := nativeMemberEnclosingUnpack(assign.LeftValue, work)
				if !bounded {
					valid = false
					return
				}
				if target, ok := left.(*values.JavaRef); ok && target != nil && target.Id == ref.Id {
					if parameter || !(assign.IsDeclare || assign.IsFirst) || declaration != nil {
						valid = false
						return
					}
					declaration = assign
				}
			}
			_, children, known := nativeSourceNameChildren(st)
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
		return nil, nativeCaptureParameterUnwritten(body, ref, &remaining)
	}
	return declaration, nativeCaptureParameterUnwritten(body, ref, &remaining, declaration)
}
func (c *ClassObjectDumper) prepareNativeCaptureBindings(body []statements.Statement, params []values.JavaValue) {
	p := c.nativeAnonymousRoot
	if p == nil {
		return
	}
	ctx := c.FuncCtx
	relevant := false
	for _, child := range p.children {
		// Dominance/stability is a body proof, independent of the archive
		// ownership proof performed before source promotion. Only a physical
		// implementation crossing a lexical scope needs the extra certificate.
		if child.method == ctx.FunctionName+ctx.CurrentMethodDesc || child.method == "" && (ctx.FunctionName == "<init>" || ctx.FunctionName == "<clinit>") || nativeAnonymousAllocationScope(c.obj, child, ctx.FunctionName, ctx.CurrentMethodDesc, c.Work) {
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
	parameterDeclarations := map[*coreutils.VariableId]*values.JavaRef{}
	for _, v := range params {
		if ref, ok := v.(*values.JavaRef); ok && ref.Id != nil {
			parameterIDs[ref.Id] = true
			parameterDeclarations[ref.Id] = ref
		}
	}
	allowed := map[int]map[*coreutils.VariableId]bool{}
	names := &nativeCaptureNameBindings{}
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
					if !stable && !parameterIDs[ref.Id] {
						declaration, stable = nativeCaptureJoinedDeclaration(body, ref, alloc, c.Work)
					}
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
					if known && erasure != captureParams[index] && !parameterIDs[ref.Id] && visible[ref.Id] && stable && declaration != nil &&
						c.nativeCaptureWidenedDeclaration(body, ref, declaration, child, alloc, field, erasure, captureParams[index]) {
						declaredType = ref.WebDeclType
						erasure, known = values.SourceTypeErasure(declaredType, ctx)
					}
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
					// A rewritten body ref may share the declaration ID but no
					// longer carry parameter flags. The original declaration,
					// not a mutable use-site copy, witnesses its captured word.
					if captureName, known := c.nativeLambdaParameterCaptureName(parameterDeclarations[ref.Id]); known {
						name = captureName
					}
					if old, known := ctx.LocalNames[ref.Id]; known && old != name {
						p.failed = true
						continue
					}
					if !names.bind(name, ref.Id, visible, c.Work) {
						p.failed = true
						continue
					}
					ctx.LocalNames[ref.Id] = name
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
			roots, children, known := nativeSourceNameChildren(st)
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
	for name, bindings := range names.bindings {
		reserved[name] = true
		for id := range bindings {
			protected[id] = true
		}
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

// Method formals may shadow lexical formals. Their declarations remain intact;
// only an outer capture's source type spelling must not be rebound to them.
func nativeAnonymousMethodLexicalShadow(c *ClassObjectDumper) (bool, bool) {
	if c == nil || c.obj == nil || c.CurrentMethod == nil || c.FuncCtx == nil || c.nativeOuterContext == nil {
		return false, false
	}
	name, nok := sourceBridgeUTF8(c.obj, c.CurrentMethod.NameIndex)
	descriptor, dok := sourceBridgeUTF8(c.obj, c.CurrentMethod.DescriptorIndex)
	if !nok || !dok || name != c.FuncCtx.FunctionName || descriptor != c.FuncCtx.CurrentMethodDesc {
		return false, false
	}
	signature := ""
	seen := false
	for _, attribute := range c.CurrentMethod.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return false, false
		}
		if sig, ok := attribute.(*SignatureAttribute); ok {
			if sig == nil || seen {
				return false, false
			}
			seen = true
			var known bool
			signature, known = sourceBridgeUTF8(c.obj, sig.SignatureIndex)
			if !known {
				return false, false
			}
		}
	}
	if !seen {
		return false, true
	}
	return nativeAnonymousSignatureLexicalShadow(signature, c.nativeOuterContext, c.Work)
}

// Use original Signature metadata rather than the optional renderer's generic
// hint: disabling bound-receiver rendering cannot disable declaration identity.
func nativeAnonymousSignatureLexicalShadow(signature string, lexical *class_context.ClassContext, work *workbudget.Budget) (bool, bool) {
	if lexical == nil || signature == "" || !nativeProofWork(work, int64(len(signature)+1)) {
		return false, false
	}
	formals, _, known := types.SignatureTypeVariableReferences(signature)
	if !known || !strings.Contains(signature, "(") {
		return false, false
	}
	for _, name := range formals {
		if lexical.IsTypeParam(name) {
			return true, true
		}
	}
	return false, true
}

// Name binding needs operand dependencies, not permission to move or absorb a
// control transfer. Unknown text stays opaque even if it prints a familiar word.
func nativeSourceNameChildren(st statements.Statement) ([]values.JavaValue, [][]statements.Statement, bool) {
	if anchor, ok := st.(*statements.SourceAnchorStatement); ok {
		return nil, nil, anchor != nil
	}
	if middle, ok := st.(*statements.MiddleStatement); ok && middle != nil && middle.Flag == "monitor_exit" {
		// The dumper omits middle instructions from Java source. An original
		// sealed release has no rendered operand; its hidden receiver belongs
		// to the independent CFG ownership proof, not this namespace visitor.
		// A flag alone, changed payload, or enter operand cannot borrow this.
		_, _, known := middle.OriginalMonitor()
		return nil, nil, known
	}
	if leaf, ok := st.(*statements.CustomStatement); ok && leaf.HasSourceTransfer() {
		return nil, nil, leaf.SourceTransferOnly()
	}
	return catchSourceChildren(st)
}
