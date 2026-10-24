package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
)

// A constructor is an instance declaration, but an anonymous expression in
// its explicit invocation arguments cannot capture the uninitialized THIS.
// This source-context query does not change class/self flags or grant source
// ownership. Require one original NEW initialized before the original THIS
// delegation and consumed directly, once, as that delegation's reference arg.
func (c *ClassObjectDumper) nativeAnonymousConstructorArgument(child *ClassObject, lexical string) bool {
	if c == nil || c.obj == nil || child == nil || !strings.HasPrefix(lexical, "<init>(") || !nativeProofWork(c.Work, 1) {
		return false
	}
	owner, methodName, known := originalAnonymousOwner(child)
	if !known || owner != c.obj.GetClassName() || methodName != lexical {
		return false
	}
	var method *MemberInfo
	var code *CodeAttribute
	wantedDescriptor := strings.TrimPrefix(lexical, "<init>")
	matches, codes := 0, 0
	for _, m := range c.obj.Methods {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		n, nk := sourceBridgeUTF8(c.obj, m.NameIndex)
		d, dk := sourceBridgeUTF8(c.obj, m.DescriptorIndex)
		if !nk || !dk || !nativeProofWork(c.Work, int64(len(n)+len(d))+1) {
			return false
		}
		if n != "<init>" || d != wantedDescriptor {
			continue
		}
		if m.AccessFlags&(StaticFlag|0x0400|0x0100) != 0 {
			return false
		}
		method = m
		matches++
		for _, a := range m.Attributes {
			if !nativeProofWork(c.Work, 1) {
				return false
			}
			if ca, ok := a.(*CodeAttribute); ok {
				code = ca
				codes++
			}
		}
	}
	if matches != 1 || codes != 1 || code == nil {
		return false
	}
	childDescriptor := ""
	constructors := 0
	for _, m := range child.Methods {
		if m == nil || !nativeProofWork(c.Work, 1) {
			return false
		}
		n, nk := sourceBridgeUTF8(child, m.NameIndex)
		d, dk := sourceBridgeUTF8(child, m.DescriptorIndex)
		if !nk || !dk || !nativeProofWork(c.Work, int64(len(n)+len(d))+1) {
			return false
		}
		if n == "<init>" {
			childDescriptor = d
			constructors++
		}
	}
	if constructors != 1 {
		return false
	}
	ir, frames, known := c.nativeOriginalMethodSnapshot(method, code)
	if !known {
		return false
	}
	newPC := -1
	for _, record := range frames.Instructions {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		op, known := ir.InstrByID(methodir.InstrID(record.PC))
		if known && op.Opcode == core.OP_NEW && op.Class == child.GetClassName() {
			if newPC >= 0 || !record.Before.ThisUninitialized {
				return false
			}
			newPC = int(op.PC)
		}
	}
	if newPC < 0 {
		return false
	}
	childPC, delegatePC, children, delegates := -1, -1, 0, 0
	for _, record := range frames.Instructions {
		if !nativeProofWork(c.Work, int64(len(record.Before.Stack)+1)) {
			return false
		}
		op, known := ir.InstrByID(methodir.InstrID(record.PC))
		if !known || op.Opcode != core.OP_INVOKESPECIAL || op.Member != "<init>" {
			continue
		}
		params, result, err := callbinding.Descriptor(op.Desc)
		index := len(record.Before.Stack) - nativeMemberParameterWidth(params) - 1
		if err != nil || result != "V" || index < 0 || len(record.BeforeOrigins) != len(record.Before.Locals)+len(record.Before.Stack) {
			return false
		}
		receiver := record.Before.Stack[index]
		origin := record.BeforeOrigins[len(record.Before.Locals)+index]
		if receiver.Kind == frametransfer.UninitNew && int(receiver.NewPC) == newPC {
			if !record.Before.ThisUninitialized || op.Class != child.GetClassName() || receiver.Class != op.Class || op.Desc != childDescriptor || origin != (ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: receiver.NewPC}) {
				return false
			}
			childPC = int(op.PC)
			children++
		}
		if receiver.Kind != frametransfer.UninitThis {
			continue
		}
		if !record.Before.ThisUninitialized || origin != (ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: 0}) || op.Class != c.obj.GetClassName() && op.Class != c.obj.GetSupperClassName() {
			return false
		}
		hits := 0
		for slot := index + 1; slot < len(record.Before.Stack); slot++ {
			at := record.BeforeOrigins[len(record.Before.Locals)+slot]
			if at.Kind == ssabuild.OriginInstr && int(at.PC) == newPC {
				if record.Before.Stack[slot].Kind != frametransfer.Ref || record.Before.Stack[slot].Class != child.GetClassName() {
					return false
				}
				hits++
			}
		}
		if hits != 1 {
			return false
		}
		delegatePC = int(op.PC)
		delegates++
	}
	if children != 1 || delegates != 1 || newPC >= childPC || childPC >= delegatePC {
		return false
	}
	for _, h := range ir.Handlers {
		if !nativeProofWork(c.Work, 1) || int(h.StartPC) < delegatePC+3 || int(h.HandlerPC) < delegatePC+3 {
			return false
		}
	}
	return true
}
