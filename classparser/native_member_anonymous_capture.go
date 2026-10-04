package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A jointly regenerated anonymous super-constructor may read its named owner's
// enclosing capture. No other body read, write or invocation is justified by
// that constructor witness; do not authorize all accesses merely by owner name.
func nativeMemberProjectedAnonymousCaptureRead(p *nativeMemberFamily, anonymous *nativeAnonymousClass, owner string, work *workbudget.Budget) bool {
	if p == nil || anonymous == nil || anonymous.object == nil || anonymous.memberSuper == nil {
		return false
	}
	current := p.children[owner]
	group := p.anonymousUnits[anonymous.object.GetClassName()]
	if current == nil || current.static || group == nil || group.owner != owner || group.children[anonymous.object.GetClassName()] != anonymous ||
		anonymous.memberSuper.owner != current.owner || anonymous.memberSuper.static {
		return false
	}
	seen := 0
	for _, method := range anonymous.object.Methods {
		if method == nil || !nativeProofWork(work, 1) {
			return false
		}
		name, nok := sourceBridgeUTF8(anonymous.object, method.NameIndex)
		descriptor, dok := sourceBridgeUTF8(anonymous.object, method.DescriptorIndex)
		if !nok || !dok {
			return false
		}
		for _, attr := range method.Attributes {
			code, ok := attr.(*CodeAttribute)
			if !ok {
				continue
			}
			if code == nil || !nativeProofWork(work, int64(len(code.Code))) {
				return false
			}
			d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(anonymous.object.ConstantPool, i) })
			d.Work = work
			if d.ParseOpcode() != nil {
				return false
			}
			for _, op := range d.Opcodes() {
				for _, opcode := range []int{core.OP_GETFIELD, core.OP_PUTFIELD, core.OP_GETSTATIC, core.OP_PUTSTATIC} {
					field := constructorMotionMember(anonymous.object, op, opcode)
					if field == nil || field.Name != owner || field.Member != current.field {
						continue
					}
					if opcode != core.OP_GETFIELD || name != "<init>" || descriptor != anonymous.descriptor || int(op.CurrentOffset) != anonymous.memberEnclosingReadPC || field.Description != "L"+current.owner+";" {
						return false
					}
					seen++
				}
			}
		}
	}
	return seen == 1
}
