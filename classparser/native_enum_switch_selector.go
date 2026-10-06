package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A selector certificate is an ordered tree of original stack producers, not
// an inference from the enum result type. Every leaf identifies an unchanged
// descriptor parameter; every call retains its original invocation and operand
// order. No DUP, STORE, phi, allocation, or unsupported producer can disappear
// into this tree. javac's switch table read remains before this entire tree.
type nativeEnumSelectorProducer struct {
	opcode                            int
	pc, slot                          int
	owner, member, descriptor, result string
	operands                          []*nativeEnumSelectorProducer
}

func nativeEnumSelectorPacket(obj *ClassObject, ops []*core.OpCode, start int, params map[int]string, enum string, work *workbudget.Budget) (*nativeEnumSelectorProducer, int, bool) {
	if start < 0 || start >= len(ops) || !nativeProofWork(work, 64) || work != nil && work.CheckAlloc(64*1024) != nil {
		return nil, 0, false
	}
	stack := make([]*nativeEnumSelectorProducer, 0, 16)
	for i := start; i < len(ops) && i-start < 64; i++ {
		op := ops[i]
		if op == nil || op.Instr == nil {
			return nil, 0, false
		}
		if nativeEnumMemberOperand(obj, op, core.OP_INVOKEVIRTUAL, enum, "ordinal", "()I") && len(stack) == 1 && stack[0].result == "L"+enum+";" && i+2 < len(ops) && nativeEnumOpcode(ops[i+1], core.OP_IALOAD) && (nativeEnumOpcode(ops[i+2], core.OP_TABLESWITCH) || nativeEnumOpcode(ops[i+2], core.OP_LOOKUPSWITCH)) {
			return stack[0], i, true
		}
		slot := core.GetRetrieveIdx(op)
		if descriptor := params[slot]; descriptor != "" && constructorMotionLoad(op, descriptor) {
			stack = append(stack, &nativeEnumSelectorProducer{opcode: op.Instr.OpCode, pc: int(op.CurrentOffset), slot: slot, result: descriptor})
			continue
		}
		opcode := op.Instr.OpCode
		if opcode == core.OP_GETFIELD || opcode == core.OP_GETSTATIC {
			field := constructorMotionMember(obj, op, opcode)
			if field == nil {
				return nil, 0, false
			}
			// Parse a field descriptor with the same closed grammar used for
			// invocation results, including rejection of void/method payloads.
			_, result, err := callbinding.Descriptor("()" + field.Description)
			if err != nil || result == "V" {
				return nil, 0, false
			}
			node := &nativeEnumSelectorProducer{opcode: opcode, pc: int(op.CurrentOffset), owner: field.Name, member: field.Member, descriptor: field.Description, result: result}
			if opcode == core.OP_GETFIELD {
				if len(stack) == 0 || stack[len(stack)-1].result != "L"+field.Name+";" {
					return nil, 0, false
				}
				node.operands = []*nativeEnumSelectorProducer{stack[len(stack)-1]}
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, node)
			continue
		}
		if opcode != core.OP_INVOKESTATIC && opcode != core.OP_INVOKEVIRTUAL && opcode != core.OP_INVOKEINTERFACE {
			return nil, 0, false
		}
		member := constructorMotionMember(obj, op, opcode)
		if member == nil || member.Member == "<init>" || member.Member == "<clinit>" {
			return nil, 0, false
		}
		arguments, result, err := callbinding.Descriptor(member.Description)
		if err != nil || result == "V" || len(arguments) > 63 {
			return nil, 0, false
		}
		if opcode == core.OP_INVOKEINTERFACE {
			words := 1 // receiver plus JVM argument words, not source arity
			for _, descriptor := range arguments {
				words++
				if descriptor == "J" || descriptor == "D" {
					words++
				}
			}
			if words > 255 || int(op.Data[2]) != words {
				return nil, 0, false
			}
		}
		expect := append([]string(nil), arguments...)
		if opcode != core.OP_INVOKESTATIC {
			expect = append([]string{"L" + member.Name + ";"}, expect...)
		}
		if len(expect) > len(stack) {
			return nil, 0, false
		}
		base := len(stack) - len(expect)
		// Exact descriptors are a closed initial profile. Admitting subtype or
		// primitive conversions needs independent original binding evidence.
		for j, descriptor := range expect {
			if stack[base+j].result != descriptor {
				return nil, 0, false
			}
		}
		node := &nativeEnumSelectorProducer{opcode: opcode, pc: int(op.CurrentOffset), owner: member.Name, member: member.Member, descriptor: member.Description, result: result, operands: append([]*nativeEnumSelectorProducer(nil), stack[base:]...)}
		stack = append(stack[:base], node)
	}
	return nil, 0, false
}

func nativeEnumSelectorSource(original *nativeEnumSelectorProducer, value values.JavaValue, ctx *class_context.ClassContext, work *workbudget.Budget, depth int) bool {
	if original == nil || depth >= 32 || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(256+int64(len(original.result)+len(original.owner)+len(original.descriptor))*8) != nil {
		return false
	}
	v, bounded := nativeMemberEnclosingUnpack(value, work)
	if !bounded || sourceProofNil(v) {
		return false
	}
	// A source binding cast must be identity at this certified descriptor.
	// Runtime CHECKCASTs and conversion casts are producers of their own.
	if cast, ok := v.(*values.CastExpression); ok {
		erasure, known := values.SourceTypeErasure(cast.TargetType, ctx)
		return cast.Binding && known && erasure == original.result && nativeEnumSelectorSource(original, cast.Value, ctx, work, depth+1)
	}
	erasure, known := values.SourceTypeErasure(v.Type(), ctx)
	if !known || erasure != original.result {
		return false
	}
	if original.owner == "" {
		ref, ok := v.(*values.JavaRef)
		if !ok {
			return false
		}
		slot, known := ref.OriginalParameterSlot()
		if original.slot == 0 && !known {
			slot, known = ref.OriginalReceiverSlot()
		}
		return known && slot == original.slot
	}
	if original.opcode == core.OP_GETSTATIC {
		field, ok := v.(*values.JavaClassMember)
		return ok && field != nil && field.OriginalStaticFieldRead(original.pc, strings.ReplaceAll(field.Name, ".", "/"), original.member, original.descriptor) && strings.ReplaceAll(field.Name, ".", "/") == original.owner
	}
	if original.opcode == core.OP_GETFIELD {
		field, ok := v.(*values.RefMember)
		return ok && field != nil && len(original.operands) == 1 && field.OriginalInstanceFieldRead(original.pc, original.owner, original.member, original.descriptor) && nativeEnumSelectorSource(original.operands[0], field.Object, ctx, work, depth+1)
	}
	call, ok := v.(*values.FunctionCallExpression)
	if !ok || call == nil || !call.HasOriginPC || call.OriginPC != original.pc || len(call.ClassName) != len(original.owner) || strings.ReplaceAll(call.ClassName, ".", "/") != original.owner || call.FunctionName != original.member || call.Descriptor != original.descriptor || call.IsSpecialInvoke {
		return false
	}
	kind := values.InvokeVirtual
	if original.opcode == core.OP_INVOKESTATIC {
		kind = values.InvokeStatic
	} else if original.opcode == core.OP_INVOKEINTERFACE {
		kind = values.InvokeInterface
	}
	if call.Kind != kind || call.IsStatic != (kind == values.InvokeStatic) || len(call.Arguments) > 63 {
		return false
	}
	operands := call.Arguments
	if kind != values.InvokeStatic {
		if work != nil && work.CheckAlloc(int64(len(operands)+1)*16) != nil {
			return false
		}
		operands = append([]values.JavaValue{call.Object}, operands...)
	} else if call.Object != nil {
		// Decoded static calls carry a symbolic owner, not an evaluated
		// receiver. An expression or an ldc Class object cannot borrow it.
		owner, ok := call.Object.(*values.JavaClassValue)
		if !ok || owner == nil || owner.HasOriginPC {
			return false
		}
		descriptor, known := values.SourceTypeErasure(owner.Type(), ctx)
		if !known || descriptor != "L"+original.owner+";" {
			return false
		}
	}
	if len(operands) != len(original.operands) {
		return false
	}
	for i, operand := range operands {
		if !nativeEnumSelectorSource(original.operands[i], operand, ctx, work, depth+1) {
			return false
		}
	}
	return true
}

func nativeEnumSelectorSlots(node *nativeEnumSelectorProducer, slots map[int]bool, depth int) bool {
	if node == nil || depth >= 32 {
		return false
	}
	if node.owner == "" {
		slots[node.slot] = true
		if node.result == "J" || node.result == "D" {
			slots[node.slot+1] = true
		}
	}
	for _, operand := range node.operands {
		if !nativeEnumSelectorSlots(operand, slots, depth+1) {
			return false
		}
	}
	return true
}

// Writes are intervals of JVM words. A wide STORE can clobber either half of a
// long/double input or an adjacent single-word input. GetRetrieveIdx is a read
// query and cannot witness STORE targets; the shared local decoder also covers
// compact opcodes, IINC and WIDE without a separate spelling-based whitelist.
func nativeEnumSelectorParametersUnchanged(ops []*core.OpCode, slots map[int]bool) bool {
	for _, op := range ops {
		if op == nil || op.Instr == nil {
			return false
		}
		access := core.LocalAccessOf(op.Instr.OpCode)
		if !access.Write {
			continue
		}
		written := core.GetStoreIdx(op)
		if written < 0 {
			return false
		}
		for word := 0; word < access.Width; word++ {
			if slots[written+word] {
				return false
			}
		}
	}
	return true
}
