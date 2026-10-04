package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A direct anonymous plan licenses only its exact projected SUPER capture
// operand. Body reads additionally require the committed mixed forest's
// complete original THIS/field/PC proof for every access. Neither capability
// licenses foreign receivers, writes or an unproved read of the same field.
func nativeMemberProjectedAnonymousCaptureRead(p *nativeMemberFamily, anonymous *nativeAnonymousClass, owner string, work *workbudget.Budget) bool {
	if p == nil || anonymous == nil || anonymous.object == nil {
		return false
	}
	current := p.children[owner]
	group := p.anonymousUnits[anonymous.object.GetClassName()]
	if current == nil || current.static || group == nil || group.failed || group.children[anonymous.object.GetClassName()] != anonymous {
		return false
	}
	forest := p.anonymousForest
	if forest != nil {
		if group.forest != forest || forest.members != p || forest.groups[group.owner] != group || forest.units[anonymous.object.GetClassName()] != anonymous {
			return false
		}
	} else if anonymous.memberSuper == nil || group.owner != owner || anonymous.memberSuper.owner != current.owner || anonymous.memberSuper.static {
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
					if opcode != core.OP_GETFIELD || field.Description != "L"+current.owner+";" {
						return false
					}
					if forest != nil {
						pc := int(op.CurrentOffset)
						read := forest.reads[anonymous.object.GetClassName()][name+descriptor][pc]
						if read == nil || read.owner != owner || read.field != current.field || read.descriptor != field.Description || !forest.lexicalThis[anonymous.object.GetClassName()][name+descriptor][pc] {
							return false
						}
					} else if name != "<init>" || descriptor != anonymous.descriptor || int(op.CurrentOffset) != anonymous.memberEnclosingReadPC {
						return false
					}

					seen++
				}
			}
		}
	}
	return forest != nil && seen > 0 || forest == nil && seen == 1
}
