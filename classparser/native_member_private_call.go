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
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

type nativeMemberPrivateCall struct{ argumentCount int }

// A legacy static access bridge forwards each physical parameter once, in
// order, into one exact private nonvirtual invocation. Checked exceptions are
// copied from that declaration. No fields, conversions or other effects are
// part of this packet; generic bridges require a separate binding proof.
func nativeMemberPrivateCallProof(obj *ClassObject, m *MemberInfo, work *workbudget.Budget) *nativeMemberPrivateGetter {
	if obj == nil || m == nil || obj.MinorVersion != 0 || (obj.MajorVersion != 51 && obj.MajorVersion != 52) || obj.AccessFlags&0x0200 != 0 || m.AccessFlags != 0x1008 || !nativeProofWork(work, 1) {
		return nil
	}
	name, nok := sourceBridgeUTF8(obj, m.NameIndex)
	desc, dok := sourceBridgeUTF8(obj, m.DescriptorIndex)
	suffix, prefix := strings.CutPrefix(name, "access$")
	ordinal, e := strconv.Atoi(suffix)
	if !nok || !dok || !prefix || e != nil || ordinal < 0 || ordinal%100 != 0 || fmt.Sprintf("%03d", ordinal) != suffix {
		return nil
	}
	params, result, e := callbinding.Descriptor(desc)
	if e != nil || len(params) == 0 || params[0] != "L"+obj.GetClassName()+";" || len(params) > 128 {
		return nil
	}
	var code *CodeAttribute
	var throws *ExceptionsAttribute
	for _, a := range m.Attributes {
		switch a := a.(type) {
		case *CodeAttribute:
			if code != nil || a == nil {
				return nil
			}
			code = a
		case *ExceptionsAttribute:
			if throws != nil || a == nil {
				return nil
			}
			throws = a
		default:
			return nil
		}
	}
	words := nativeMemberParameterWidth(params)
	returnWidth := 1
	if result == "V" {
		returnWidth = 0
	} else if result == "J" || result == "D" {
		returnWidth = 2
	}
	if code == nil || len(code.ExceptionTable) != 0 || int(code.MaxLocals) != words || int(code.MaxStack) != max(words, returnWidth) || len(code.Code) > 512 || !nativeProofWork(work, int64(len(code.Code))) {
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
			if a == nil || seenLocals || a.Name != "LocalVariableTable" || len(a.Info) != 2+10*len(params) || !nativeProofWork(work, int64(len(a.Info))) {
				return nil
			}
			u := func(i int) uint16 { return binary.BigEndian.Uint16(a.Info[i : i+2]) }
			if int(u(0)) != len(params) {
				return nil
			}
			slots := make(map[int]int, len(params))
			physicalSlot := 0
			for i, param := range params {
				slots[physicalSlot] = i
				physicalSlot++
				if param == "J" || param == "D" {
					physicalSlot++
				}
			}
			seen := map[int]bool{}
			for offset := 2; offset < len(a.Info); offset += 10 {
				slot := int(u(offset + 8))
				index, known := slots[slot]
				n, nk := sourceBridgeUTF8(obj, u(offset+4))
				ds, dk := sourceBridgeUTF8(obj, u(offset+6))
				if !known || seen[slot] || u(offset) != 0 || int(u(offset+2)) != len(code.Code) || !nk || !dk || class_context.SafeIdentifier(n) != n || ds != params[index] {
					return nil
				}
				seen[slot] = true
			}
			seenLocals = true
		default:
			return nil
		}
	}

	d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	d.Work = work
	if d.ParseOpcode() != nil {
		return nil
	}
	ops := constructorMotionOps(d)
	if len(ops) != len(params)+2 {
		return nil
	}
	slot := 0
	for i, param := range params {
		if !constructorMotionLoad(ops[i], param) || core.GetRetrieveIdx(ops[i]) != slot {
			return nil
		}
		slot++
		if param == "J" || param == "D" {
			slot++
		}
	}
	invoke := constructorMotionMember(obj, ops[len(params)], core.OP_INVOKESPECIAL)
	if invoke == nil || invoke.Name != obj.GetClassName() || invoke.Member == "<init>" || class_context.SafeIdentifier(invoke.Member) != invoke.Member {
		return nil
	}
	targetParams, targetResult, e := callbinding.Descriptor(invoke.Description)
	if e != nil || targetResult != result || len(targetParams) != len(params)-1 {
		return nil
	}
	for i, param := range targetParams {
		if param != params[i+1] {
			return nil
		}
	}
	ret := core.OP_ARETURN
	switch result {
	case "V":
		ret = core.OP_RETURN
	case "J":
		ret = core.OP_LRETURN
	case "D":
		ret = core.OP_DRETURN
	case "F":
		ret = core.OP_FRETURN
	case "Z", "B", "C", "S", "I":
		ret = core.OP_IRETURN
	}
	if ops[len(ops)-1].Instr.OpCode != ret || len(ops[len(ops)-1].Data) != 0 {
		return nil
	}
	var target *MemberInfo
	for _, candidate := range obj.Methods {
		if candidate == nil || !nativeProofWork(work, 1) {
			return nil
		}
		n, nk := sourceBridgeUTF8(obj, candidate.NameIndex)
		ds, dk := sourceBridgeUTF8(obj, candidate.DescriptorIndex)
		if !nk || !dk {
			return nil
		}
		if n == invoke.Member && ds == invoke.Description {
			if target != nil {
				return nil
			}
			target = candidate
		}
	}
	if target == nil || target.AccessFlags&0x0002 == 0 || target.AccessFlags&(0x0008|0x0400|0x1000|0x0040|0x0100) != 0 {
		return nil
	}
	var targetThrows *ExceptionsAttribute
	for _, a := range target.Attributes {
		switch a := a.(type) {
		case *SignatureAttribute:
			return nil
		case *ExceptionsAttribute:
			if a == nil || targetThrows != nil {
				return nil
			}
			targetThrows = a
		}
	}
	if (throws == nil) != (targetThrows == nil) {
		return nil
	}
	if throws != nil {
		if len(throws.ExceptionIndexTable) != len(targetThrows.ExceptionIndexTable) {
			return nil
		}
		for i, index := range throws.ExceptionIndexTable {
			name, nk := sourceBridgeClassName(obj, index)
			targetName, tk := sourceBridgeClassName(obj, targetThrows.ExceptionIndexTable[i])
			if !nk || !tk || name != targetName || !nativeProofWork(work, 1) {
				return nil
			}
		}
	}
	return &nativeMemberPrivateGetter{owner: obj.GetClassName(), name: name, descriptor: desc, field: invoke.Member, fieldDescriptor: invoke.Description, ordinal: ordinal, method: m, call: &nativeMemberPrivateCall{argumentCount: len(params)}}
}

func nativeMemberPrivateCallSource(getter *nativeMemberPrivateGetter, args []any, ctx *class_context.ClassContext) (string, bool) {
	if getter == nil || getter.call == nil || len(args) != getter.call.argumentCount || ctx == nil {
		return "", false
	}
	mt, e := types.ParseMethodDescriptor(getter.fieldDescriptor)
	if e != nil {
		return "", false
	}
	call := &values.FunctionCallExpression{ClassName: getter.owner, FunctionName: getter.field, Descriptor: getter.fieldDescriptor, Kind: values.InvokeVirtual, FuncType: mt.FunctionType()}
	for _, arg := range args[1:] {
		v, ok := arg.(values.JavaValue)
		if !ok || sourceProofNil(v) {
			return "", false
		}
		call.Arguments = append(call.Arguments, v)
	}
	receiver, ok := args[0].(values.JavaValue)
	if !ok || sourceProofNil(receiver) {
		return "", false
	}
	rendered := call.ArgumentStrings(ctx)
	owner := ctx.ShortTypeName(strings.ReplaceAll(getter.owner, "/", "."))
	return "((" + owner + ")(" + receiver.String(ctx) + "))." + getter.field + "(" + strings.Join(rendered, ",") + ")/*jdec-owned-getter:" + strconv.Itoa(getter.ordinal) + ":" + getter.owner + ":" + getter.field + "*/", true
}
