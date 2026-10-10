package javaclassparser

import (
	"encoding/binary"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A bound metafactory captures the anonymous receiver without declaring its
// unnameable type in Java source. Its NameAndType belongs only to the original
// dynamic factories: ordinary members, condy and unused dynamic aliases cannot
// borrow that permission. Source consumption of every receiver is a separate
// publication gate.
func nativeAnonymousForestLambdaNameType(forest *nativeAnonymousForest, object *ClassObject, index int, work *workbudget.Budget) bool {
	if forest == nil || object == nil || index < 1 || index > len(object.ConstantPool) || !nativeProofWork(work, 1) {
		return false
	}
	child := forest.units[object.GetClassName()]
	if child == nil || child.object != object || child.lambdaImplementation == nil || child.lambdaImplementation.object != object {
		return false
	}
	view := child.lambdaImplementation
	if len(view.lambdaImplementations) == 0 || len(view.lambdaImplementations) > 64 || work != nil && work.CheckAlloc(4096*32) != nil {
		return false
	}
	context := view.lambdaContext
	context.localCaptures = nil
	context.factorySites = nil
	fresh := &nativeMemberClass{object: object, lambdaContext: context}
	allowed := map[int]bool{}
	for method := range view.lambdaImplementations {
		if method == nil || method.AccessFlags&StaticFlag != 0 {
			continue
		}
		if !nativeMemberLambdaImplementation(fresh, method, work) {
			return false
		}
		name, nk := sourceBridgeUTF8(object, method.NameIndex)
		desc, dk := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !nk || !dk {
			return false
		}
		for _, site := range fresh.lambdaContext.factorySites[name+desc] {
			if site == nil || site.code == nil || site.pc < 0 || site.pc > len(site.code.Code)-5 || len(site.operands) == 0 || !nativeProofWork(work, 1) {
				return false
			}
			code := site.code.Code[site.pc : site.pc+5]
			if code[0] != core.OP_INVOKEDYNAMIC || code[3] != 0 || code[4] != 0 {
				return false
			}
			cp := int(binary.BigEndian.Uint16(code[1:3]))
			if cp < 1 || cp > len(object.ConstantPool) {
				return false
			}
			dynamic, ok := object.ConstantPool[cp-1].(*ConstantInvokeDynamicInfo)
			if !ok || dynamic == nil || int(dynamic.NameAndTypeIndex) != index {
				continue
			}
			nt, ok := object.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
			if !ok || nt == nil {
				return false
			}
			descriptor, known := sourceBridgeUTF8(object, nt.DescriptorIndex)
			if !known || !nativeProofWork(work, int64(len(descriptor))+1) || work != nil && work.CheckAlloc(int64(len(descriptor))*64+128) != nil {
				return false
			}
			parameters, result, err := callbinding.Descriptor(descriptor)
			if err != nil || len(parameters) != len(site.operands) || parameters[0] != "L"+object.GetClassName()+";" || site.operands[0] == nil || site.operands[0].slot != 0 || site.operands[0].result != parameters[0] {
				return false
			}
			for owned := range forest.units {
				if !nativeProofWork(work, int64(len(descriptor))+1) || strings.Contains(result, "L"+owned+";") {
					return false
				}
				for _, parameter := range parameters[1:] {
					if strings.Contains(parameter, "L"+owned+";") {
						return false
					}
				}
			}
			allowed[cp] = true
		}
	}
	if len(allowed) == 0 || !nativeProofWork(work, int64(len(object.ConstantPool))) {
		return false
	}
	for i, constant := range object.ConstantPool {
		if member := nativeConstantMember(constant); member != nil && int(member.NameAndTypeIndex) == index {
			return false
		}
		switch constant := constant.(type) {
		case *ConstantDynamicInfo:
			if constant == nil || int(constant.NameAndTypeIndex) == index {
				return false
			}
		case *ConstantInvokeDynamicInfo:
			if constant == nil || int(constant.NameAndTypeIndex) == index && !allowed[i+1] {
				return false
			}
		}
	}
	return true
}
