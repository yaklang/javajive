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
// publish or overwrite this receiver's captures. This deliberately small proof
// admits parameter-only field stores and constructor delegation; it rejects
// virtual calls, field reads, handlers, branches and every opaque effect.
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
	remaining := 512
	return c.constructorChainDoesNotObserve(call.Name, call.Description, writes, map[string]bool{}, &remaining, 0)
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
	if op == nil || op.Instr == nil || op.Instr.OpCode != opcode || len(op.Data) != 2 {
		return nil
	}
	member, _ := GetValueFromCP(obj.ConstantPool, int(core.Convert2bytesToInt(op.Data))).(*values.JavaClassMember)
	if member == nil {
		return nil
	}
	copy := *member
	copy.Name = strings.ReplaceAll(copy.Name, ".", "/")
	return &copy
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
		for _, attribute := range field.Attributes {
			if _, generic := attribute.(*SignatureAttribute); generic {
				return false
			}
		}
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
	obj := c.obj
	if owner != obj.GetClassName() {
		var raw []byte
		var ok bool
		if c.foldSiblingResolver != nil {
			raw, ok = c.foldSiblingResolver(owner)
		}
		if !ok && c.declarationResolver != nil {
			raw, ok = c.declarationResolver(owner)
		}
		var err error
		if !ok {
			return false
		}
		obj, err = c.parseResolved(raw)
		if err != nil || obj.GetClassName() != owner {
			return false
		}
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
	ops := constructorMotionOps(decoder)
	*remaining -= len(decoder.Opcodes())
	if *remaining < 0 {
		return false
	}
	params, _, err := callbinding.Descriptor(descriptor)
	if err != nil {
		return false
	}
	slots := constructorParameterSlots(params)
	index, delegated := constructorMotionDelegation(obj, ops, 0, params, slots)
	if delegated == nil || !c.constructorChainDoesNotObserve(delegated.Name, delegated.Description, writes, active, remaining, depth+1) {
		return false
	}
	for index < len(ops)-1 {
		if index+2 >= len(ops) || core.GetRetrieveIdx(ops[index]) != 0 || !constructorMotionLoad(ops[index], "Ljava/lang/Object;") {
			return false
		}
		slot := core.GetRetrieveIdx(ops[index+1])
		parameter, known := slots[slot]
		member := constructorMotionMember(obj, ops[index+2], core.OP_PUTFIELD)
		if !known || !constructorMotionLoad(ops[index+1], params[parameter]) || member == nil || member.Description != params[parameter] || !constructorMotionField(obj, member, false) || writes[member.Name+"\x00"+member.Member+"\x00"+member.Description] {
			return false
		}
		index += 3
	}
	return index == len(ops)-1 && ops[index].Instr.OpCode == core.OP_RETURN
}
