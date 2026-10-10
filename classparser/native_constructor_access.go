package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"slices"
	"strings"
)

// This is an original-code proof, not the older synthetic/numeric-name
// heuristic used by ordinary flat source rendering. The trailing parameter is
// absent from the original target call; no source or runtime operation uses it.
// A complete lexical family must separately prove its compiler-generated role.
type nativeConstructorAccessBridge struct {
	descriptor string
	target     string
	marker     string
	method     *MemberInfo
}

func (c *ClassObjectDumper) nativeConstructorAccessBridges() map[string]*nativeConstructorAccessBridge {
	if c.options.TargetSourceVersion != 0 && c.options.TargetSourceVersion != 8 {
		return map[string]*nativeConstructorAccessBridge{}
	}
	return c.originalNativeConstructorAccessBridges()
}

// Original descriptor evidence is independent of the compiler used to emit
// source. Callers must separately prove that compiler's regeneration behavior.
func (c *ClassObjectDumper) originalNativeConstructorAccessBridges() map[string]*nativeConstructorAccessBridge {
	result := map[string]*nativeConstructorAccessBridge{}
	obj := c.obj
	if !nativeAccessorVersion(obj, c.Work) {
		return result
	}
	constructors := map[string][]*MemberInfo{}
	if len(obj.Methods) > 65535 {
		return nil
	}
	if c.Work != nil && c.Work.CheckAlloc(int64(len(obj.Methods))*32) != nil {
		return nil
	}
	for _, method := range obj.Methods {
		if !nativeProofWork(c.Work, 1) {
			return nil
		}
		name, _ := obj.getUtf8(method.NameIndex)
		desc, _ := obj.getUtf8(method.DescriptorIndex)
		if name == "<init>" {
			constructors[desc] = append(constructors[desc], method)
		}
	}
	for _, method := range obj.Methods {
		if !nativeProofWork(c.Work, 1) {
			return nil
		}
		name, _ := obj.getUtf8(method.NameIndex)
		desc, _ := obj.getUtf8(method.DescriptorIndex)
		if name != "<init>" || method.AccessFlags != 0x1000 || len(constructors[desc]) != 1 {
			continue
		}
		params, ret, err := callbinding.Descriptor(desc)
		if err != nil || ret != "V" || len(params) < 1 {
			continue
		}
		marker := params[len(params)-1]
		if !strings.HasPrefix(marker, "L") || !strings.HasSuffix(marker, ";") {
			continue
		}
		var code *CodeAttribute
		valid := true
		for _, a := range method.Attributes {
			switch a := a.(type) {
			case *CodeAttribute:
				if code != nil {
					valid = false
				}
				code = a
			case *ExceptionsAttribute:
			default:
				valid = false
			}
		}
		if !valid || code == nil || len(code.ExceptionTable) != 0 || len(code.Code) > 512 || !nativeProofWork(c.Work, int64(len(code.Code))) {
			continue
		}
		decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
		decoder.Work = c.Work
		if decoder.ParseOpcode() != nil {
			continue
		}
		ops := constructorMotionOps(decoder)
		if len(ops) != len(params)+2 || core.GetRetrieveIdx(ops[0]) != 0 || !constructorMotionLoad(ops[0], "Ljava/lang/Object;") {
			continue
		}
		slot := 1
		for i, param := range params[:len(params)-1] {
			if core.GetRetrieveIdx(ops[i+1]) != slot || !constructorMotionLoad(ops[i+1], param) {
				valid = false
				break
			}
			slot += constructorEffectType(param).width()
		}
		if !valid || int(code.MaxLocals) < slot+1 || int(code.MaxStack) < slot {
			continue
		}
		call := constructorMotionMember(obj, ops[len(ops)-2], core.OP_INVOKESPECIAL)
		if call == nil || call.Name != obj.GetClassName() || call.Member != "<init>" || ops[len(ops)-1].Instr.OpCode != core.OP_RETURN || len(ops[len(ops)-1].Data) != 0 {
			continue
		}
		targetParams, targetRet, err := callbinding.Descriptor(call.Description)
		if err != nil || targetRet != "V" || !slices.Equal(targetParams, params[:len(params)-1]) {
			continue
		}
		targets := constructors[call.Description]
		if len(targets) != 1 {
			continue
		}
		target := targets[0]
		if target.AccessFlags != 0x0002 && target.AccessFlags != 0x0082 {
			continue
		}
		targetCodes := 0
		for _, a := range target.Attributes {
			if _, ok := a.(*CodeAttribute); ok {
				targetCodes++
			}
		}
		if targetCodes != 1 {
			continue
		}
		exceptions := func(m *MemberInfo) ([]string, bool) {
			var names []string
			count := 0
			for _, a := range m.Attributes {
				if a, ok := a.(*ExceptionsAttribute); ok {
					count++
					for _, index := range a.ExceptionIndexTable {
						name, ok := sourceBridgeClassName(obj, index)
						if !ok {
							return nil, false
						}
						names = append(names, name)
					}
				}
			}
			return names, count <= 1
		}
		a, aok := exceptions(method)
		b, bok := exceptions(target)
		if !aok || !bok || !slices.Equal(a, b) || result[desc] != nil {
			continue
		}
		result[desc] = &nativeConstructorAccessBridge{descriptor: desc, target: call.Description, marker: strings.TrimSuffix(strings.TrimPrefix(marker, "L"), ";"), method: method}
	}
	return result
}

// Only the declaration that is regenerated by this complete family may vanish.
// Every original symbolic use of the descriptor must still have its exact
// owner, and every actual call must be the already proved anonymous super call.
func (p *nativeAnonymousFamily) accessBridgeDescriptor(object *ClassObject, name, descriptor string) bool {
	return p != nil && name == "<init>" && object.GetClassName() == p.owner && p.bridges[descriptor] != nil
}
func (p *nativeAnonymousFamily) accessBridgeNameTypes(object *ClassObject, work *workbudget.Budget) map[int]bool {
	if p == nil {
		return nil
	}
	return nativeConstructorBridgeNameTypes(object, map[string]map[string]*nativeConstructorAccessBridge{p.owner: p.bridges}, work)
}

// NameAndType is shared constant-pool data, not a declaring owner. Certify all
// member edges of a candidate tuple against the same original ownership scope.
// A standalone anonymous family still contributes exactly one owner; a joint
// named transaction contributes its independently proved bridge owners.
func nativeConstructorBridgeNameTypes(object *ClassObject, owners map[string]map[string]*nativeConstructorAccessBridge, work *workbudget.Budget) map[int]bool {
	valid := map[int]bool{}
	seen := map[int]bool{}
	references := map[uint16]int{}
	if object == nil || len(object.ConstantPool) > 65535 {
		return nil
	}
	if work != nil && work.CheckAlloc(int64(len(object.ConstantPool))*32) != nil {
		return nil
	}
	descriptors := map[string]bool{}
	for _, bridges := range owners {
		if !nativeProofWork(work, 1) {
			return nil
		}
		for descriptor, bridge := range bridges {
			if !nativeProofWork(work, 1) || bridge == nil || bridge.descriptor != descriptor || work != nil && work.CheckAlloc(int64(len(descriptors)+1)*64) != nil {
				return nil
			}
			descriptors[descriptor] = true
		}
	}
	for i, constant := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return nil
		}
		if nt, ok := constant.(*ConstantNameAndTypeInfo); ok && nt != nil {
			name, nok := sourceBridgeUTF8(object, nt.NameIndex)
			desc, dok := sourceBridgeUTF8(object, nt.DescriptorIndex)
			if nok && dok && name == "<init>" && descriptors[desc] {
				valid[i+1] = true
			}
		}
	}
	for i, constant := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return nil
		}
		if ref := nativeConstantMember(constant); ref != nil {
			index := int(ref.NameAndTypeIndex)
			if _, candidate := valid[index]; !candidate {
				continue
			}
			owner, known := sourceBridgeClassName(object, ref.ClassIndex)
			_, ordinary := constant.(*ConstantMethodrefInfo)
			nt, nknown := object.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
			descriptor, dknown := "", false
			if nknown && nt != nil {
				descriptor, dknown = sourceBridgeUTF8(object, nt.DescriptorIndex)
			}
			if !known || !ordinary || !dknown || owners[owner][descriptor] == nil {
				valid[index] = false
			}
			references[uint16(i+1)] = index
			seen[index] = true
		}
	}
	for _, constant := range object.ConstantPool {
		if !nativeProofWork(work, 1) {
			return nil
		}
		switch x := constant.(type) {
		case *ConstantMethodHandleInfo:
			if x == nil {
				return nil
			}
			if index := references[x.ReferenceIndex]; index != 0 {
				valid[index] = false
			}
		case *ConstantInvokeDynamicInfo:
			if x == nil {
				return nil
			}
			if _, candidate := valid[int(x.NameAndTypeIndex)]; candidate {
				valid[int(x.NameAndTypeIndex)] = false
			}
		case *ConstantDynamicInfo:
			if x == nil {
				return nil
			}
			if _, candidate := valid[int(x.NameAndTypeIndex)]; candidate {
				valid[int(x.NameAndTypeIndex)] = false
			}
		}
	}
	for index := range valid {
		valid[index] = valid[index] && seen[index]
	}
	return valid
}
func (p *nativeAnonymousFamily) accessBridgeNameType(object *ClassObject, index int, work *workbudget.Budget) bool {
	if object == nil || index <= 0 || index > len(object.ConstantPool) {
		return false
	}
	return p.accessBridgeNameTypes(object, work)[index]
}

func (c *ClassObjectDumper) nativeAccessBridgeCalls(p *nativeAnonymousFamily, obj *ClassObject) bool {
	if len(p.bridges) == 0 {
		return true
	}
	child := p.children[obj.GetClassName()]
	for _, m := range obj.Methods {
		name, _ := obj.getUtf8(m.NameIndex)
		desc, _ := obj.getUtf8(m.DescriptorIndex)
		for _, a := range m.Attributes {
			code, ok := a.(*CodeAttribute)
			if !ok {
				continue
			}
			if !nativeProofWork(c.Work, int64(len(code.Code))) {
				return false
			}
			d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			d.Work = c.Work
			if d.ParseOpcode() != nil {
				return false
			}
			for _, op := range d.Opcodes() {
				call := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL)
				if call == nil || call.Name != p.owner || call.Member != "<init>" || p.bridges[call.Description] == nil {
					continue
				}
				if child == nil || name != "<init>" || desc != child.descriptor || int(op.CurrentOffset) != child.superPC || call.Description != child.superDescriptor {
					return false
				}
			}
		}
	}
	return true
}
