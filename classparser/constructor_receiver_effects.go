package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Values are logical JVM values, including category-2 width. Only the newly
// allocated receiver can carry receiver=true: publishing it into any other
// object, static field or opaque call is rejected. Therefore a receiver-free
// value loaded from a parameter/field/call cannot acquire an alias to it.
type constructorEffectValue struct {
	kind     byte
	receiver bool
}

func constructorEffectType(desc string) constructorEffectValue {
	if len(desc) == 0 {
		return constructorEffectValue{}
	}
	kind := desc[0]
	if kind == '[' || kind == 'L' {
		kind = 'L'
	} else if kind == 'Z' || kind == 'B' || kind == 'C' || kind == 'S' {
		kind = 'I'
	}
	return constructorEffectValue{kind: kind}
}

func (v constructorEffectValue) width() int {
	if v.kind == 'J' || v.kind == 'D' {
		return 2
	}
	if v.kind == 'I' || v.kind == 'F' || v.kind == 'L' {
		return 1
	}
	return 0
}

func (c *ClassObjectDumper) constructorMotionClass(owner string) (*ClassObject, bool) {
	if c.obj != nil && owner == c.obj.GetClassName() {
		return c.obj, true
	}
	var raw []byte
	var ok bool
	if c.foldSiblingResolver != nil {
		raw, ok = c.foldSiblingResolver(owner)
	}
	if !ok && c.declarationResolver != nil {
		raw, ok = c.declarationResolver(owner)
	}
	if !ok {
		return nil, false
	}
	obj, err := c.parseResolved(raw)
	return obj, err == nil && obj != nil && obj.GetClassName() == owner
}

// A Fieldref may name a subclass while resolving to an ancestor's field. Bind
// the symbolic owner first, then search its original declaration hierarchy.
// Parent and child fields named this$0 must remain different storage locations.
func (c *ClassObjectDumper) constructorEffectField(obj *ClassObject, member *values.JavaClassMember, remaining *int) (string, bool) {
	if member == nil {
		return "", false
	}
	symbolic := false
	seen := map[string]bool{}
	for depth := 0; obj != nil && depth <= 16; depth++ {
		*remaining--
		name := obj.GetClassName()
		if *remaining < 0 || seen[name] {
			return "", false
		}
		seen[name] = true
		symbolic = symbolic || name == member.Name
		if symbolic {
			matches := 0
			var flags uint16
			for _, field := range obj.Fields {
				n, _ := obj.getUtf8(field.NameIndex)
				d, _ := obj.getUtf8(field.DescriptorIndex)
				if n == member.Member && d == member.Description {
					matches++
					flags = field.AccessFlags
				}
			}
			if matches != 0 {
				return name + "\x00" + member.Member + "\x00" + member.Description, matches == 1 && flags&(0x0008|0x0040) == 0
			}
		}
		var ok bool
		obj, ok = c.constructorMotionClass(obj.GetSupperClassName())
		if !ok {
			return "", false
		}
	}
	return "", false
}

// This first effect domain admits straight-line receiver-independent scalar
// computation, local aliases, own/inherited ordinary field access and complete
// constructor delegation. Unknown control flow remains unsupported. A normal
// Java exception after Object initialization can expose the receiver to a
// finalizer, even without an explicit publication; potentially throwing opaque
// operations after initialization are therefore deliberately rejected.
func (c *ClassObjectDumper) constructorReceiverEffects(obj *ClassObject, code *CodeAttribute, ops []*core.OpCode, descriptor string, writes, active map[string]bool, remaining *int, depth int) bool {
	params, ret, err := callbinding.Descriptor(descriptor)
	if err != nil || ret != "V" || code.MaxLocals == 0 {
		return false
	}
	locals := make([]constructorEffectValue, int(code.MaxLocals))
	locals[0] = constructorEffectValue{kind: 'L', receiver: true}
	slot := 1
	for _, param := range params {
		value := constructorEffectType(param)
		if value.width() == 0 || slot+value.width() > len(locals) {
			return false
		}
		locals[slot] = value
		slot += value.width()
	}
	stack := []constructorEffectValue{}
	pop := func(kind byte) (constructorEffectValue, bool) {
		if len(stack) == 0 {
			return constructorEffectValue{}, false
		}
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return v, kind == 0 || v.kind == kind
	}
	initialized := false
	for index, op := range ops {
		// Check before each instruction, including local-load fast paths and
		// RETURN. A malformed declared stack limit cannot certify movement.
		words := 0
		for _, v := range stack {
			words += v.width()
		}
		if words > int(code.MaxStack) {
			return false
		}
		if op == nil || op.Instr == nil {
			return false
		}
		opcode := op.Instr.OpCode
		access := core.LocalAccessOf(opcode)
		if access.Read || access.Write {
			kind := byte(0)
			if opcode == core.OP_IINC {
				slot := core.GetRetrieveIdx(op)
				if slot < 0 || slot >= len(locals) || locals[slot].kind != 'I' {
					return false
				}
				continue
			}
			for category, k := range []byte{'I', 'J', 'F', 'D', 'L'} {
				if opcode == core.OP_ILOAD+category || opcode == core.OP_ISTORE+category || opcode >= core.OP_ILOAD_0+category*4 && opcode < core.OP_ILOAD_0+category*4+4 || opcode >= core.OP_ISTORE_0+category*4 && opcode < core.OP_ISTORE_0+category*4+4 {
					kind = k
				}
			}
			if kind == 0 {
				return false
			}
			if access.Read {
				slot := core.GetRetrieveIdx(op)
				if slot < 0 || slot+access.Width > len(locals) || locals[slot].kind != kind {
					return false
				}
				stack = append(stack, locals[slot])
			} else {
				v, ok := pop(kind)
				slot := core.GetStoreIdx(op)
				if !ok || slot < 0 || slot+v.width() > len(locals) {
					return false
				}
				// Overwriting either half invalidates the previous wide value.
				if slot > 0 && locals[slot-1].width() == 2 {
					locals[slot-1] = constructorEffectValue{}
				}
				if locals[slot].width() == 2 {
					locals[slot+1] = constructorEffectValue{}
				}
				locals[slot] = v
				if v.width() == 2 {
					locals[slot+1] = constructorEffectValue{}
				}
			}
			continue
		}
		switch {
		case opcode == core.OP_ACONST_NULL:
			stack = append(stack, constructorEffectValue{kind: 'L'})
		case opcode >= core.OP_ICONST_M1 && opcode <= core.OP_ICONST_5 || opcode == core.OP_BIPUSH || opcode == core.OP_SIPUSH:
			stack = append(stack, constructorEffectValue{kind: 'I'})
		case opcode >= core.OP_LCONST_0 && opcode <= core.OP_DCONST_1:
			kind := byte('F')
			if opcode <= core.OP_LCONST_1 {
				kind = 'J'
			} else if opcode >= core.OP_DCONST_0 {
				kind = 'D'
			}
			stack = append(stack, constructorEffectValue{kind: kind})
		case opcode == core.OP_LDC || opcode == core.OP_LDC_W || opcode == core.OP_LDC2_W:
			cp := 0
			if len(op.Data) == 1 {
				cp = int(op.Data[0])
			} else if len(op.Data) == 2 {
				cp = int(core.Convert2bytesToInt(op.Data))
			}
			if cp <= 0 || cp > len(obj.ConstantPool) {
				return false
			}
			kind := byte(0)
			switch obj.ConstantPool[cp-1].(type) {
			case *ConstantIntegerInfo:
				kind = 'I'
			case *ConstantFloatInfo:
				kind = 'F'
			case *ConstantLongInfo:
				kind = 'J'
			case *ConstantDoubleInfo:
				kind = 'D'
			case *ConstantStringInfo, *ConstantClassInfo:
				kind = 'L'
			}
			if _, class := obj.ConstantPool[cp-1].(*ConstantClassInfo); class && initialized {
				// A missing class can fail linkage after Object initialization.
				// Do not move captures past that potentially finalizable failure.
				return false
			}
			if kind == 0 || (opcode == core.OP_LDC2_W) != (kind == 'J' || kind == 'D') {
				return false
			}
			stack = append(stack, constructorEffectValue{kind: kind})
		case opcode == core.OP_DUP:
			v, ok := pop(0)
			if !ok || v.width() != 1 {
				return false
			}
			stack = append(stack, v, v)
		case opcode == core.OP_DUP2:
			v, ok := pop(0)
			if !ok {
				return false
			}
			if v.width() == 2 {
				stack = append(stack, v, v)
			} else {
				w, ok := pop(0)
				if !ok || w.width() != 1 {
					return false
				}
				stack = append(stack, w, v, w, v)
			}
		case opcode == core.OP_SWAP:
			v, ok := pop(0)
			w, ok2 := pop(0)
			if !ok || !ok2 || v.width() != 1 || w.width() != 1 {
				return false
			}
			stack = append(stack, v, w)
		case opcode == core.OP_POP || opcode == core.OP_POP2:
			v, ok := pop(0)
			if !ok {
				return false
			}
			if opcode == core.OP_POP && v.width() != 1 {
				return false
			}
			if opcode == core.OP_POP2 && v.width() == 1 {
				w, ok := pop(0)
				if !ok || w.width() != 1 {
					return false
				}
			}
		case opcode == core.OP_GETFIELD || opcode == core.OP_PUTFIELD:
			member := constructorMotionMember(obj, op, opcode)
			if member == nil {
				return false
			}
			typeOf := constructorEffectType(member.Description)
			if typeOf.width() == 0 {
				return false
			}
			if opcode == core.OP_PUTFIELD {
				value, ok := pop(typeOf.kind)
				if !ok || value.receiver {
					return false
				}
			}
			receiver, ok := pop('L')
			if !ok || !receiver.receiver {
				return false
			}
			field, known := c.constructorEffectField(obj, member, remaining)
			if !known || writes[field] {
				return false
			}
			if opcode == core.OP_GETFIELD {
				if !initialized {
					return false
				}
				stack = append(stack, typeOf)
			}
		case opcode == core.OP_INVOKESPECIAL:
			member := constructorMotionMember(obj, op, opcode)
			if member == nil || member.Member != "<init>" || initialized || (member.Name != obj.GetClassName() && member.Name != obj.GetSupperClassName()) {
				return false
			}
			args, result, err := callbinding.Descriptor(member.Description)
			if err != nil || result != "V" {
				return false
			}
			for i := len(args) - 1; i >= 0; i-- {
				v, ok := pop(constructorEffectType(args[i]).kind)
				if !ok || v.receiver {
					return false
				}
			}
			v, ok := pop('L')
			if !ok || !v.receiver || !c.constructorChainDoesNotObserve(member.Name, member.Description, writes, active, remaining, depth+1) {
				return false
			}
			initialized = true
		case opcode >= core.OP_IADD && opcode <= core.OP_DREM:
			kind := []byte{'I', 'J', 'F', 'D'}[(opcode-core.OP_IADD)%4]
			if kind == 'I' || kind == 'J' {
				if opcode >= core.OP_IDIV {
					return false
				}
			}
			_, ok := pop(kind)
			_, ok2 := pop(kind)
			if !ok || !ok2 {
				return false
			}
			stack = append(stack, constructorEffectValue{kind: kind})
		case opcode >= core.OP_INEG && opcode <= core.OP_DNEG:
			kind := []byte{'I', 'J', 'F', 'D'}[opcode-core.OP_INEG]
			_, ok := pop(kind)
			if !ok {
				return false
			}
			stack = append(stack, constructorEffectValue{kind: kind})
		case opcode >= core.OP_ISHL && opcode <= core.OP_LXOR:
			kind := byte('I')
			if (opcode-core.OP_ISHL)%2 == 1 {
				kind = 'J'
			}
			right := kind
			if opcode <= core.OP_LUSHR {
				right = 'I'
			}
			_, ok := pop(right)
			_, ok2 := pop(kind)
			if !ok || !ok2 {
				return false
			}
			stack = append(stack, constructorEffectValue{kind: kind})
		case opcode >= core.OP_I2L && opcode <= core.OP_I2S:
			conversions := [][2]byte{{'I', 'J'}, {'I', 'F'}, {'I', 'D'}, {'J', 'I'}, {'J', 'F'}, {'J', 'D'}, {'F', 'I'}, {'F', 'J'}, {'F', 'D'}, {'D', 'I'}, {'D', 'J'}, {'D', 'F'}, {'I', 'I'}, {'I', 'I'}, {'I', 'I'}}
			pair := conversions[opcode-core.OP_I2L]
			_, ok := pop(pair[0])
			if !ok {
				return false
			}
			stack = append(stack, constructorEffectValue{kind: pair[1]})
		case opcode == core.OP_RETURN:
			return initialized && len(stack) == 0 && index == len(ops)-1
		default:
			return false
		}
	}
	return false
}
