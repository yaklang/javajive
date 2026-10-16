package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Fresh confinement adds a source permission. A provider must not change the
// caller-owned root while that permission is established, including on the
// all-original-bytes path which has no later platform-profile replay.
func constructorConfinedRootSnapshot(object *ClassObject) (raw []byte, known bool) {
	defer func() {
		if recover() != nil {
			raw, known = nil, false
		}
	}()
	if object == nil {
		return nil, false
	}
	raw = object.Bytes()
	// The serializer reports unsupported/malformed trees with an empty result.
	// Two failed serializations must never stand for equal original evidence.
	return raw, len(raw) >= 10
}

// An exact NEW can copy its arguments into its own ordinary storage without
// publishing them. Keep the initialized result receiver-reaching, not THIS:
// later calls, field reads and publication cannot borrow THIS permissions.
// The caller separately proves the original receiver's finalization boundary.
func (c *ClassObjectDumper) constructorConfinedCapture(member *values.JavaClassMember, actuals []constructorEffectValue, remaining *int) (bool, bool) {
	if c == nil || member == nil || member.Member != "<init>" || len(member.Description) > 65535 || !c.constructorReceiverFinalizerSilent || remaining == nil || *remaining <= 0 || !nativeProofWork(c.Work, int64(len(member.Description))+1) {
		return false, false
	}
	object, known := c.constructorMotionClassWithSnapshot(member.Name, true)
	if !known || object == nil || object.GetClassName() != member.Name || object.GetSupperClassName() != "java/lang/Object" || object.AccessFlags&(0x0200|0x0400|0x2000|0x4000|0x8000) != 0 {
		return false, false
	}
	params, result, err := callbinding.Descriptor(member.Description)
	if err != nil || result != "V" || len(params) != len(actuals) || len(params) > 128 {
		return false, false
	}
	for i, param := range params {
		value := actuals[i]
		if value.kind != constructorEffectType(param).kind || value.allocation != 0 || value.reachesReceiver() && value.kind != 'L' || value.receiver && value.retainsReceiver {
			return false, false
		}
	}
	var selected *MemberInfo
	for _, method := range object.Methods {
		*remaining--
		if method == nil || *remaining < 0 || !nativeProofWork(c.Work, 1) {
			return false, false
		}
		name, nk := sourceBridgeUTF8(object, method.NameIndex)
		desc, dk := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !nk || !dk || name == "finalize" && desc == "()V" {
			return false, false
		}
		if name == "<init>" && desc == member.Description {
			if selected != nil || method.AccessFlags&(0x0008|0x0020|0x0100|0x0400) != 0 {
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
	if code == nil || len(code.ExceptionTable) != 0 || len(code.Code) > *remaining || len(code.Code) == 0 || !nativeProofWork(c.Work, int64(len(code.Code))) {
		return false, false
	}
	locals := nativeMemberParameterWidth(params) + 1
	if locals > 255 || int(code.MaxLocals) < locals || code.MaxStack < 1 || c.Work != nil && c.Work.CheckAlloc(int64(locals)*32+int64(len(code.Code))*64) != nil {
		return false, false
	}
	decoder := core.NewDecompiler(code.Code, nil)
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return false, false
	}
	ops := constructorMotionOps(decoder)
	if len(ops) < 3 || !constructorEffectOriginalOperandFree(code, ops[len(ops)-1]) || ops[len(ops)-1].Instr.OpCode != core.OP_RETURN {
		return false, false
	}
	slots := constructorParameterSlots(params)
	initialized, retained := false, false
	for i := 0; i < len(ops)-1; {
		*remaining--
		if *remaining < 0 || !nativeProofWork(c.Work, 1) || core.GetRetrieveIdx(ops[i]) != 0 || !constructorMotionLoad(ops[i], "Ljava/lang/Object;") {
			return false, false
		}
		if i+1 < len(ops)-1 {
			delegate := constructorMotionMember(object, ops[i+1], core.OP_INVOKESPECIAL)
			if delegate != nil && delegate.Name == "java/lang/Object" && delegate.Member == "<init>" && delegate.Description == "()V" {
				if initialized {
					return false, false
				}
				initialized = true
				i += 2
				continue
			}
		}
		if i+2 >= len(ops)-1 {
			return false, false
		}
		slot := core.GetRetrieveIdx(ops[i+1])
		index, parameter := slots[slot]
		field := constructorMotionMember(object, ops[i+2], core.OP_PUTFIELD)
		if !parameter || slot <= 0 || index >= len(actuals) || actuals[index].allocation != 0 || actuals[index].kind != constructorEffectType(params[index]).kind || !constructorMotionLoad(ops[i+1], params[index]) || int(code.MaxStack) < 1+actuals[index].width() || field == nil || field.Name != member.Name || field.Description != params[index] || !constructorMotionField(object, field, false, c.Work) {
			return false, false
		}
		retained = retained || actuals[index].reachesReceiver()
		i += 3
	}
	return retained, initialized
}
