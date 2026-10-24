package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"strings"
)

// A copy factory returns exactly one fresh allocation after its original
// constructor has confined all incoming arguments. It cannot return an input,
// publish it, invoke a callback, or borrow THIS's dispatch/type permissions.
// Class initialization and constructor failures stay at their original call.
func (c *ClassObjectDumper) constructorConfinedCopyFactory(member *values.JavaClassMember, actuals []constructorEffectValue, remaining *int, active map[string]bool, depth int) (bool, bool) {
	if c == nil || member == nil || member.Member == "<init>" || member.Member == "<clinit>" || remaining == nil || *remaining <= 0 || depth > 16 || len(member.Description) > 65535 {
		return false, false
	}
	params, result, err := callbinding.Descriptor(member.Description)
	if err != nil || !callbinding.Reference(result) || len(params) != len(actuals) || len(params) > 128 {
		return false, false
	}
	slots, width := map[int]int{}, 0
	for i, param := range params {
		if actuals[i].kind != constructorEffectType(param).kind || actuals[i].allocation != 0 || !constructorValidNullFact(actuals[i]) {
			return false, false
		}
		slots[width] = i
		width += actuals[i].width()
	}
	key := "factory\x00" + member.Name + "\x00" + member.Member + "\x00" + member.Description
	if width > 255 || active[key] {
		return false, false
	}
	active[key] = true
	defer delete(active, key)
	object, known := c.constructorMotionClassWithSnapshot(member.Name, true)
	if !known {
		return false, false
	}
	var selected *MemberInfo
	for _, method := range object.Methods {
		*remaining--
		if method == nil || *remaining < 0 || !nativeProofWork(c.Work, 1) {
			return false, false
		}
		name, nk := sourceBridgeUTF8(object, method.NameIndex)
		desc, dk := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !nk || !dk {
			return false, false
		}
		if name == member.Member && desc == member.Description {
			if selected != nil || method.AccessFlags&0x0008 == 0 || method.AccessFlags&(0x0020|0x0100|0x0400) != 0 {
				return false, false
			}
			selected = method
		}
	}
	if selected == nil {
		return false, false
	}
	var code *CodeAttribute
	for _, attr := range selected.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return false, false
		}
		if body, ok := attr.(*CodeAttribute); ok {
			if body == nil || code != nil {
				return false, false
			}
			code = body
		}
	}
	if code == nil || len(code.Code) == 0 || len(code.Code) > *remaining || len(code.ExceptionTable) != 0 || int(code.MaxLocals) < width || c.Work != nil && c.Work.CheckAlloc(int64(len(code.Code))*64+int64(width)*32) != nil {
		return false, false
	}
	decoder := core.NewDecompiler(code.Code, nil)
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return false, false
	}
	ops := constructorMotionOps(decoder)
	if len(ops) < 4 || ops[0].Instr.OpCode != core.OP_NEW || len(ops[0].Data) != 2 || ops[1].Instr.OpCode != core.OP_DUP || !constructorEffectOriginalOperandFree(code, ops[1]) || ops[len(ops)-1].Instr.OpCode != core.OP_ARETURN || !constructorEffectOriginalOperandFree(code, ops[len(ops)-1]) {
		return false, false
	}
	owner, known := sourceBridgeClassName(object, core.Convert2bytesToInt(ops[0].Data))
	ctor := constructorMotionMember(object, ops[len(ops)-2], core.OP_INVOKESPECIAL)
	if !known || ctor == nil || ctor.Name != owner || ctor.Member != "<init>" {
		return false, false
	}
	formals, ret, err := callbinding.Descriptor(ctor.Description)
	if err != nil || ret != "V" || len(formals) != len(ops)-4 || len(formals) > 128 {
		return false, false
	}
	args, words := make([]constructorEffectValue, len(formals)), 2
	for i, op := range ops[2 : len(ops)-2] {
		*remaining--
		if *remaining < 0 || !nativeProofWork(c.Work, 1) {
			return false, false
		}
		if parameter, ok := slots[core.GetRetrieveIdx(op)]; ok && constructorMotionLoad(op, params[parameter]) {
			args[i] = actuals[parameter]
		} else if literal, known := constructorMotionLiteral(object, op); known {
			if literal == "null" {
				literal = "Ljava/lang/Object;"
			}
			args[i] = constructorEffectType(literal)
		} else {
			return false, false
		}
		if args[i].kind != constructorEffectType(formals[i]).kind {
			return false, false
		}
		words += args[i].width()
	}
	if words > int(code.MaxStack) {
		return false, false
	}
	return c.constructorConfinedCaptureChain(ctor, args, remaining, active, depth+1, true)
}

// Primitive holder fields cannot expose the retained reader. Resolve the exact
// symbolic field through private original snapshots and charge every field scan.
func (c *ClassObjectDumper) constructorConfinedPrimitiveField(object *ClassObject, member *values.JavaClassMember, remaining *int) bool {
	if object == nil || member == nil || remaining == nil {
		return false
	}
	params, result, err := callbinding.Descriptor("(" + member.Description + ")V")
	if err != nil || result != "V" || len(params) != 1 || constructorEffectType(member.Description).kind == 'L' {
		return false
	}
	caller := object.GetClassName()
	pkg := func(name string) string {
		if n := strings.LastIndexByte(name, '/'); n >= 0 {
			return name[:n]
		}
		return ""
	}
	seen := map[string]bool{}
	symbolic := false
	for depth := 0; object != nil && depth <= 16; depth++ {
		name := object.GetClassName()
		cost := len(object.Fields) + 1
		*remaining -= cost
		if seen[name] || *remaining < 0 || !nativeProofWork(c.Work, int64(cost)) {
			return false
		}
		seen[name] = true
		symbolic = symbolic || name == member.Name
		if symbolic {
			matches := 0
			var flags uint16
			for _, field := range object.Fields {
				if field == nil {
					return false
				}
				n, nk := sourceBridgeUTF8(object, field.NameIndex)
				d, dk := sourceBridgeUTF8(object, field.DescriptorIndex)
				if !nk || !dk {
					return false
				}
				if n == member.Member && d == member.Description {
					matches++
					flags = field.AccessFlags
				}
			}
			if matches != 0 {
				accessible := name == caller || flags&0x0001 != 0 || flags&0x0004 != 0 || flags&0x0002 == 0 && pkg(name) == pkg(caller)
				return matches == 1 && flags&(0x0008|0x0040) == 0 && accessible
			}
		}
		var known bool
		object, known = c.constructorMotionClassWithSnapshot(object.GetSupperClassName(), true)
		if !known {
			return false
		}
	}
	return false
}
