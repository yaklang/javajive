package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
	"slices"
)

// A concrete anonymous allocation exposes its own source declarations without
// spelling its suppressed binary name. Require the original owned allocation
// and exact non-generic declaration; unrelated/inherited lookup stays separate.
func nativeAnonymousDeclaredCall(forest *nativeAnonymousForest, caller *ClassObject, method string, op *core.OpCode, work *workbudget.Budget) bool {
	if forest == nil || caller == nil || forest.objects[caller.GetClassName()] != caller || op == nil || op.Instr == nil || op.Instr.OpCode != core.OP_INVOKEVIRTUAL || len(op.Data) != 2 || !nativeProofWork(work, 1) {
		return false
	}
	index := int(core.Convert2bytesToInt(op.Data))
	if index <= 0 || index > len(caller.ConstantPool) {
		return false
	}
	if ref, ok := caller.ConstantPool[index-1].(*ConstantMethodrefInfo); !ok || ref == nil {
		return false
	}
	symbol := constructorMotionMember(caller, op, core.OP_INVOKEVIRTUAL)
	if symbol == nil || symbol.Member == "<init>" || symbol.Member == "<clinit>" {
		return false
	}
	child := forest.units[symbol.Name]
	group := forest.groups[caller.GetClassName()]
	if child == nil || child.object == nil || forest.objects[symbol.Name] != child.object || group == nil || group.failed || group.forest != forest || group.children[symbol.Name] != child || child.method != method || int(op.CurrentOffset) <= child.invokePC {
		return false
	}
	params, _, err := callbinding.Descriptor(symbol.Description)
	if err != nil || work != nil && work.CheckAlloc(int64(len(params))*64+128) != nil {
		return false
	}
	var target *MemberInfo
	for _, decl := range child.object.Methods {
		if decl == nil || !nativeProofWork(work, 1) {
			return false
		}
		name, known := sourceBridgeUTF8(child.object, decl.NameIndex)
		if !known {
			return false
		}
		if name != symbol.Member {
			continue
		}
		desc, known := sourceBridgeUTF8(child.object, decl.DescriptorIndex)
		if !known {
			return false
		}
		args, _, err := callbinding.Descriptor(desc)
		if err != nil {
			return false
		}
		if !slices.Equal(params, args) {
			continue
		}
		if target != nil || desc != symbol.Description || decl.AccessFlags&(0x0002|0x0008|0x0040|0x0100|0x0400|0x1000) != 0 {
			return false
		}
		for _, a := range decl.Attributes {
			if _, generic := a.(*SignatureAttribute); generic {
				return false
			}
		}
		target = decl
	}
	return target != nil
}
