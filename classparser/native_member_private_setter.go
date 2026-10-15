package javaclassparser

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Each original access operation has its own packet proof. A plain write is
// never a read/modify/write operation, even when its descriptor looks similar.
func nativeMemberPrivateAccessProof(obj *ClassObject, m *MemberInfo, work *workbudget.Budget) *nativeMemberPrivateGetter {
	if getter := nativeMemberPrivateGetterProof(obj, m, work); getter != nil {
		return getter
	}
	if call := nativeMemberPrivateCallProof(obj, m, work); call != nil {
		return call
	}
	if setter := nativeMemberPlainSetterProof(obj, m, work); setter != nil {
		return setter
	}
	return nativeMemberPrivateUpdateProof(obj, m, work)
}

// Family eligibility and accessor registration must use the same original
// packet proof. Otherwise a valid inherited access inside a nonstatic member
// is rejected before the resolver-aware registration phase can examine it.
func nativeMemberPrivateAccessProofWithDeclarations(obj *ClassObject, m *MemberInfo, resolve func(string) (*ClassObject, bool), work *workbudget.Budget, lexical ...map[string]*ClassObject) *nativeMemberPrivateGetter {
	if p := nativeMemberPrivateAccessProof(obj, m, work); p != nil {
		return p
	}
	if len(lexical) == 1 && lexical[0] != nil {
		if p := nativeMemberCallPacketProof(obj, m, nil, work, lexical[0]); p != nil && p.call != nil && !p.call.inherited {
			return p
		}
	}
	if resolve == nil {
		return nil
	}
	if p := nativeMemberProtectedCallProof(obj, m, resolve, work); p != nil {
		return p
	}
	if p := nativeMemberSetterPacketProof(obj, m, resolve, work); p != nil && p.inheritedField {
		return p
	}
	return nativeMemberProtectedFieldProof(obj, m, resolve, work)
}

func nativeMemberPlainSetterProof(obj *ClassObject, m *MemberInfo, work *workbudget.Budget) *nativeMemberPrivateGetter {
	return nativeMemberSetterPacketProof(obj, m, nil, work)
}
func nativeMemberSetterPacketProof(obj *ClassObject, m *MemberInfo, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) *nativeMemberPrivateGetter {
	if obj == nil || m == nil || m.AccessFlags != 0x1008 || !nativeAccessorVersion(obj, work) || !nativeProofWork(work, 1) {
		return nil
	}
	name, nok := sourceBridgeUTF8(obj, m.NameIndex)
	desc, dok := sourceBridgeUTF8(obj, m.DescriptorIndex)
	suffix, prefix := strings.CutPrefix(name, "access$")
	ordinal, e := strconv.Atoi(suffix)
	if !nok || !dok || !prefix || e != nil || ordinal < 2 || ordinal%100 != 2 || fmt.Sprintf("%03d", ordinal) != suffix {
		return nil
	}
	if len(desc) > 65535 || !nativeProofWork(work, int64(len(desc)+len(name))+1) || work != nil && work.CheckAlloc(int64(len(desc))*32+128) != nil {
		return nil
	}
	args, result, e := callbinding.Descriptor(desc)
	if e != nil || (len(args) != 1 && len(args) != 2) || result == "V" {
		return nil
	}
	static := len(args) == 1
	valueIndex := 0
	if !static {
		if args[0] != "L"+obj.GetClassName()+";" {
			return nil
		}
		valueIndex = 1
	}
	if args[valueIndex] != result {
		return nil
	}
	width, ret, dup := 1, core.OP_ARETURN, core.OP_DUP_X1
	switch result {
	case "J":
		width, ret, dup = 2, core.OP_LRETURN, core.OP_DUP2_X1
	case "D":
		width, ret, dup = 2, core.OP_DRETURN, core.OP_DUP2_X1
	case "F":
		ret = core.OP_FRETURN
	case "Z", "B", "C", "S", "I":
		ret = core.OP_IRETURN
	}
	if static {
		if width == 2 {
			dup = core.OP_DUP2
		} else {
			dup = core.OP_DUP
		}
	}
	receiverWords := valueIndex
	codeSize := 6 + receiverWords

	var code *CodeAttribute
	for _, a := range m.Attributes {
		c, ok := a.(*CodeAttribute)
		if !ok || c == nil || code != nil {
			return nil
		}
		code = c
	}
	if code == nil || int(code.MaxLocals) != receiverWords+width || int(code.MaxStack) != receiverWords+2*width || len(code.ExceptionTable) != 0 || len(code.Code) != codeSize || !nativeProofWork(work, int64(codeSize)) {
		return nil
	}
	seenLines, seenLocals := false, false
	if len(code.Attributes) > 2 {
		return nil
	}
	for _, attr := range code.Attributes {
		if !nativeProofWork(work, 1) {
			return nil
		}
		switch a := attr.(type) {
		case *LineNumberTableAttribute:
			if a == nil || seenLines || len(a.LineNumberTable) != 1 || a.LineNumberTable[0] == nil || a.LineNumberTable[0].StartPc != 0 {
				return nil
			}
			seenLines = true
		case *UnparsedAttribute:
			if a == nil || seenLocals || a.Name != "LocalVariableTable" || len(a.Info) != 2+10*len(args) || !nativeProofWork(work, int64(len(a.Info))) {
				return nil
			}
			u := func(i int) uint16 { return binary.BigEndian.Uint16(a.Info[i : i+2]) }
			if int(u(0)) != len(args) {
				return nil
			}
			seen := map[uint16]bool{}
			for offset := 2; offset < len(a.Info); offset += 10 {
				slot := u(offset + 8)
				n, nok := sourceBridgeUTF8(obj, u(offset+4))
				d, dok := sourceBridgeUTF8(obj, u(offset+6))
				if int(slot) >= len(args) || seen[slot] || u(offset) != 0 || int(u(offset+2)) != codeSize || !nok || !dok || !class_context.PreservesJava8Identifier(n) || d != args[slot] {
					return nil
				}
				seen[slot] = true
			}
			seenLocals = true
		default:
			return nil
		}
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return nil
	}
	ops := constructorMotionOps(decoder)
	if len(ops) != 4+receiverWords || !static && ops[0].Instr.OpCode != core.OP_ALOAD_0 || !constructorMotionLoad(ops[valueIndex], result) || core.GetRetrieveIdx(ops[valueIndex]) != valueIndex || ops[valueIndex+1].Instr.OpCode != dup || len(ops[valueIndex+1].Data) != 0 || ops[valueIndex+3].Instr.OpCode != ret || len(ops[valueIndex+3].Data) != 0 {
		return nil
	}
	fieldOp := core.OP_PUTFIELD
	if static {
		fieldOp = core.OP_PUTSTATIC
	}
	field := constructorMotionMember(obj, ops[valueIndex+2], fieldOp)
	if field == nil || field.Name != obj.GetClassName() || field.Description != result || !class_context.PreservesJava8Identifier(field.Member) {
		return nil
	}
	found, generic := false, false
	for _, f := range obj.Fields {
		if f == nil || !nativeProofWork(work, 1) {
			return nil
		}
		n, nok := sourceBridgeUTF8(obj, f.NameIndex)
		d, dok := sourceBridgeUTF8(obj, f.DescriptorIndex)
		if !nok || !dok {
			return nil
		}
		if n != field.Member {
			continue
		}
		// Java source hides fields by name; JVM resolution also includes the
		// descriptor. A same-name declaration cannot be silently skipped.
		if d != result || found || f.AccessFlags&7 != 2 || f.AccessFlags&^uint16(2|8|64|128) != 0 || (f.AccessFlags&8 != 0) != static {
			return nil
		}
		found = true
		for _, a := range f.Attributes {
			if sourceProofNil(a) || !nativeProofWork(work, 1) {
				return nil
			}
			if _, ok := a.(*SignatureAttribute); ok {
				generic = true
			}
			if _, ok := a.(*ConstantValueAttribute); ok {
				return nil
			}
		}
	}
	inherited := false
	if !found {
		if resolve == nil {
			return nil
		}
		owner, target := nativeMemberInheritedFieldTarget(obj, field.Member, result, resolve, work)
		if owner == nil || target == nil || owner == obj || target.AccessFlags&7 != 4 || target.AccessFlags&^uint16(4|8|64|128) != 0 || (target.AccessFlags&8 != 0) != static || nativeBinaryPackage(owner.GetClassName()) == nativeBinaryPackage(obj.GetClassName()) {
			return nil
		}
		var known bool
		generic, known = nativeMemberInheritedFieldSourceSignature(obj, owner, target, result, resolve, work)
		if !known {
			return nil
		}
		inherited = true
	}
	return &nativeMemberPrivateGetter{owner: obj.GetClassName(), name: name, descriptor: desc, field: field.Member, fieldDescriptor: result, ordinal: ordinal - 2, method: m, setter: true, staticField: static, genericField: generic, inheritedField: inherited}
}
