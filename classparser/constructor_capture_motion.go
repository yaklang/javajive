package javaclassparser

import (
	"strconv"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A flattened inner class cannot write captures before super in Java source.
// Such writes commute only when the original constructor chain cannot read,
// publish or overwrite this receiver's captures. The receiver-effect analysis
// follows local aliases and original field identities, rather than requiring
// every ancestor to have one particular assignment/delegation source shape.
func (c *ClassObjectDumper) constructorCapturesCommute(p *constructorSourceBoundary, code *CodeAttribute, method *MemberInfo, decoder *core.Decompiler) bool {
	if p == nil || p.delegate == nil || len(p.prefix) == 0 || len(code.ExceptionTable) != 0 {
		return false
	}
	desc, err := c.obj.getUtf8(method.DescriptorIndex)
	if err != nil {
		return false
	}
	params, _, err := callbinding.Descriptor(desc)
	if err != nil || len(params) != len(p.params) {
		return false
	}
	slots := constructorParameterSlots(params)
	refs := map[int]*values.JavaRef{}
	for slot, index := range slots {
		ref, ok := p.params[index].(*values.JavaRef)
		if !ok || ref == nil || !ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil {
			return false
		}
		refs[slot] = ref
	}
	ops := constructorMotionOps(decoder)
	index := 0
	writes := map[string]bool{}
	for _, statement := range p.prefix {
		assign, ok := statement.(*statements.AssignStatement)
		if !ok || assign == nil || assign.IsDeclare || assign.ArrayMember != nil || index+2 >= len(ops) {
			return false
		}
		field, ok := values.UnpackSoltValue(assign.LeftValue).(*values.RefMember)
		if !ok || field == nil {
			return false
		}
		receiver, ok := values.UnpackSoltValue(field.Object).(*values.JavaRef)
		if !ok || receiver == nil || !receiver.IsThis {
			return false
		}
		slot := core.GetRetrieveIdx(ops[index+1])
		member := constructorMotionMember(c.obj, ops[index+2], core.OP_PUTFIELD)
		if core.GetRetrieveIdx(ops[index]) != 0 || !constructorMotionLoad(ops[index], "Ljava/lang/Object;") || refs[slot] == nil || !constructorMotionLoad(ops[index+1], params[slots[slot]]) || values.UnpackSoltValue(assign.JavaValue) != refs[slot] || member == nil || member.Name != c.obj.GetClassName() || member.Member != field.Member || member.Description != params[slots[slot]] || !constructorMotionField(c.obj, member, true) {
			return false
		}
		key := member.Name + "\x00" + member.Member + "\x00" + member.Description
		if writes[key] {
			return false
		}
		writes[key] = true
		index += 3
	}
	// Remaining operands are original receiver-free computations evaluated
	// before initialization. None can observe an early capture on this receiver.
	next, call := constructorMotionDelegation(c.obj, ops, index, params, slots, c.FuncCtx.InvocationMetadata)
	if call == nil || next == 0 || int(ops[next-1].CurrentOffset) != p.pc || call.Name != strings.ReplaceAll(p.delegate.ClassName, ".", "/") || call.Description != p.delegate.Descriptor {
		return false
	}
	return c.constructorCaptureChainDoesNotObserve(call.Name, call.Description, writes)
}

func constructorParameterSlots(params []string) map[int]int {
	result := map[int]int{}
	slot := 1
	for index, descriptor := range params {
		result[slot] = index
		slot++
		if descriptor == "J" || descriptor == "D" {
			slot++
		}
	}
	return result
}

func constructorMotionOps(decoder *core.Decompiler) []*core.OpCode {
	result := []*core.OpCode{}
	for index, op := range decoder.Opcodes() {
		if index == 0 && op != nil && op.Instr != nil && op.Instr.OpCode == core.OP_START {
			continue
		}
		if op != nil && op.Instr != nil && op.Instr.OpCode != core.OP_NOP {
			result = append(result, op)
		}
	}
	return result
}

func constructorMotionMember(obj *ClassObject, op *core.OpCode, opcode int) *values.JavaClassMember {
	if obj == nil || op == nil || op.Instr == nil || op.Instr.OpCode != opcode {
		return nil
	}
	if opcode == core.OP_INVOKEINTERFACE {
		if len(op.Data) != 4 || op.Data[3] != 0 {
			return nil
		}
	} else if len(op.Data) != 2 {
		return nil
	}
	// This proof must reject incomplete/wrong-kind symbolic references rather
	// than allowing the general expression decoder to panic or coerce them.
	constant := func(index uint16) ConstantInfo {
		if index == 0 || int(index) > len(obj.ConstantPool) {
			return nil
		}
		return obj.ConstantPool[index-1]
	}
	var ref *ConstantMemberrefInfo
	interfaceRef := false
	switch item := constant(core.Convert2bytesToInt(op.Data[:2])).(type) {
	case *ConstantFieldrefInfo:
		if item != nil && (opcode == core.OP_PUTFIELD || opcode == core.OP_GETFIELD || opcode == core.OP_GETSTATIC) {
			ref = &item.ConstantMemberrefInfo
		}
	case *ConstantMethodrefInfo:
		if item != nil && (opcode == core.OP_INVOKESPECIAL || opcode == core.OP_INVOKESTATIC || opcode == core.OP_INVOKEVIRTUAL) {
			ref = &item.ConstantMemberrefInfo
		}
	case *ConstantInterfaceMethodrefInfo:
		if item != nil && (opcode == core.OP_INVOKEINTERFACE || (obj.MajorVersion >= 52 && (opcode == core.OP_INVOKESTATIC || opcode == core.OP_INVOKESPECIAL))) {
			ref = &item.ConstantMemberrefInfo
			interfaceRef = true
		}
	}
	if ref == nil {
		return nil
	}
	owner, ok := constant(ref.ClassIndex).(*ConstantClassInfo)
	nameType, ok2 := constant(ref.NameAndTypeIndex).(*ConstantNameAndTypeInfo)
	if !ok || !ok2 || owner == nil || nameType == nil {
		return nil
	}
	className, ok := constant(owner.NameIndex).(*ConstantUtf8Info)
	name, ok2 := constant(nameType.NameIndex).(*ConstantUtf8Info)
	desc, ok3 := constant(nameType.DescriptorIndex).(*ConstantUtf8Info)
	if !ok || !ok2 || !ok3 || className == nil || name == nil || desc == nil {
		return nil
	}
	if interfaceRef && name.Value == "<init>" {
		return nil
	}
	return &values.JavaClassMember{Name: className.Value, Member: name.Value, Description: desc.Value}
}

func constructorMotionLoad(op *core.OpCode, descriptor string) bool {
	if op == nil || op.Instr == nil || descriptor == "" {
		return false
	}
	category := 0
	switch descriptor[0] {
	case 'J':
		category = 1
	case 'F':
		category = 2
	case 'D':
		category = 3
	case 'L', '[':
		category = 4
	case 'I', 'Z', 'B', 'C', 'S':
	default:
		return false
	}
	opcode := op.Instr.OpCode
	return opcode == core.OP_ILOAD+category || opcode >= core.OP_ILOAD_0+category*4 && opcode < core.OP_ILOAD_0+category*4+4
}

func constructorMotionField(obj *ClassObject, member *values.JavaClassMember, capture bool) bool {
	if member == nil || member.Name != obj.GetClassName() {
		return false
	}
	count := 0
	for _, field := range obj.Fields {
		name, _ := obj.getUtf8(field.NameIndex)
		desc, _ := obj.getUtf8(field.DescriptorIndex)
		if name != member.Member || desc != member.Description {
			continue
		}
		count++
		if field.AccessFlags&(0x0008|0x0040) != 0 || capture && field.AccessFlags&(0x0010|0x1000) != (0x0010|0x1000) {
			return false
		}
		// The movement proof concerns the erased, original field storage and
		// parameter identity. Signature still governs source binding elsewhere;
		// its presence does not make this field observe a different capture.
	}
	return count == 1
}

func constructorMotionDelegation(obj *ClassObject, ops []*core.OpCode, start int, params []string, slots map[int]int, metadata callbinding.Provider, enclosingSlots ...int) (int, *values.JavaClassMember) {
	return constructorMotionDelegationEnclosing(obj, ops, start, params, slots, metadata, nil, enclosingSlots...)
}

// A supplied lexical path comes only from the complete original member forest.
// Preserve a distinct origin through its exact GETFIELD PCs; casts, calls or
// other computations destroy that origin and cannot justify omission.
func constructorMotionDelegationEnclosing(obj *ClassObject, ops []*core.OpCode, start int, params []string, slots map[int]int, metadata callbinding.Provider, path *nativeMemberLexicalRead, enclosingSlots ...int) (int, *values.JavaClassMember) {
	reads := map[int]*nativeMemberLexicalRead{}
	tags := map[int]int{}
	chain := []*nativeMemberLexicalRead{}
	for read := path; read != nil; read = read.prior {
		if len(chain) >= 64 {
			return 0, nil
		}
		chain = append(chain, read)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		read := chain[i]
		if reads[read.pc] != nil {
			return 0, nil
		}
		reads[read.pc] = read
		tags[read.pc] = -2 - (len(chain) - 1 - i)
	}

	if start < 0 || start >= len(ops) || ops[start] == nil || ops[start].Instr == nil || core.GetRetrieveIdx(ops[start]) != 0 || !constructorMotionLoad(ops[start], "Ljava/lang/Object;") {
		return 0, nil
	}
	arguments := []string{}
	// Exact local origins complement erased assignability. Duplication retains
	// an origin; casts, field reads, computations and calls do not establish
	// identity with the original enclosing parameter. The optional source
	// projection may omit only that exact first delegation operand.
	origins := []int{}
	appendArgument := func(descriptor string, slot int) {
		arguments = append(arguments, descriptor)
		origins = append(origins, slot)
	}
	allocations := map[string]string{}
	// Track only newly allocated reference arrays on this operand stack.
	// Stores must consume that exact allocation origin, an int index and an
	// assignable reference element; parameter arrays never borrow this proof.
	freshArrays := map[int]string{}
	widening := newConstructorWideningQuery(metadata)
	index := start + 1
	for index < len(ops) && index-start <= 512 {
		if ops[index] == nil || ops[index].Instr == nil {
			return 0, nil
		}
		if ops[index].Instr.OpCode == core.OP_NEW {
			if len(ops[index].Data) != 2 {
				return 0, nil
			}
			owner, known := sourceBridgeClassName(obj, core.Convert2bytesToInt(ops[index].Data))
			if !known || strings.HasPrefix(owner, "[") {
				return 0, nil
			}
			decl, known := widening.class(owner)
			if !known || !decl.MembersComplete || decl.IsInterface {
				return 0, nil
			}
			token := "@allocation:" + strconv.Itoa(index)
			allocations[token] = owner
			appendArgument(token, -1)
			index++
			continue
		}

		if ops[index].Instr.OpCode == core.OP_ANEWARRAY {
			if len(ops[index].Data) != 2 || len(arguments) == 0 || arguments[len(arguments)-1] != "I" {
				return 0, nil
			}
			component, known := sourceBridgeClassName(obj, core.Convert2bytesToInt(ops[index].Data))
			if !known {
				return 0, nil
			}
			if !strings.HasPrefix(component, "[") {
				component = "L" + component + ";"
			}
			descriptor := "[" + component
			ps, _, err := callbinding.Descriptor("(" + descriptor + ")V")
			if err != nil || len(ps) != 1 {
				return 0, nil
			}
			arguments = arguments[:len(arguments)-1]
			origins = origins[:len(origins)-1]
			origin := -1000 - index
			freshArrays[origin] = descriptor
			appendArgument(descriptor, origin)
			index++
			continue
		}
		if ops[index].Instr.OpCode == core.OP_AASTORE {
			if len(ops[index].Data) != 0 || len(arguments) < 3 {
				return 0, nil
			}
			base := len(arguments) - 3
			descriptor, known := freshArrays[origins[base]]
			if !known || arguments[base] != descriptor || arguments[base+1] != "I" || !callbinding.Reference(descriptor[1:]) || !widening.assignable(arguments[base+2], descriptor[1:]) {
				return 0, nil
			}
			arguments = arguments[:base]
			origins = origins[:base]
			index++
			continue
		}
		if ops[index].Instr.OpCode == core.OP_DUP {
			if len(ops[index].Data) != 0 || len(arguments) == 0 {
				return 0, nil
			}
			value := arguments[len(arguments)-1]
			_, allocated := allocations[value]
			if !allocated && value != "null" && constructorEffectType(value).width() != 1 {
				return 0, nil
			}
			appendArgument(value, origins[len(origins)-1])
			index++
			continue
		}
		if ops[index].Instr.OpCode == core.OP_INVOKESPECIAL {
			member := constructorMotionMember(obj, ops[index], core.OP_INVOKESPECIAL)
			if member == nil || member.Member != "<init>" {
				return 0, nil
			}
			formals, ret, err := callbinding.Descriptor(member.Description)
			if err != nil || ret != "V" || len(arguments) < len(formals) {
				return 0, nil
			}
			base := len(arguments) - len(formals)
			for i := range formals {
				if !widening.assignable(arguments[base+i], formals[i]) {
					return 0, nil
				}
			}
			if base == 0 {
				wanted := 0
				if path != nil {
					wanted = tags[path.pc]
				} else if len(enclosingSlots) == 1 {
					wanted = enclosingSlots[0]
				}
				if (path != nil || len(enclosingSlots) > 0) && (len(enclosingSlots) > 1 || len(origins) == 0 || origins[0] != wanted) {
					return 0, nil
				}
				if member.Name != obj.GetClassName() && member.Name != obj.GetSupperClassName() {
					return 0, nil
				}
				return index + 1, member
			}
			// A different freshly allocated receiver may be constructed while
			// THIS remains uninitialized. Its exact NEW-site token cannot be
			// passed to a call, cast or field access until this matching init.
			token := arguments[base-1]
			allocation, exists := allocations[token]
			if !exists || allocation != member.Name || !widening.constructor(member) {
				return 0, nil
			}
			arguments = arguments[:base-1]
			origins = origins[:base-1]
			for i, value := range arguments {
				if value == token {
					arguments[i] = "L" + allocation + ";"
				}
			}
			delete(allocations, token)
			index++
			continue
		}
		if ops[index].Instr.OpCode == core.OP_CHECKCAST {
			// The original cast is evaluated before Object initialization, so
			// a cast/linkage failure cannot expose this receiver by finalization.
			// Only an already receiver-free operand is on this argument stack.
			if len(ops[index].Data) != 2 || len(arguments) == 0 || arguments[len(arguments)-1] != "null" && !callbinding.Reference(arguments[len(arguments)-1]) {
				return 0, nil
			}
			name, known := sourceBridgeClassName(obj, core.Convert2bytesToInt(ops[index].Data))
			if !known {
				return 0, nil
			}
			descriptor := name
			if !strings.HasPrefix(name, "[") {
				descriptor = "L" + name + ";"
			}
			if ps, _, err := callbinding.Descriptor("(" + descriptor + ")V"); err != nil || len(ps) != 1 {
				return 0, nil
			}
			arguments[len(arguments)-1] = descriptor
			origins[len(origins)-1] = -1
			index++
			continue
		}
		if opcode := ops[index].Instr.OpCode; opcode == core.OP_INVOKESTATIC || opcode == core.OP_INVOKEVIRTUAL || opcode == core.OP_INVOKEINTERFACE {
			// THIS stays outside the argument stack. Original receiver-free
			// calls may have effects and throw, but remain before initialization
			// and cannot publish the uninitialized receiver. Bind the original
			// descriptor, opcode and declared method instead of assuming purity.
			member := constructorMotionMember(obj, ops[index], opcode)
			if member == nil || !widening.invocation(member, opcode) {
				return 0, nil
			}
			formals, result, err := callbinding.Descriptor(member.Description)
			if err != nil || len(arguments) < len(formals) {
				return 0, nil
			}
			words := 1
			base := len(arguments) - len(formals)
			for i, formal := range formals {
				words += constructorEffectType(formal).width()
				if !widening.assignable(arguments[base+i], formal) {
					return 0, nil
				}
			}
			if opcode == core.OP_INVOKEINTERFACE && (words > 255 || int(ops[index].Data[2]) != words) {
				return 0, nil
			}
			arguments = arguments[:base]
			origins = origins[:base]
			if opcode != core.OP_INVOKESTATIC {
				if len(arguments) == 0 || !widening.assignable(arguments[len(arguments)-1], "L"+member.Name+";") {
					return 0, nil
				}
				arguments = arguments[:len(arguments)-1]
				origins = origins[:len(origins)-1]
			}
			if result != "V" {
				appendArgument(result, -1)
			}
			index++
			continue
		}
		if ops[index].Instr.OpCode == core.OP_ARRAYLENGTH {
			if len(ops[index].Data) != 0 || len(arguments) == 0 || arguments[len(arguments)-1] != "null" && !strings.HasPrefix(arguments[len(arguments)-1], "[") {
				return 0, nil
			}
			arguments[len(arguments)-1] = "I"
			origins[len(origins)-1] = -1
			index++
			continue
		}
		if opcode := ops[index].Instr.OpCode; opcode == core.OP_GETFIELD || opcode == core.OP_GETSTATIC {
			// Only external references enter the argument stack: the original
			// uninitialized THIS is held outside it. Reading an external field
			// cannot recover an alias to THIS because the leading captures have
			// not published it. Keep the original read and its linkage, null and
			// class-initialization failures before the original delegate.
			member := constructorMotionMember(obj, ops[index], opcode)
			if member == nil {
				return 0, nil
			}
			fields, _, err := callbinding.Descriptor("(" + member.Description + ")V")
			if err != nil || len(fields) != 1 {
				return 0, nil
			}
			origin := -1
			if opcode == core.OP_GETFIELD {
				if len(arguments) == 0 || !widening.assignable(arguments[len(arguments)-1], "L"+member.Name+";") {
					return 0, nil
				}
				if read := reads[int(ops[index].CurrentOffset)]; read != nil {
					prior := 1
					if read.prior != nil {
						prior = tags[read.prior.pc]
					}
					if origins[len(origins)-1] != prior || member.Name != read.owner || member.Member != read.field || member.Description != read.descriptor {
						return 0, nil
					}
					origin = tags[read.pc]
				}
				arguments = arguments[:len(arguments)-1]
				origins = origins[:len(origins)-1]
			}
			appendArgument(fields[0], origin)
			index++
			continue
		}
		if literal, proved := constructorMotionLiteral(obj, ops[index]); proved {
			appendArgument(literal, -1)
		} else {
			slot := core.GetRetrieveIdx(ops[index])
			parameter, ok := slots[slot]
			if !ok || parameter < 0 || parameter >= len(params) || slot == 0 || !constructorMotionLoad(ops[index], params[parameter]) {
				return 0, nil
			}
			appendArgument(params[parameter], slot)
		}
		index++
	}
	return 0, nil
}

// Conversion proves that the original operand can enter the original formal;
// it does not select a source overload. The original invocation descriptor and
// source constructor binding remain authoritative. Share a bounded immutable
// hierarchy query across all operands, including a guard on retained edges.
type constructorWideningQuery struct {
	provider         callbinding.Provider
	classes          map[string]callbinding.Class
	known            map[string]bool
	remaining        int
	exhausted        bool
	methods          map[string]map[string][]callbinding.Method
	remainingMethods int
}

func newConstructorWideningQuery(provider callbinding.Provider) *constructorWideningQuery {
	return &constructorWideningQuery{provider: provider, classes: map[string]callbinding.Class{}, known: map[string]bool{}, remaining: 64, methods: map[string]map[string][]callbinding.Method{}, remainingMethods: 512}
}

func (q *constructorWideningQuery) assignable(actual, formal string) bool {
	return callbinding.Assignable(actual, formal, q.class) && !q.exhausted
}

func (q *constructorWideningQuery) class(name string) (callbinding.Class, bool) {
	if ok, seen := q.known[name]; seen {
		return q.classes[name], ok
	}
	q.remaining--
	if q.remaining < 0 {
		q.exhausted = true
		return callbinding.Class{}, false
	}
	if q.provider == nil {
		q.known[name] = false
		return callbinding.Class{}, false
	}
	class, known := q.provider(name)
	known = known && class.Name == name && class.ParentsComplete
	if known && len(class.Parents) > q.remaining {
		q.exhausted = true
		known = false
	}
	if known {
		q.remaining -= len(class.Parents)
		class.Parents = append([]string(nil), class.Parents...)
		q.classes[name] = class
	}
	q.known[name] = known
	return class, known
}

// Resolve one exact original method through a bounded declaration graph.
// Static interface methods are not inherited. Multiple inherited candidates,
// unknown declarations, wrong opcode kinds and cycles cannot certify a call.
func (q *constructorWideningQuery) invocation(member *values.JavaClassMember, opcode int) bool {
	if member == nil || member.Member == "<init>" || member.Member == "<clinit>" || opcode != core.OP_INVOKESTATIC && opcode != core.OP_INVOKEVIRTUAL && opcode != core.OP_INVOKEINTERFACE {
		return false
	}
	seen := map[string]bool{}
	var find func(string) (int, bool)
	find = func(name string) (int, bool) {
		if seen[name] {
			return 0, false
		}
		seen[name] = true
		defer delete(seen, name)
		q.remaining--
		if q.remaining < 0 {
			q.exhausted = true
			return 0, false
		}
		decl, known := q.class(name)
		if !known || !decl.MembersComplete {
			return 0, false
		}
		methods, indexed := q.methodIndex(decl)
		if !indexed {
			return 0, false
		}
		count := 0
		for _, method := range methods[member.Member+"\x00"+member.Description] {
			if method.Static != (opcode == core.OP_INVOKESTATIC) {
				return 0, false
			}
			count++
		}
		if count != 0 {
			return count, count == 1
		}
		if decl.IsInterface && opcode == core.OP_INVOKESTATIC {
			return 0, false
		}
		for _, parent := range decl.Parents {
			n, ok := find(parent)
			if !ok {
				return 0, false
			}
			count += n
		}
		return count, true
	}
	decl, known := q.class(member.Name)
	if !known || opcode == core.OP_INVOKEINTERFACE && !decl.IsInterface || opcode == core.OP_INVOKEVIRTUAL && decl.IsInterface {
		return false
	}
	n, ok := find(member.Name)
	return ok && n == 1 && !q.exhausted
}

func (q *constructorWideningQuery) constructor(member *values.JavaClassMember) bool {
	if member == nil || member.Member != "<init>" {
		return false
	}
	decl, known := q.class(member.Name)
	if !known || !decl.MembersComplete || decl.IsInterface {
		return false
	}
	methods, indexed := q.methodIndex(decl)
	if !indexed {
		return false
	}
	count := 0
	for _, method := range methods["<init>\x00"+member.Description] {
		if method.Static {
			return false
		}
		count++
	}
	return count == 1 && !q.exhausted
}

// Index each exact declaration once, with a separate shared method-entry
// budget. A large but ordinary SDK method table is one hierarchy node, not
// hundreds of hierarchy edges. Repeated calls never rescan the same table.
func (q *constructorWideningQuery) methodIndex(decl callbinding.Class) (map[string][]callbinding.Method, bool) {
	if methods, known := q.methods[decl.Name]; known {
		return methods, true
	}
	if len(decl.Methods) > q.remainingMethods {
		q.exhausted = true
		return nil, false
	}
	q.remainingMethods -= len(decl.Methods)
	methods := map[string][]callbinding.Method{}
	for _, method := range decl.Methods {
		key := method.Name + "\x00" + method.Desc
		methods[key] = append(methods[key], method)
	}
	q.methods[decl.Name] = methods
	return methods, true
}

func (c *ClassObjectDumper) constructorChainDoesNotObserve(owner, descriptor string, writes, active map[string]bool, remaining *int, depth int) bool {
	aliases := &constructorSelfStorageProof{}
	return c.constructorChainEffects(owner, descriptor, writes, active, remaining, depth, aliases) && aliases.closed()
}

func (c *ClassObjectDumper) constructorChainEffects(owner, descriptor string, writes, active map[string]bool, remaining *int, depth int, aliases *constructorSelfStorageProof) bool {
	if depth > 16 || *remaining <= 0 {
		return false
	}
	if owner == "java/lang/Object" && descriptor == "()V" {
		exceptions, known := exactInvocationExceptions(c.FuncCtx.InvocationMetadata, owner, "<init>", descriptor)
		return known && len(exceptions) == 0
	}
	key := owner + "\x00" + descriptor
	if active[key] {
		return false
	}
	active[key] = true
	defer delete(active, key)
	obj, ok := c.constructorMotionClass(owner)
	if !ok {
		return false
	}
	var code *CodeAttribute
	matches := 0
	for _, method := range obj.Methods {
		name, _ := obj.getUtf8(method.NameIndex)
		desc, _ := obj.getUtf8(method.DescriptorIndex)
		if name == "<init>" && desc == descriptor {
			matches++
			for _, attribute := range method.Attributes {
				if candidate, ok := attribute.(*CodeAttribute); ok {
					if code != nil {
						return false
					}
					code = candidate
				}
			}
		}
	}
	if matches != 1 || code == nil || len(code.ExceptionTable) != 0 {
		return false
	}
	decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, index) })
	if decoder.ParseOpcode() != nil {
		return false
	}
	ops := []*core.OpCode{}
	for _, op := range decoder.Opcodes() {
		if op != nil && op.Instr != nil && op.Instr.OpCode != core.OP_START {
			ops = append(ops, op)
		}
	}
	if len(decoder.Opcodes()) > *remaining {
		return false
	}
	return c.constructorReceiverEffectsWithStorage(obj, code, ops, descriptor, writes, active, remaining, depth, aliases)
}

// Literal operands keep their original values and widths. Class/method-handle/
// dynamic constants can have linkage or bootstrap effects and require a separate
// proof; never admit them merely because they produce a reference on the stack.
func constructorMotionLiteral(obj *ClassObject, op *core.OpCode) (string, bool) {
	if obj == nil || op == nil || op.Instr == nil {
		return "", false
	}
	opcode := op.Instr.OpCode
	switch {
	case opcode == core.OP_ACONST_NULL && len(op.Data) == 0:
		return "null", true
	case opcode >= core.OP_ICONST_M1 && opcode <= core.OP_ICONST_5 && len(op.Data) == 0:
		return "I", true
	case opcode == core.OP_BIPUSH && len(op.Data) == 1 || opcode == core.OP_SIPUSH && len(op.Data) == 2:
		return "I", true
	case opcode >= core.OP_LCONST_0 && opcode <= core.OP_DCONST_1 && len(op.Data) == 0:
		if opcode <= core.OP_LCONST_1 {
			return "J", true
		}
		if opcode >= core.OP_DCONST_0 {
			return "D", true
		}
		return "F", true
	case opcode == core.OP_LDC || opcode == core.OP_LDC_W || opcode == core.OP_LDC2_W:
		index := 0
		if opcode == core.OP_LDC && len(op.Data) == 1 {
			index = int(op.Data[0])
		} else if opcode != core.OP_LDC && len(op.Data) == 2 {
			index = int(core.Convert2bytesToInt(op.Data))
		} else {
			return "", false
		}
		if index <= 0 || index > len(obj.ConstantPool) {
			return "", false
		}
		descriptor := ""
		switch constant := obj.ConstantPool[index-1].(type) {
		case *ConstantIntegerInfo:
			if constant != nil {
				descriptor = "I"
			}
		case *ConstantFloatInfo:
			if constant != nil {
				descriptor = "F"
			}
		case *ConstantLongInfo:
			if constant != nil {
				descriptor = "J"
			}
		case *ConstantDoubleInfo:
			if constant != nil {
				descriptor = "D"
			}
		case *ConstantStringInfo:
			if constant != nil {
				if utf, valid := NewConstantPoolWithConstant(&obj.ConstantPool).IndexInfo(int(constant.StringIndex)).(*ConstantUtf8Info); valid && utf != nil {
					descriptor = "Ljava/lang/String;"
				}
			}
		}
		return descriptor, descriptor != "" && (opcode == core.OP_LDC2_W) == (descriptor == "J" || descriptor == "D")
	}
	return "", false
}
