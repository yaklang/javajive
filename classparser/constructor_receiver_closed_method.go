package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A closed instance call is not necessarily read-only. It can modify storage
// distinct from the captures being moved, provided its entire original body
// satisfies the constructor's receiver-effect invariant. Prove closed dispatch
// to an exact own declaration, then analyze it with initialized THIS.
// Open dispatch, monitors, native bodies, handlers and recursive call cycles
// require separate proofs. No member name or library identity grants admission.
func (c *ClassObjectDumper) constructorReceiverClosedMethod(obj *ClassObject, member *values.JavaClassMember, opcode int, writes, active map[string]bool, remaining *int, depth int, aliases *constructorSelfStorageProof, arguments ...constructorEffectValue) (constructorEffectValue, bool) {
	if c == nil || !c.constructorReceiverFinalizerSilent || obj == nil || member == nil || aliases == nil || active == nil || remaining == nil || depth > 16 || *remaining <= 0 || obj.AccessFlags&0x0200 != 0 || member.Name != obj.GetClassName() || member.Member == "" || member.Member == "<init>" || member.Member == "<clinit>" || opcode != core.OP_INVOKEVIRTUAL && opcode != core.OP_INVOKESPECIAL {
		return constructorEffectValue{}, false
	}
	params, result, err := callbinding.Descriptor(member.Description)
	if err != nil || len(params) != len(arguments) || nativeMemberParameterWidth(params) > 254 || !nativeProofWork(c.Work, int64(len(params))) {
		return constructorEffectValue{}, false
	}
	for i, parameter := range params {
		if arguments[i].kind != constructorEffectType(parameter).kind || arguments[i].receiver || arguments[i].allocation != 0 {
			return constructorEffectValue{}, false
		}
	}
	key := member.Name + "\x00" + member.Member + "\x00" + member.Description
	if active[key] {
		return constructorEffectValue{}, false
	}
	active[key] = true
	defer delete(active, key)
	var target *MemberInfo
	for _, method := range obj.Methods {
		*remaining--
		if *remaining < 0 || method == nil || !nativeProofWork(c.Work, 1) {
			return constructorEffectValue{}, false
		}
		name, nameErr := obj.getUtf8(method.NameIndex)
		desc, descErr := obj.getUtf8(method.DescriptorIndex)
		if nameErr != nil || descErr != nil || name == "" {
			return constructorEffectValue{}, false
		}
		if name == member.Member && desc == member.Description {
			if target != nil {
				return constructorEffectValue{}, false
			}
			target = method
		}
	}
	if target == nil || target.AccessFlags&(0x0008|0x0020|0x0100|0x0400) != 0 || !c.constructorReceiverMethodDispatchClosed(obj, target, member, opcode, remaining) {
		return constructorEffectValue{}, false
	}
	var code *CodeAttribute
	for _, attribute := range target.Attributes {
		if candidate, ok := attribute.(*CodeAttribute); ok {
			if candidate == nil || code != nil {
				return constructorEffectValue{}, false
			}
			code = candidate
		}
	}
	if code == nil || len(code.ExceptionTable) != 0 || int(code.MaxLocals) < nativeMemberParameterWidth(params)+1 || len(code.Code) > *remaining || !nativeProofWork(c.Work, int64(len(code.Code))) || c.Work != nil && c.Work.CheckAlloc((int64(code.MaxLocals)+int64(code.MaxStack))*32) != nil {
		return constructorEffectValue{}, false
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return constructorEffectValue{}, false
	}
	ops := constructorMotionOps(decoder)
	if len(ops) > *remaining || !c.constructorReceiverBodyEffectsWithStorage(obj, code, ops, member.Description, writes, active, remaining, depth, aliases, true, arguments...) || !aliases.closed() {
		return constructorEffectValue{}, false
	}
	return constructorEffectType(result), true
}
