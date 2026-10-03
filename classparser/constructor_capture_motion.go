package javaclassparser

import (
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
	// The remaining pre-delegation operands are the receiver and direct
	// parameter loads. No expression can observe an earlier capture write.
	next, call := constructorMotionDelegation(c.obj, ops, index, params, slots)
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
	if obj == nil || op == nil || op.Instr == nil || op.Instr.OpCode != opcode || len(op.Data) != 2 {
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
	switch item := constant(core.Convert2bytesToInt(op.Data)).(type) {
	case *ConstantFieldrefInfo:
		if item != nil && (opcode == core.OP_PUTFIELD || opcode == core.OP_GETFIELD) {
			ref = &item.ConstantMemberrefInfo
		}
	case *ConstantMethodrefInfo:
		if item != nil && opcode == core.OP_INVOKESPECIAL {
			ref = &item.ConstantMemberrefInfo
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

func constructorMotionDelegation(obj *ClassObject, ops []*core.OpCode, start int, params []string, slots map[int]int) (int, *values.JavaClassMember) {
	if start >= len(ops) || core.GetRetrieveIdx(ops[start]) != 0 || !constructorMotionLoad(ops[start], "Ljava/lang/Object;") {
		return 0, nil
	}
	arguments := []string{}
	index := start + 1
	for index < len(ops) {
		if ops[index].Instr.OpCode == core.OP_INVOKESPECIAL {
			member := constructorMotionMember(obj, ops[index], core.OP_INVOKESPECIAL)
			if member == nil || member.Member != "<init>" || (member.Name != obj.GetClassName() && member.Name != obj.GetSupperClassName()) {
				return 0, nil
			}
			formals, ret, err := callbinding.Descriptor(member.Description)
			if err != nil || ret != "V" || len(formals) != len(arguments) {
				return 0, nil
			}
			for i := range formals {
				if formals[i] != arguments[i] {
					return 0, nil
				}
			}
			return index + 1, member
		}
		slot := core.GetRetrieveIdx(ops[index])
		parameter, ok := slots[slot]
		if !ok || slot == 0 || !constructorMotionLoad(ops[index], params[parameter]) {
			return 0, nil
		}
		arguments = append(arguments, params[parameter])
		index++
	}
	return 0, nil
}

func (c *ClassObjectDumper) constructorChainDoesNotObserve(owner, descriptor string, writes, active map[string]bool, remaining *int, depth int) bool {
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
	return c.constructorReceiverEffects(obj, code, ops, descriptor, writes, active, remaining, depth)
}
