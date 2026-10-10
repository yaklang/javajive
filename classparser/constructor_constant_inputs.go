package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

// Facts are read from immutable original instructions, never printed operands,
// method names or a caller's customary inputs. A constructor remains callable
// with every argument of its original descriptor; parameter loads are unknown.
func constructorOriginalIntLiteral(obj *ClassObject, op *core.OpCode) (constructorEffectValue, bool) {
	unknown := constructorEffectValue{}
	if obj == nil || op == nil || op.Instr == nil || op.IsWide {
		return unknown, false
	}
	code := op.Instr.OpCode
	var word int32
	switch {
	case code >= core.OP_ICONST_M1 && code <= core.OP_ICONST_5:
		if len(op.Data) != 0 {
			return unknown, false
		}
		word = int32(code - core.OP_ICONST_0)
	case code == core.OP_BIPUSH:
		if len(op.Data) != 1 {
			return unknown, false
		}
		word = int32(int8(op.Data[0]))
	case code == core.OP_SIPUSH:
		if len(op.Data) != 2 {
			return unknown, false
		}
		word = int32(int16(core.Convert2bytesToInt(op.Data)))
	case code == core.OP_LDC || code == core.OP_LDC_W:
		index := 0
		if code == core.OP_LDC && len(op.Data) == 1 {
			index = int(op.Data[0])
		} else if code == core.OP_LDC_W && len(op.Data) == 2 {
			index = int(core.Convert2bytesToInt(op.Data))
		}
		if index <= 0 || index > len(obj.ConstantPool) {
			return unknown, false
		}
		constant, ok := obj.ConstantPool[index-1].(*ConstantIntegerInfo)
		if !ok || constant == nil {
			return unknown, false
		}
		word = constant.Value
	default:
		return unknown, false
	}
	return constructorEffectValue{kind: 'I', knownInt: true, intWord: word}, true
}

// A narrow input packet complements the existing full delegation binding proof.
// Complex producers keep the old unknown-input analysis. Even when another
// caller usually supplies a constant, an original parameter is never specialized.
func constructorOriginalArgumentFacts(obj *ClassObject, ops []*core.OpCode, start, invokePC int, params []string, slots map[int]int) ([]constructorEffectValue, bool) {
	if start < 0 || start >= len(ops) || core.GetRetrieveIdx(ops[start]) != 0 || !constructorMotionLoad(ops[start], "Ljava/lang/Object;") {
		return nil, false
	}
	var facts []constructorEffectValue
	for i := start + 1; i < len(ops) && i <= start+256; i++ {
		op := ops[i]
		if op == nil || op.Instr == nil {
			return nil, false
		}
		if int(op.CurrentOffset) == invokePC {
			member := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL)
			if member == nil || member.Member != "<init>" {
				return nil, false
			}
			formals, result, err := callbinding.Descriptor(member.Description)
			if err != nil || result != "V" || len(formals) != len(facts) {
				return nil, false
			}
			for j, p := range formals {
				if constructorEffectType(p).kind != facts[j].kind {
					return nil, false
				}
			}
			return facts, true
		}
		if literal, known := constructorOriginalIntLiteral(obj, op); known {
			facts = append(facts, literal)
			continue
		}
		slot := core.GetRetrieveIdx(op)
		param, ok := slots[slot]
		if !ok || slot <= 0 || param < 0 || param >= len(params) || !constructorMotionLoad(op, params[param]) {
			return nil, false
		}
		facts = append(facts, constructorEffectType(params[param]))
	}
	return nil, false
}

func constructorKnownIntBranch(opcode int, operands []constructorEffectValue) (bool, bool) {
	for _, v := range operands {
		if v.kind != 'I' || !v.knownInt {
			return false, false
		}
	}
	if len(operands) == 1 {
		n := operands[0].intWord
		switch opcode {
		case core.OP_IFEQ:
			return n == 0, true
		case core.OP_IFNE:
			return n != 0, true
		case core.OP_IFLT:
			return n < 0, true
		case core.OP_IFGE:
			return n >= 0, true
		case core.OP_IFGT:
			return n > 0, true
		case core.OP_IFLE:
			return n <= 0, true
		}
	} else if len(operands) == 2 {
		a, b := operands[0].intWord, operands[1].intWord
		switch opcode {
		case core.OP_IF_ICMPEQ:
			return a == b, true
		case core.OP_IF_ICMPNE:
			return a != b, true
		case core.OP_IF_ICMPLT:
			return a < b, true
		case core.OP_IF_ICMPGE:
			return a >= b, true
		case core.OP_IF_ICMPGT:
			return a > b, true
		case core.OP_IF_ICMPLE:
			return a <= b, true
		}
	}
	return false, false
}
