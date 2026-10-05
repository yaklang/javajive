package javaclassparser

import (
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

type nativeMemberPrivateCall struct {
	argumentCount     int
	methodFormalCount int
	static            bool
	inherited         bool
	rawGeneric        bool
}

// A private bridge forwards each physical parameter once, in order, into
// its original owned nonvirtual/static target. No additional field effects,
// conversions or generic binding guesses are part of this packet.
func nativeMemberPrivateCallProof(obj *ClassObject, m *MemberInfo, work *workbudget.Budget) *nativeMemberPrivateGetter {
	return nativeMemberCallPacketProof(obj, m, nil, work)
}

func nativeMemberProtectedCallProof(obj *ClassObject, m *MemberInfo, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) *nativeMemberPrivateGetter {
	packet := nativeMemberCallPacketProof(obj, m, resolve, work)
	if packet == nil || packet.call == nil || !packet.call.inherited {
		return nil
	}
	return packet
}

// The common packet proof retains physical slots, descriptor widths, return
// category and the exact checked-exception contract. Only a complete original
// superclass lookup licenses the separate inherited protected virtual case.
func nativeMemberCallPacketProof(obj *ClassObject, m *MemberInfo, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) *nativeMemberPrivateGetter {
	if obj == nil || m == nil || m.AccessFlags != 0x1008 || obj.AccessFlags&0x0200 != 0 || !nativeAccessorVersion(obj, work) || !nativeProofWork(work, 1) {
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
	if e != nil || len(params) > 128 {
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

	if !nativeAccessorCodeMetadata(obj, code, params, work) {
		return nil
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
	// The original invocation kind distinguishes a receiver slot from static
	// parameters, including the valid zero-argument factory packet.
	static := ops[len(params)].Instr.OpCode == core.OP_INVOKESTATIC
	inherited := ops[len(params)].Instr.OpCode == core.OP_INVOKEVIRTUAL
	if inherited && resolve == nil {
		return nil
	}
	kind, offset := core.OP_INVOKESPECIAL, 1
	if static {
		kind, offset = core.OP_INVOKESTATIC, 0
	} else if len(params) == 0 || params[0] != "L"+obj.GetClassName()+";" {
		return nil
	}
	if inherited {
		kind = core.OP_INVOKEVIRTUAL
	}
	invoke := constructorMotionMember(obj, ops[len(params)], kind)
	if invoke == nil || invoke.Name != obj.GetClassName() || invoke.Member == "<init>" || class_context.SafeIdentifier(invoke.Member) != invoke.Member {
		return nil
	}
	targetParams, targetResult, e := callbinding.Descriptor(invoke.Description)
	if e != nil || targetResult != result || len(targetParams) != len(params)-offset {
		return nil
	}
	for i, param := range targetParams {
		if param != params[i+offset] {
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
	targetOwner, target := nativeMemberCallTarget(obj, invoke.Member, invoke.Description, inherited, resolve, work)
	if target == nil {
		return nil
	}
	if inherited {
		if static || targetOwner == obj || target.AccessFlags&7 != 4 || target.AccessFlags&(0x0008|0x1000|0x0040) != 0 || nativeBinaryPackage(targetOwner.GetClassName()) == nativeBinaryPackage(obj.GetClassName()) {
			return nil
		}
	} else if target.AccessFlags&0x0002 == 0 || target.AccessFlags&(0x0400|0x1000|0x0040|0x0100) != 0 || (target.AccessFlags&0x0008 != 0) != static {
		return nil
	}
	var targetThrows *ExceptionsAttribute
	var targetSignature string
	methodFormalCount := 0
	for _, a := range target.Attributes {
		switch a := a.(type) {
		case *SignatureAttribute:
			if a == nil || targetSignature != "" {
				return nil
			}
			var known bool
			targetSignature, known = sourceBridgeUTF8(targetOwner, a.SignatureIndex)
			if !known || targetSignature == "" {
				return nil
			}
			if inherited {
				if !nativeMemberRawInheritedCall(obj, targetOwner, targetSignature, invoke.Description, target, work) {
					return nil
				}
			} else if !nativeMemberConcretePrivateCall(targetOwner, targetSignature, invoke.Description, target, work) {
				if len(targetSignature) > 4096 || !nativeProofWork(work, int64(len(targetSignature))*130+1) || work != nil && work.CheckAlloc(int64(len(targetSignature))*256) != nil {
					return nil
				}
				erased, signatureThrows, formals, known := types.UnboundedMethodErasure(targetSignature)
				if !known || erased != invoke.Description || !nativeOriginalSignatureThrows(targetOwner, target, signatureThrows, work) {
					return nil
				}
				methodFormalCount = len(formals)
			}
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
			targetName, tk := sourceBridgeClassName(targetOwner, targetThrows.ExceptionIndexTable[i])
			if !nk || !tk || name != targetName || !nativeProofWork(work, 1) {
				return nil
			}
		}
	}
	return &nativeMemberPrivateGetter{owner: obj.GetClassName(), name: name, descriptor: desc, field: invoke.Member, fieldDescriptor: invoke.Description, ordinal: ordinal, method: m, call: &nativeMemberPrivateCall{argumentCount: len(params), methodFormalCount: methodFormalCount, static: static, inherited: inherited, rawGeneric: targetSignature != ""}}
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
	offset := 1
	if getter.call.static {
		call.IsStatic, call.Kind, offset = true, values.InvokeStatic, 0
	}
	for _, arg := range args[offset:] {
		v, ok := arg.(values.JavaValue)
		if !ok || sourceProofNil(v) {
			return "", false
		}
		call.Arguments = append(call.Arguments, v)
	}
	owner := ctx.ShortTypeName(strings.ReplaceAll(getter.owner, "/", "."))
	var receiver string
	if getter.call.rawGeneric && strings.ContainsAny(owner, "<>") {
		return "", false
	}
	if getter.call.inherited && nativeStaticAccessorQualifierShadowed(owner, ctx) {
		return "", false
	}
	if getter.call.static {
		// An unqualified private method needs its own overload/hierarchy proof;
		// a hidden class qualifier cannot silently change method binding.
		if nativeStaticAccessorQualifierShadowed(owner, ctx) {
			return "", false
		}
		receiver = owner
	} else {
		v, ok := args[0].(values.JavaValue)
		if !ok || sourceProofNil(v) {
			return "", false
		}
		receiver = "((" + owner + ")(" + v.String(ctx) + "))"
	}
	var rendered []string
	if getter.call.methodFormalCount > 0 {
		var known bool
		rendered, known = call.DescriptorArgumentStrings(ctx)
		if !known {
			return "", false
		}
	} else {
		rendered = call.ArgumentStrings(ctx)
	}
	receiver, registration := ctx.InvocationReceiverSource(receiver)
	method := getter.field
	if getter.call.methodFormalCount > 0 {
		if getter.call.methodFormalCount > 128 {
			return "", false
		}
		formals := make([]string, getter.call.methodFormalCount)
		for i := range formals {
			formals[i] = ctx.ShortTypeName("java.lang.Object")
		}
		method = "<" + strings.Join(formals, ",") + ">" + method
	}
	return receiver + "." + method + "(" + strings.Join(rendered, ",") + ")" + registration + "/*jdec-owned-getter:" + strconv.Itoa(getter.ordinal) + ":" + getter.owner + ":" + getter.field + "*/", true
}
