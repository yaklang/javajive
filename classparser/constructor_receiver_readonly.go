package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A call on THIS is ordinarily an observation/publication boundary. Admit only
// an exact own private/final declaration whose complete original body is an
// bounded return or one nonvolatile field/literal read and return. There is no
// virtual override, allocation, receiver publication or throwable computation.
// The caller still needs a closed-finalizer proof: method entry can fail, e.g.
// with StackOverflowError, even when its entire body has no throwing opcode.
// No method name participates in this proof.
func (c *ClassObjectDumper) constructorReceiverReadOnlyMethod(obj *ClassObject, member *values.JavaClassMember, opcode int, writes map[string]bool, remaining *int) (constructorEffectValue, bool) {
	if obj == nil || member == nil || member.Name != obj.GetClassName() || obj.AccessFlags&0x0200 != 0 || opcode != core.OP_INVOKEVIRTUAL && opcode != core.OP_INVOKESPECIAL {
		return constructorEffectValue{}, false
	}
	params, result, err := callbinding.Descriptor(member.Description)
	if err != nil || len(params) != 0 {
		return constructorEffectValue{}, false
	}
	var target *MemberInfo
	for _, method := range obj.Methods {
		*remaining--
		if *remaining < 0 {
			return constructorEffectValue{}, false
		}
		name, _ := obj.getUtf8(method.NameIndex)
		desc, _ := obj.getUtf8(method.DescriptorIndex)
		if name == member.Member && desc == member.Description {
			if target != nil {
				return constructorEffectValue{}, false
			}
			target = method
		}
	}
	if target == nil || target.AccessFlags&(0x0002|0x0010) == 0 || target.AccessFlags&(0x0008|0x0020|0x0100|0x0400) != 0 {
		return constructorEffectValue{}, false
	}
	var code *CodeAttribute
	for _, attribute := range target.Attributes {
		if candidate, ok := attribute.(*CodeAttribute); ok {
			if code != nil {
				return constructorEffectValue{}, false
			}
			code = candidate
		}
	}
	if code == nil || len(code.ExceptionTable) != 0 || code.MaxLocals < 1 || len(code.Code) > 16 || !nativeProofWork(c.Work, int64(len(code.Code))) {
		return constructorEffectValue{}, false
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return constructorEffectValue{}, false
	}
	ops := constructorMotionOps(decoder)
	*remaining -= len(ops)
	if *remaining < 0 {
		return constructorEffectValue{}, false
	}
	if result == "V" {
		return constructorEffectValue{}, len(ops) == 1 && ops[0].Instr.OpCode == core.OP_RETURN && len(ops[0].Data) == 0
	}
	value := constructorEffectType(result)
	returnOpcode := map[byte]int{'I': core.OP_IRETURN, 'J': core.OP_LRETURN, 'F': core.OP_FRETURN, 'D': core.OP_DRETURN, 'L': core.OP_ARETURN}[value.kind]
	if len(ops) < 2 || returnOpcode == 0 || ops[len(ops)-1].Instr.OpCode != returnOpcode || len(ops[len(ops)-1].Data) != 0 || int(code.MaxStack) < value.width() {
		return constructorEffectValue{}, false
	}
	if len(ops) == 2 {
		descriptor, known := constructorMotionLiteral(obj, ops[0])
		if !known {
			return constructorEffectValue{}, false
		}
		if descriptor == "null" {
			return value, value.kind == 'L'
		}
		// String/class literals may resolve or allocate; they are not inert reads.
		literal := constructorEffectType(descriptor)
		return value, literal.kind != 'L' && literal.kind == value.kind
	}
	if len(ops) != 3 || core.GetRetrieveIdx(ops[0]) != 0 || !constructorMotionLoad(ops[0], "Ljava/lang/Object;") {
		return constructorEffectValue{}, false
	}
	field := constructorMotionMember(obj, ops[1], core.OP_GETFIELD)
	if field == nil || constructorEffectType(field.Description).kind != value.kind {
		return constructorEffectValue{}, false
	}
	identity, known := c.constructorEffectField(obj, field, false, remaining)
	return value, known && !writes[identity]
}
