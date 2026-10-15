package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Source omits name/ordinal in every enum constructor, including this(...).
// Prove their unchanged passage through an acyclic constructor graph. Payload
// expressions and constructor bodies remain original source; this certificate
// grants no permission to replace an expression or move a receiver effect.
func nativeEnumConstructorHiddenGraph(obj *ClassObject, order []*MemberInfo, constructors map[string]*MemberInfo, work *workbudget.Budget) bool {
	if obj == nil || len(order) == 0 || len(order) > 64 || len(order) != len(constructors) || !nativeProofWork(work, int64(len(order))) || work != nil && work.CheckAlloc(int64(len(order))*192) != nil {
		return false
	}
	edges := make(map[string]string, len(order))
	for _, method := range order {
		if method == nil || !nativeProofWork(work, 1) {
			return false
		}
		desc, known := sourceBridgeUTF8(obj, method.DescriptorIndex)
		if !known || len(desc) > 65535 || !nativeProofWork(work, int64(len(desc))) || work != nil && work.CheckAlloc(int64(len(desc))*32+int64(len(order))*192) != nil {
			return false
		}
		params, result, err := callbinding.Descriptor(desc)
		width := nativeMemberParameterWidth(params)
		if err != nil || result != "V" || len(params) < 2 || params[0] != "Ljava/lang/String;" || params[1] != "I" || width > 254 || constructors[desc] != method || !nativeEnumOriginalFrameCapacity(method, 3, uint16(width+1)) {
			return false
		}
		if _, duplicate := edges[desc]; duplicate {
			return false
		}
		ops, valid := nativeEnumMethodOps(obj, method, work)
		if !valid || len(ops) < 5 || !constructorMotionLoad(ops[0], "L"+obj.GetClassName()+";") || core.GetRetrieveIdx(ops[0]) != 0 || !constructorMotionLoad(ops[1], "Ljava/lang/String;") || core.GetRetrieveIdx(ops[1]) != 1 || !constructorMotionLoad(ops[2], "I") || core.GetRetrieveIdx(ops[2]) != 2 {
			return false
		}
		direct := nativeEnumMemberOperand(obj, ops[3], core.OP_INVOKESPECIAL, "java/lang/Enum", "<init>", "(Ljava/lang/String;I)V")
		start := 3
		if direct {
			start = 4
		}
		for _, op := range ops[start:] {
			if !nativeProofWork(work, 1) {
				return false
			}
			load, store := core.GetRetrieveIdx(op), core.GetStoreIdx(op)
			width := core.LocalAccessOf(op.Instr.OpCode).Width
			if load >= 0 && load <= 2 && load+width > 1 || store >= 0 && store <= 2 && store+width > 1 {
				return false
			}
		}
		if direct {
			edges[desc] = "" // The unchanged Enum(String,int) root.
			continue
		}
		target, valid := nativeEnumThisConstructorTarget(obj, method, ops, constructors, int64(len(order))*192, work)
		if !valid {
			return false
		}
		edges[desc] = target
	}
	// Each node has exactly one initialization edge. Memoized path colors make
	// cycle/closure checking linear, including chains shared by many overloads.
	colors := make(map[string]uint8, len(edges))
	for desc := range edges {
		var path []string
		for desc != "" && colors[desc] != 2 {
			if !nativeProofWork(work, 1) || colors[desc] == 1 {
				return false
			}
			target, known := edges[desc]
			if !known {
				return false
			}
			colors[desc] = 1
			path = append(path, desc)
			desc = target
		}
		for _, name := range path {
			colors[name] = 2
		}
	}
	return true
}

func nativeEnumThisConstructorTarget(obj *ClassObject, method *MemberInfo, ops []*core.OpCode, constructors map[string]*MemberInfo, retained int64, work *workbudget.Budget) (string, bool) {
	var code *CodeAttribute
	for _, attribute := range method.Attributes {
		if !nativeProofWork(work, 1) {
			return "", false
		}
		if body, ok := attribute.(*CodeAttribute); ok {
			if body == nil || code != nil {
				return "", false
			}
			code = body
		}
	}
	if code == nil || len(code.Code) == 0 || len(code.Code) > 65535 {
		return "", false
	}
	allocation := int64(len(code.Code)+1) * int64(int(code.MaxLocals)+int(code.MaxStack)+1) * 64
	if allocation > 64<<20 || work != nil && work.CheckAlloc(allocation+retained) != nil {
		return "", false
	}
	if !nativeEnumConstructorFrameOperands(obj, ops, allocation+retained, work) {
		return "", false
	}
	reader := NewClassObjectDumper(obj)
	reader.Work = work
	ir, frames, known := reader.nativeOriginalMethodSnapshot(method, code)
	if !known {
		return "", false
	}
	var delegate *methodir.Instr
	for _, record := range frames.Instructions {
		if !nativeProofWork(work, 1) {
			return "", false
		}
		invoke, found := ir.InstrByID(methodir.InstrID(record.PC))
		if !found || invoke.Opcode != core.OP_INVOKESPECIAL || invoke.Member != "<init>" {
			continue
		}
		params, result, err := callbinding.Descriptor(invoke.Desc)
		index := len(record.Before.Stack) - nativeMemberParameterWidth(params) - 1
		if err != nil || result != "V" || index < 0 {
			return "", false
		}
		if record.Before.Stack[index].Kind != frametransfer.UninitThis {
			continue // A nested payload allocation is a different JVM identity.
		}
		if delegate != nil || index != 0 || len(params) < 2 || params[0] != "Ljava/lang/String;" || params[1] != "I" || invoke.Class != obj.GetClassName() || constructors[invoke.Desc] == nil || len(record.Uses) != len(params)+1 || !record.Before.ThisUninitialized {
			return "", false
		}
		for i := 0; i < 3; i++ {
			if record.Uses[i] != (ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: i}) {
				return "", false
			}
		}
		after, _, err := frametransfer.Transfer(record.Before, frametransfer.FromIR(invoke))
		if err != nil || after.ThisUninitialized {
			return "", false
		}
		var opcode *core.OpCode
		for _, op := range ops {
			if int(op.CurrentOffset) == int(record.PC) {
				opcode = op
				break
			}
		}
		if !nativeEnumMemberOperand(obj, opcode, core.OP_INVOKESPECIAL, invoke.Class, "<init>", invoke.Desc) {
			return "", false
		}
		delegate = &invoke
	}
	if delegate == nil {
		return "", false
	}
	// A constructor method formal is not an enum class formal. Its bounds and
	// overload inference cannot be erased merely because hidden roles are exact.
	for _, attribute := range constructors[delegate.Desc].Attributes {
		if !nativeProofWork(work, 1) {
			return "", false
		}
		if signature, ok := attribute.(*SignatureAttribute); ok {
			if signature == nil {
				return "", false
			}
			text, known := sourceBridgeUTF8(obj, signature.SignatureIndex)
			if !known || len(types.MethodFormalTypeParamNames(text)) != 0 {
				return "", false
			}
		}
	}
	for _, record := range frames.Instructions {
		if int(record.PC) <= int(ops[2].CurrentOffset) || !record.Before.ThisUninitialized {
			continue
		}
		if int(record.PC) > int(delegate.PC) || !nativeProofWork(work, int64(len(record.Before.Stack)+len(record.Before.Locals)+1)) || len(record.Before.Stack) < 3 || len(record.BeforeOrigins) != len(record.Before.Stack)+len(record.Before.Locals) {
			return "", false
		}
		if record.Before.Stack[0].Kind != frametransfer.UninitThis || record.Before.Stack[1].Kind != frametransfer.Ref || record.Before.Stack[1].Class != "java/lang/String" || record.Before.Stack[2].Kind != frametransfer.Int {
			return "", false
		}
		for i := 0; i < 3; i++ {
			if record.BeforeOrigins[len(record.Before.Locals)+i] != (ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: i}) {
				return "", false
			}
		}
		for _, value := range record.Before.Stack[3:] {
			if value.Kind == frametransfer.UninitThis {
				return "", false
			}
		}
		for _, value := range record.Before.Locals[1:] {
			if value.Kind == frametransfer.UninitThis {
				return "", false
			}
		}
		// A duplicate hidden word cannot become a source payload, even if the
		// first name/ordinal pair remains intact at the delegation call. Check
		// stack and local origins before any derived operation can hide it.
		for i, origin := range record.BeforeOrigins {
			if i < len(record.Before.Locals) && i < 3 || i >= len(record.Before.Locals) && i < len(record.Before.Locals)+3 {
				continue
			}
			if origin.Kind == ssabuild.OriginParam && origin.Slot >= 0 && origin.Slot < 3 {
				return "", false
			}
		}
	}
	return delegate.Desc, true
}
