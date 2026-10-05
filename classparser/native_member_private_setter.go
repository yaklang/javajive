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

func nativeMemberPlainSetterProof(obj *ClassObject, m *MemberInfo, work *workbudget.Budget) *nativeMemberPrivateGetter {
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
	args, result, e := callbinding.Descriptor(desc)
	if e != nil || len(args) != 2 || args[0] != "L"+obj.GetClassName()+";" || args[1] != result || result == "V" {
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
	var code *CodeAttribute
	for _, a := range m.Attributes {
		c, ok := a.(*CodeAttribute)
		if !ok || code != nil {
			return nil
		}
		code = c
	}
	if code == nil || int(code.MaxLocals) != 1+width || int(code.MaxStack) != 1+2*width || len(code.ExceptionTable) != 0 || len(code.Code) != 7 || !nativeProofWork(work, 7) {
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
			if a == nil || seenLocals || a.Name != "LocalVariableTable" || len(a.Info) != 22 || !nativeProofWork(work, 22) {
				return nil
			}
			u := func(i int) uint16 { return binary.BigEndian.Uint16(a.Info[i : i+2]) }
			if u(0) != 2 {
				return nil
			}
			seen := map[uint16]bool{}
			for offset := 2; offset < len(a.Info); offset += 10 {
				slot := u(offset + 8)
				n, nok := sourceBridgeUTF8(obj, u(offset+4))
				d, dok := sourceBridgeUTF8(obj, u(offset+6))
				if slot > 1 || seen[slot] || u(offset) != 0 || u(offset+2) != 7 || !nok || !dok || class_context.SafeIdentifier(n) != n || d != args[slot] {
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
	if len(ops) != 5 || ops[0].Instr.OpCode != core.OP_ALOAD_0 || !constructorMotionLoad(ops[1], result) || core.GetRetrieveIdx(ops[1]) != 1 || ops[2].Instr.OpCode != dup || len(ops[2].Data) != 0 || ops[4].Instr.OpCode != ret || len(ops[4].Data) != 0 {
		return nil
	}
	field := constructorMotionMember(obj, ops[3], core.OP_PUTFIELD)
	if field == nil || field.Name != obj.GetClassName() || field.Description != result || class_context.SafeIdentifier(field.Member) != field.Member {
		return nil
	}
	found := false
	for _, f := range obj.Fields {
		if f == nil || !nativeProofWork(work, 1) {
			return nil
		}
		n, nok := sourceBridgeUTF8(obj, f.NameIndex)
		d, dok := sourceBridgeUTF8(obj, f.DescriptorIndex)
		if !nok || !dok {
			return nil
		}
		if n != field.Member || d != result {
			continue
		}
		if found || f.AccessFlags&2 == 0 || f.AccessFlags&(8|16|0x1000) != 0 {
			return nil
		}
		found = true
		for _, a := range f.Attributes {
			if _, ok := a.(*ConstantValueAttribute); ok {
				return nil
			}
		}
	}
	if !found {
		return nil
	}
	return &nativeMemberPrivateGetter{owner: obj.GetClassName(), name: name, descriptor: desc, field: field.Member, fieldDescriptor: result, ordinal: ordinal - 2, method: m, setter: true}
}
