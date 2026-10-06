package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A default method-local constructor regenerates only the independently
// proved enclosing/captured symbols. This certificate says nothing about a
// source declaration's placement or private users; they must close jointly.
type nativeMethodLocalConstructor struct {
	descriptor, delegateOwner string
	delegatePC                int
	enclosingField            string
	captures                  map[string]int
	capturePCs                map[string]int
}

func originalMethodLocalDefaultConstructor(obj, enclosing *ClassObject, work *workbudget.Budget) (*nativeMethodLocalConstructor, bool) {
	owner, known := originalMethodLocalOwner(obj, enclosing, work)
	if !known || obj.AccessFlags != 0x20 && obj.AccessFlags != 0x30 {
		return nil, false
	}
	var constructor *MemberInfo
	var code *CodeAttribute
	for _, m := range obj.Methods {
		if m == nil || !nativeProofWork(work, 1) {
			return nil, false
		}
		name, named := sourceBridgeUTF8(obj, m.NameIndex)
		if !named {
			return nil, false
		}
		if name != "<init>" {
			continue
		}
		if constructor != nil || m.AccessFlags != 0 {
			return nil, false
		}
		constructor = m
		signatureSeen := false
		parameterSeen := false
		for _, a := range m.Attributes {
			if !nativeProofWork(work, 1) {
				return nil, false
			}
			switch attr := a.(type) {
			case *CodeAttribute:
			case *UnparsedAttribute:
				if attr == nil || attr.Name != "MethodParameters" || parameterSeen {
					return nil, false
				}
				parameterSeen = true
			case *SignatureAttribute:
				if attr == nil || signatureSeen {
					return nil, false
				}
				signatureSeen = true
				sig, known := sourceBridgeUTF8(obj, attr.SignatureIndex)
				if !known || sig != "()V" {
					return nil, false
				}
			default:
				return nil, false
			}
			if c, ok := a.(*CodeAttribute); ok {
				if code != nil || c == nil {
					return nil, false
				}
				code = c
			}
		}
	}
	if constructor == nil || code == nil || len(code.ExceptionTable) != 0 {
		return nil, false
	}
	descriptor, typed := sourceBridgeUTF8(obj, constructor.DescriptorIndex)
	params, result, err := callbinding.Descriptor(descriptor)
	if !typed || err != nil || result != "V" || !nativeMethodLocalConstructorParameters(obj, constructor, params, owner, false, work) {
		return nil, false
	}
	d := NewClassObjectDumper(obj)
	d.Work = work
	_, _, verified := d.nativeOriginalMethodSnapshot(constructor, code)
	if !verified {
		return nil, false
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return nil, false
	}
	ops := constructorMotionOps(decoder)
	if len(ops) > 512 {
		return nil, false
	}
	plan := &nativeMethodLocalConstructor{descriptor: descriptor, delegateOwner: obj.GetSupperClassName(), captures: map[string]int{}, capturePCs: map[string]int{}}
	slots := constructorParameterSlots(params)
	used := map[int]bool{}
	cursor := 0
	for cursor+2 < len(ops) {
		field := constructorMotionMember(obj, ops[cursor+2], core.OP_PUTFIELD)
		if field == nil {
			break
		}
		slot := core.GetRetrieveIdx(ops[cursor+1])
		param, found := slots[slot]
		if !nativeProofWork(work, 3) || !constructorMotionLoad(ops[cursor], "Ljava/lang/Object;") || core.GetRetrieveIdx(ops[cursor]) != 0 || !found || !constructorMotionLoad(ops[cursor+1], params[param]) || field.Name != obj.GetClassName() || field.Description != params[param] || !constructorMotionField(obj, field, true) || used[param] {
			return nil, false
		}
		if _, duplicate := plan.captures[field.Member]; duplicate {
			return nil, false
		}
		plan.captures[field.Member] = param
		plan.capturePCs[field.Member] = int(ops[cursor+2].CurrentOffset)
		used[param] = true
		if owner.declaration.AccessFlags&8 == 0 && param == 0 {
			if params[0] != "L"+owner.owner+";" {
				return nil, false
			}
			plan.enclosingField = field.Member
		}
		cursor += 3
	}
	// No explicit user argument, overwritten capture or residual initializer is
	// silently discarded. The source default constructor must regenerate every
	// actual parameter/store and the same no-argument parent call before return.
	if len(used) != len(params) || owner.declaration.AccessFlags&8 == 0 && plan.enclosingField == "" || cursor+3 != len(ops) || !constructorMotionLoad(ops[cursor], "Ljava/lang/Object;") || core.GetRetrieveIdx(ops[cursor]) != 0 {
		return nil, false
	}
	delegate := constructorMotionMember(obj, ops[cursor+1], core.OP_INVOKESPECIAL)
	if delegate == nil || delegate.Name != plan.delegateOwner || delegate.Member != "<init>" || delegate.Description != "()V" || ops[cursor+2].Instr.OpCode != core.OP_RETURN {
		return nil, false
	}
	plan.delegatePC = int(ops[cursor+1].CurrentOffset)
	fields := 0
	for _, f := range obj.Fields {
		if f == nil || !nativeProofWork(work, 1) {
			return nil, false
		}
		name, named := sourceBridgeUTF8(obj, f.NameIndex)
		descriptor, typed := sourceBridgeUTF8(obj, f.DescriptorIndex)
		param, captured := plan.captures[name]
		if !named || !typed || !captured || f.AccessFlags != 0x1010 || param >= len(params) || descriptor != params[param] {
			return nil, false
		}
		fields++
	}
	if fields != len(plan.captures) {
		return nil, false
	}
	return plan, true
}

// Captured/default constructors have no source parameters. Their original
// physical MethodParameters entries, when present, must therefore be unnamed
// mandated enclosing and synthetic captured arguments. A named parameter or
// unrelated attribute needs a different compiler profile, not silent erasure.
// The physical protocol certificate can accept an older absent table; source
// eligibility separately requires the tables our Java-8 compiler regenerates.
func nativeMethodLocalConstructorParameters(obj *ClassObject, method *MemberInfo, params []string, owner *nativeMethodLocalOwner, source bool, work *workbudget.Budget) bool {
	signatureSeen, parametersSeen := false, false
	for _, a := range method.Attributes {
		if !nativeProofWork(work, 1) {
			return false
		}
		switch attribute := a.(type) {
		case *CodeAttribute:
		case *SignatureAttribute:
			if attribute == nil || signatureSeen {
				return false
			}
			signatureSeen = true
			signature, known := sourceBridgeUTF8(obj, attribute.SignatureIndex)
			if !known || signature != "()V" {
				return false
			}
		case *UnparsedAttribute:
			if attribute == nil || parametersSeen || attribute.Name != "MethodParameters" || attribute.Length != uint32(1+4*len(params)) || len(attribute.Info) != 1+4*len(params) || int(attribute.Info[0]) != len(params) || !nativeProofWork(work, int64(len(attribute.Info))) {
				return false
			}
			parametersSeen = true
			for i := range params {
				at := 1 + i*4
				name := uint16(attribute.Info[at])<<8 | uint16(attribute.Info[at+1])
				flags := uint16(attribute.Info[at+2])<<8 | uint16(attribute.Info[at+3])
				wanted := uint16(0x1010)
				if i == 0 && owner.declaration.AccessFlags&8 == 0 {
					wanted = 0x8010
				}
				if name != 0 || flags != wanted {
					return false
				}
			}
		default:
			return false
		}
	}
	if !source {
		return true
	}
	if len(params) == 0 {
		return !signatureSeen && !parametersSeen
	}
	// Without -parameters, javac writes the hidden parameter table when an
	// enclosing instance is mandated. A static local has only capture arguments
	// and no table. Both are actual compiler protocols, not optional flag loss.
	wantParameters := owner.declaration.AccessFlags&8 == 0
	return obj.MajorVersion == 52 && signatureSeen && parametersSeen == wantParameters
}
