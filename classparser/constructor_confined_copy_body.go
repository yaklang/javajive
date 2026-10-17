package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

// This domain has two distinct reference roles: the private, newly allocated
// holder and an input that can retain the original reader. Neither may be
// published. Only exact holder-field copies and original constructor
// delegations are admitted; a call on either role is never a storage copy.
type confinedCopyValue struct {
	kind             byte
	holder, retained bool
	allocation       int
}

func (c *ClassObjectDumper) constructorConfinedCaptureBody(object *ClassObject, code *CodeAttribute, ops []*core.OpCode, params []string, actuals []constructorEffectValue, remaining *int, active map[string]bool, depth int) (bool, bool) {
	allocations := map[int]string{}
	slots := constructorParameterSlots(params)
	offsets := map[int]int{}
	for i, op := range ops {
		if op == nil || op.Instr == nil {
			return false, false
		}
		offsets[int(op.CurrentOffset)] = i
	}
	branch := func(op *core.OpCode, index int) (int, bool) {
		if !constructorEffectOriginalBranch(code, op) {
			return 0, false
		}
		delta := int(int16(core.Convert2bytesToInt(op.Data)))
		target, ok := offsets[int(op.CurrentOffset)+delta]
		return target, ok && target > index // cyclic holder protocols need a separate invariant
	}
	var walk func(int, []confinedCopyValue, bool) (bool, bool)
	walk = func(start int, stack []confinedCopyValue, initialized bool) (bool, bool) {
		retained := false
		pop := func(kind byte) (confinedCopyValue, bool) {
			if len(stack) == 0 {
				return confinedCopyValue{}, false
			}
			value := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			return value, value.kind == kind
		}
		for i := start; i < len(ops); i++ {
			*remaining--
			if *remaining < 0 || !nativeProofWork(c.Work, 1) {
				return false, false
			}
			words := 0
			for _, v := range stack {
				words++
				if v.kind == 'J' || v.kind == 'D' {
					words++
				}
			}
			if words > int(code.MaxStack) {
				return false, false
			}
			op := ops[i]
			opcode := op.Instr.OpCode
			if constructorMotionLoad(op, "Ljava/lang/Object;") && core.GetRetrieveIdx(op) == 0 {
				stack = append(stack, confinedCopyValue{kind: 'L', holder: true})
				continue
			}
			if index, ok := slots[core.GetRetrieveIdx(op)]; ok && constructorMotionLoad(op, params[index]) {
				value := actuals[index]
				stack = append(stack, confinedCopyValue{kind: value.kind, retained: value.reachesReceiver()})
				continue
			}
			if descriptor, ok := constructorMotionLiteral(object, op); ok {
				if descriptor == "null" {
					descriptor = "Ljava/lang/Object;"
				}
				stack = append(stack, confinedCopyValue{kind: constructorEffectType(descriptor).kind})
				continue
			}
			switch {
			case opcode == core.OP_GETFIELD:
				field := constructorMotionMember(object, op, opcode)
				if !initialized || field == nil || !c.constructorConfinedPrimitiveField(object, field, remaining) {
					return false, false
				}
				receiver, ok := pop('L')
				if !ok || !receiver.holder {
					return false, false
				}
				stack = append(stack, confinedCopyValue{kind: constructorEffectType(field.Description).kind})
			case opcode == core.OP_INVOKESTATIC:
				factory := constructorMotionMember(object, op, opcode)
				if factory == nil || !initialized {
					return false, false
				}
				formals, result, err := callbinding.Descriptor(factory.Description)
				if err != nil || !callbinding.Reference(result) || len(formals) > 128 {
					return false, false
				}
				args := make([]constructorEffectValue, len(formals))
				for j := len(formals) - 1; j >= 0; j-- {
					value, ok := pop(constructorEffectType(formals[j]).kind)
					if !ok || value.allocation != 0 {
						return false, false
					}
					args[j] = constructorEffectValue{kind: value.kind, retainsReceiver: value.holder || value.retained}
				}
				capture, known := c.constructorConfinedCopyFactory(factory, args, remaining, active, depth+1)
				if !known {
					return false, false
				}
				stack = append(stack, confinedCopyValue{kind: 'L', retained: capture})
			case opcode == core.OP_PUTFIELD:
				field := constructorMotionMember(object, op, opcode)
				if field == nil || field.Name != object.GetClassName() || !constructorMotionField(object, field, false, c.Work) {
					return false, false
				}
				if p, r, err := callbinding.Descriptor("(" + field.Description + ")V"); err != nil || len(p) != 1 || r != "V" {
					return false, false
				}
				value, ok := pop(constructorEffectType(field.Description).kind)
				receiver, receiverOK := pop('L')
				if !ok || !receiverOK || !receiver.holder || value.holder || value.allocation != 0 {
					return false, false
				}
				retained = retained || value.retained
			case opcode == core.OP_NEW:
				if op.IsWide || len(op.Data) != 2 {
					return false, false
				}
				name, known := sourceBridgeClassName(object, core.Convert2bytesToInt(op.Data))
				if !known || !nativeProofWork(c.Work, int64(len(name))+1) {
					return false, false
				}
				token := int(op.CurrentOffset) + 1
				if token <= 0 {
					return false, false
				}
				allocations[token] = name
				stack = append(stack, confinedCopyValue{kind: 'L', allocation: token})
			case opcode == core.OP_DUP:
				if !constructorEffectOriginalOperandFree(code, op) || len(stack) == 0 || stack[len(stack)-1].kind != 'L' || stack[len(stack)-1].allocation == 0 {
					return false, false
				}
				stack = append(stack, stack[len(stack)-1])
			case opcode == core.OP_INSTANCEOF:
				if op.IsWide || len(op.Data) != 2 {
					return false, false
				}
				if _, known := sourceBridgeClassName(object, core.Convert2bytesToInt(op.Data)); !known {
					return false, false
				}
				value, ok := pop('L')
				if !ok || value.allocation != 0 {
					return false, false
				}
				stack = append(stack, confinedCopyValue{kind: 'I'})
			case opcode == core.OP_ATHROW:
				value, ok := pop('L')
				return retained, ok && !value.holder && !value.retained && value.allocation == 0 && len(stack) == 0 && constructorEffectOriginalOperandFree(code, op)
			case opcode == core.OP_INVOKESPECIAL:
				delegate := constructorMotionMember(object, op, opcode)
				if delegate == nil || delegate.Member != "<init>" {
					return false, false
				}
				formals, result, err := callbinding.Descriptor(delegate.Description)
				if err != nil || result != "V" || len(formals) > 128 {
					return false, false
				}
				args := make([]constructorEffectValue, len(formals))
				tainted := false
				for j := len(formals) - 1; j >= 0; j-- {
					value, ok := pop(constructorEffectType(formals[j]).kind)
					if !ok || value.allocation != 0 || value.holder && !initialized {
						return false, false
					}
					args[j] = constructorEffectValue{kind: value.kind, retainsReceiver: value.holder || value.retained}
					tainted = tainted || args[j].retainsReceiver
				}
				receiver, ok := pop('L')
				if !ok {
					return false, false
				}
				if receiver.allocation != 0 {
					if receiver.holder || allocations[receiver.allocation] != delegate.Name {
						return false, false
					}
					captured := false
					if tainted {
						var known bool
						captured, known = c.constructorConfinedCaptureChain(delegate, args, remaining, active, depth+1, true)
						if !known {
							return false, false
						}
					}
					for j := range stack {
						if stack[j].allocation == receiver.allocation {
							stack[j].allocation = 0
							stack[j].retained = captured
						}
					}
					continue
				}
				if !receiver.holder || initialized || (delegate.Name != object.GetClassName() && delegate.Name != object.GetSupperClassName()) {
					return false, false
				}
				captured, known := c.constructorConfinedCaptureChain(delegate, args, remaining, active, depth+1, false)
				if !known {
					return false, false
				}
				retained = retained || captured
				initialized = true
			case opcode == core.OP_IFNULL || opcode == core.OP_IFNONNULL || opcode >= core.OP_IFEQ && opcode <= core.OP_IF_ACMPNE:
				kind, count := byte('I'), 1
				if opcode >= core.OP_IF_ICMPEQ && opcode <= core.OP_IF_ACMPNE {
					count = 2
				}
				if opcode == core.OP_IFNULL || opcode == core.OP_IFNONNULL || opcode == core.OP_IF_ACMPEQ || opcode == core.OP_IF_ACMPNE {
					kind = 'L'
				}
				for j := 0; j < count; j++ {
					if v, ok := pop(kind); !ok || v.allocation != 0 {
						return false, false
					}
				}
				target, ok := branch(op, i)
				cost := len(stack) + 1
				*remaining -= cost
				if !ok || *remaining < 0 || !nativeProofWork(c.Work, int64(cost)) || c.Work != nil && c.Work.CheckAlloc(int64(cost)*32) != nil {
					return false, false
				}
				a, aOK := walk(target, append([]confinedCopyValue(nil), stack...), initialized)
				b, bOK := walk(i+1, stack, initialized)
				return retained || a || b, aOK && bOK
			case opcode == core.OP_GOTO:
				target, ok := branch(op, i)
				if !ok {
					return false, false
				}
				a, known := walk(target, stack, initialized)
				return retained || a, known
			case opcode == core.OP_RETURN:
				return retained, initialized && len(stack) == 0 && constructorEffectOriginalOperandFree(code, op)
			default:
				return false, false
			}
		}
		return false, false
	}
	return walk(0, nil, false)
}
