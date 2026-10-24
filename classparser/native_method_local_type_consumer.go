package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A local declaration need not have a direct NEW: its Class literal and an
// anonymous subclass are independently observable uses. Without hidden values,
// placement at the declaring method's entry does not move a capture producer.
// This certificate records only original operands. Archive closure must still
// prove every user and commit any anonymous subclass's complete source plan.
func (c *ClassObjectDumper) nativeMethodLocalTypeConsumerFacts(local *ClassObject, owner *nativeMethodLocalOwner, constructor *nativeMethodLocalConstructor) (map[int]string, bool) {
	if c == nil || c.obj == nil || local == nil || owner == nil || owner.declaration == nil || owner.owner != c.obj.GetClassName() || owner.declaration.AccessFlags&8 == 0 || constructor == nil || constructor.descriptor != "()V" || len(constructor.captures) != 0 || c.foldSiblingResolver == nil {
		return nil, false
	}
	actual, known := originalMethodLocalOwner(local, c.obj, c.Work)
	packet, physical := originalMethodLocalSourceConstructor(local, c.obj, c.Work)
	if !known || *actual != *owner || !physical || !sameOriginalMethodLocalConstructor(packet, constructor, c.Work) {
		return nil, false
	}
	var code *CodeAttribute
	for _, attribute := range owner.declaration.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if body, ok := attribute.(*CodeAttribute); ok {
			if body == nil || code != nil {
				return nil, false
			}
			code = body
		}
	}
	if code == nil {
		return nil, false
	}
	if _, _, verified := c.nativeOriginalMethodSnapshot(owner.declaration, code); !verified {
		return nil, false
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(c.obj.ConstantPool, i) })
	decoder.Work = c.Work
	if decoder.ParseOpcode() != nil {
		return nil, false
	}
	consumers := map[int]string{}
	for _, op := range decoder.Opcodes() {
		if op == nil || op.Instr == nil || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		switch op.Instr.OpCode {
		case core.OP_NEW:
			if len(op.Data) != 2 {
				return nil, false
			}
			name, known := sourceBridgeClassName(c.obj, core.Convert2bytesToInt(op.Data))
			if !known || name == local.GetClassName() {
				// A failed direct-allocation proof cannot become type-only.
				return nil, false
			}
			raw, found := c.foldSiblingResolver(name)
			if !found {
				continue
			}
			child, err := c.parseResolved(raw)
			if err != nil || child.GetClassName() != name {
				return nil, false
			}
			if child.GetSupperClassName() != local.GetClassName() {
				continue
			}
			declaring, method, anonymous := originalAnonymousOwner(child)
			if !anonymous || declaring != owner.owner || method != owner.method+owner.descriptor {
				return nil, false
			}
			consumers[int(op.CurrentOffset)] = name
		case core.OP_LDC, core.OP_LDC_W:
			index := 0
			if len(op.Data) == 1 {
				index = int(op.Data[0])
			} else if len(op.Data) == 2 {
				index = int(core.Convert2bytesToInt(op.Data))
			}
			if index < 1 || index > len(c.obj.ConstantPool) {
				return nil, false
			}
			if constant, ok := c.obj.ConstantPool[index-1].(*ConstantClassInfo); ok {
				if constant == nil {
					return nil, false
				}
				name, known := sourceBridgeUTF8(c.obj, constant.NameIndex)
				if !known {
					return nil, false
				}
				if name == local.GetClassName() {
					consumers[int(op.CurrentOffset)] = name
				}
			}
		}
		if c.Work != nil && c.Work.CheckAlloc(int64(len(consumers))*128) != nil {
			return nil, false
		}
	}
	return consumers, len(consumers) > 0
}

// Anonymous inheritance is a lexical use only when the independently completed
// anonymous constructor regenerates exactly this local's zero-argument packet.
// Membership in the same archive, a dollar spelling or a nominal SUPER name
// alone grants neither constructor access nor a method-local source scope.
func nativeMethodLocalAnonymousSubtype(local *nativeMethodLocalClass, p *nativeMemberFamily, binary string, work *workbudget.Budget) bool {
	if local == nil || local.object == nil || local.owner == nil || local.constructor == nil || len(local.typeConsumers) == 0 || local.constructor.descriptor != "()V" || len(local.constructor.captures) != 0 || p == nil || p.failed || !nativeProofWork(work, 1) {
		return false
	}
	group := p.anonymousUnits[binary]
	if group == nil || group.failed || group.owner != local.owner.owner || p.lexicalObjects[local.owner.owner] == nil {
		return false
	}
	child := group.children[binary]
	if child == nil || child.object == nil || child.object.GetClassName() != binary || child.object.GetSupperClassName() != local.object.GetClassName() || child.superDescriptor != local.constructor.descriptor || child.sourceSuperDescriptor != local.constructor.descriptor || len(child.superParams) != 0 {
		return false
	}
	owner, method, known := originalAnonymousOwner(child.object)
	if !known || owner != local.owner.owner || method != local.owner.method+local.owner.descriptor || child.method != method {
		return false
	}
	reader := NewClassObjectDumper(p.lexicalObjects[owner])
	reader.Work = work
	reader.options.SourceCompiler = local.sourceCompiler
	reader.options.TargetSourceVersion = local.targetSourceVersion
	actual := reader.nativeAnonymousConstructorForCompiler(child.object, owner, method, p.owner, p, group.forest, nil, group.bridges)
	if actual == nil || actual.descriptor != child.descriptor || actual.superDescriptor != child.superDescriptor || actual.superPC != child.superPC || len(actual.superParams) != 0 {
		return false
	}
	for _, name := range local.typeConsumers {
		if !nativeProofWork(work, 1) {
			return false
		}
		if name == binary {
			return true
		}
	}
	return false
}
