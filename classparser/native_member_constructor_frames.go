package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A lexical member regenerates its original enclosing capture before SUPER.
// Finding that original boundary is a verifier identity question, not a claim
// about an external producer's purity or source overload. The ordinary source
// boundary and call binding proofs still own its argument expressions.
//
// Use the immutable original frames when the small declaration-aware packet
// cannot resolve an external method. THIS must stay at the bottom of the operand
// stack throughout a bounded forward argument region. No local definition,
// extra receiver alias, field write, handler or escaping control edge can borrow
// this certificate. A NEW inside an argument is distinct from uninitialized THIS.
func nativeMemberFrameDelegation(obj *ClassObject, method *MemberInfo, code *CodeAttribute, ops []*core.OpCode, start int, work *workbudget.Budget, enclosingPaths ...*nativeMemberLexicalRead) (int, *values.JavaClassMember) {
	return nativeMemberFrameDelegationWithMetadata(obj, method, code, ops, start, work, nil, enclosingPaths...)
}

func nativeMemberFrameDelegationWithMetadata(obj *ClassObject, method *MemberInfo, code *CodeAttribute, ops []*core.OpCode, start int, work *workbudget.Budget, metadata callbinding.Provider, enclosingPaths ...*nativeMemberLexicalRead) (int, *values.JavaClassMember) {
	if obj == nil || method == nil || code == nil || len(enclosingPaths) > 1 || len(enclosingPaths) == 1 && start != 3 || start != 0 && start != 3 || start >= len(ops) || ops[start] == nil || ops[start].Instr == nil || core.GetRetrieveIdx(ops[start]) != 0 || !constructorMotionLoad(ops[start], "Ljava/lang/Object;") {
		return 0, nil
	}
	matches, codes := 0, 0
	for _, original := range obj.Methods {
		if !nativeProofWork(work, 1) {
			return 0, nil
		}
		if original == method {
			matches++
		}
	}
	for _, attribute := range method.Attributes {
		if !nativeProofWork(work, 1) {
			return 0, nil
		}
		if candidate, ok := attribute.(*CodeAttribute); ok {
			codes++
			if candidate != code {
				return 0, nil
			}
		}
	}
	if name, known := sourceBridgeUTF8(obj, method.NameIndex); matches != 1 || codes != 1 || !known || name != "<init>" || method.AccessFlags&(StaticFlag|0x0400|0x0100) != 0 {
		return 0, nil
	}
	if start == 3 {
		capture := constructorMotionMember(obj, ops[2], core.OP_PUTFIELD)
		descriptor, known := sourceBridgeUTF8(obj, method.DescriptorIndex)
		params, result, err := callbinding.Descriptor(descriptor)
		if !known || err != nil || result != "V" || len(params) == 0 || capture == nil || capture.Name != obj.GetClassName() || capture.Description != params[0] || core.GetRetrieveIdx(ops[0]) != 0 || !constructorMotionLoad(ops[0], "Ljava/lang/Object;") || core.GetRetrieveIdx(ops[1]) != 1 || !constructorMotionLoad(ops[1], params[0]) {
			return 0, nil
		}
	}
	reader := NewClassObjectDumper(obj)
	reader.Work = work
	ir, frames, known := reader.nativeOriginalMethodSnapshot(method, code)
	if !known {
		return 0, nil
	}
	var delegate *methodir.Instr
	var enclosing ssabuild.Origin
	var words *nativeConstructorFrameWords
	for _, record := range frames.Instructions {
		if !nativeProofWork(work, 1) {
			return 0, nil
		}
		invoke, found := ir.InstrByID(methodir.InstrID(record.PC))
		if !found || invoke.Opcode != core.OP_INVOKESPECIAL || invoke.Member != "<init>" {
			continue
		}
		params, result, err := callbinding.Descriptor(invoke.Desc)
		index := len(record.Before.Stack) - nativeMemberParameterWidth(params) - 1
		if err != nil || result != "V" || index < 0 || len(record.BeforeOrigins) != len(record.Before.Locals)+len(record.Before.Stack) {
			return 0, nil
		}
		if record.Before.Stack[index].Kind != frametransfer.UninitThis {
			continue
		}
		after, _, err := frametransfer.Transfer(record.Before, frametransfer.FromIR(invoke))
		if err != nil || delegate != nil || index != 0 || !record.Before.ThisUninitialized || after.ThisUninitialized || invoke.Class != obj.GetClassName() && invoke.Class != obj.GetSupperClassName() {
			return 0, nil
		}
		delegate = &invoke
		for slot, i := 1, 0; i < len(params); i++ {
			formal := params[i]
			if formal == "Z" || formal == "B" || formal == "C" || formal == "S" {
				if words == nil {
					words = newNativeConstructorFrameWords(obj, method, ir, frames, work)
				}
				origin := record.BeforeOrigins[len(record.Before.Locals)+slot]
				if words == nil || !words.argument(record.Before.Stack[slot], origin, formal) {
					return 0, nil
				}
			}
			if formal == "J" || formal == "D" {
				slot += 2
			} else {
				slot++
			}
		}
		if len(params) > 0 {
			enclosing = record.BeforeOrigins[len(record.Before.Locals)+1]
		}
	}
	if delegate == nil || int(delegate.PC) <= int(ops[start].CurrentOffset) {
		return 0, nil
	}
	// Projection of a nonstatic SUPER enclosing operand is a separate
	// identity claim. Its declaring-class widening/access/lexical scope is
	// still proved by the caller. A nil path means the unchanged slot-1 word;
	// a path means the exact original consecutive capture reads, never a
	// compatible ordinary argument, cast, call result or merged substitute.
	if len(enclosingPaths) == 1 && (delegate.Class != obj.GetSupperClassName() || !nativeMemberFrameEnclosingOrigin(ir, frames, enclosing, enclosingPaths[0], work)) {
		return 0, nil
	}
	// An exception-table domain is relevant even when its current prefix has
	// no throwing opcode. Do not let an omitted exceptional CFG edge bypass
	// the same pre-delegation handler guard used by the source boundary.
	for _, handler := range ir.Handlers {
		if !nativeProofWork(work, 1) || int(handler.StartPC) < int(delegate.PC)+3 {
			return 0, nil
		}
	}
	// A THIS edge does not write a new capture. It must pass the very same
	// physical enclosing parameter to the target constructor, without a cast,
	// producer, phi or a different nominally compatible enclosing object.
	if start == 0 && (delegate.Class != obj.GetClassName() || enclosing != (ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: 1})) {
		return 0, nil
	}
	startPC := int(ops[start].CurrentOffset)
	var widening *constructorWideningQuery
	for _, record := range frames.Instructions {
		pc := int(record.PC)
		if pc <= startPC || pc > int(delegate.PC) {
			continue
		}
		if !nativeProofWork(work, int64(len(record.Before.Stack)+len(record.Before.Locals)+1)) || !record.Before.ThisUninitialized || len(record.Before.Stack) == 0 || record.Before.Stack[0].Kind != frametransfer.UninitThis || len(record.BeforeOrigins) != len(record.Before.Locals)+len(record.Before.Stack) || record.BeforeOrigins[len(record.Before.Locals)] != (ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: 0}) {
			return 0, nil
		}
		for _, operand := range record.Before.Stack[1:] {
			if operand.Kind == frametransfer.UninitThis {
				return 0, nil
			}
		}
		for _, local := range record.Before.Locals[1:] {
			if local.Kind == frametransfer.UninitThis {
				return 0, nil
			}
		}
		original, known := ir.InstrByID(methodir.InstrID(record.PC))
		if !known {
			return 0, nil
		}
		if original.Opcode == core.OP_AASTORE {
			// JVM computational references permit a covariance failure at
			// AASTORE. A source initializer needs a separate assignment proof;
			// inserting a cast would change its exception class and timing.
			if widening == nil {
				widening = newConstructorWideningQuery(func(name string) (callbinding.Class, bool) {
					if metadata == nil || !nativeProofWork(work, 1) {
						return callbinding.Class{}, false
					}
					return metadata(name)
				})
			}
			stack := record.Before.Stack
			if len(stack) < 3 {
				return 0, nil
			}
			array, index, value := stack[len(stack)-3], stack[len(stack)-2], stack[len(stack)-1]
			if !nativeMemberFrameArrayStoreAssignable(array, index, value, widening, work) {
				component, known := nativeMemberFrameArrayStoreComponent(array, index, work)
				if !known || value.Kind != frametransfer.Ref {
					return 0, nil
				}
				if words == nil {
					words = newNativeConstructorFrameWords(obj, method, ir, frames, work)
				}
				// A coarse verifier join may lose a legal common superclass.
				// Prove every reaching original producer; do not pick one arm
				// or reinterpret computational Object as an assignment cast.
				origin := record.BeforeOrigins[len(record.BeforeOrigins)-1]
				if words == nil || !words.reference(origin, component, widening, frames) {
					return 0, nil
				}
			}
		}
	}
	for _, edge := range ir.Edges {
		if !nativeProofWork(work, 1) {
			return 0, nil
		}
		if int(edge.From) < int(delegate.PC) && edge.Kind != core.EdgeFallthrough && (edge.Kind == core.EdgeException || int(edge.To) <= startPC || edge.To <= edge.From || int(edge.To) > int(delegate.PC)) {
			return 0, nil
		}
	}
	for i := start + 1; i < len(ops) && i-start <= 512; i++ {
		op := ops[i]
		if op == nil || op.Instr == nil || !nativeProofWork(work, 1) {
			return 0, nil
		}
		if int(op.CurrentOffset) == int(delegate.PC) {
			member := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL)
			if member == nil || member.Name != delegate.Class || member.Member != delegate.Member || member.Description != delegate.Desc {
				return 0, nil
			}
			return i + 1, member
		}
		if core.GetStoreIdx(op) >= 0 || op.Instr.OpCode == core.OP_IINC || op.Instr.OpCode == core.OP_PUTFIELD || op.Instr.OpCode == core.OP_PUTSTATIC || core.GetRetrieveIdx(op) == 0 && constructorMotionLoad(op, "Ljava/lang/Object;") {
			return 0, nil
		}
	}
	return 0, nil
}

func nativeMemberFrameArrayStoreAssignable(array, index, value frametransfer.Type, widening *constructorWideningQuery, work *workbudget.Budget) bool {
	component, known := nativeMemberFrameArrayStoreComponent(array, index, work)
	if widening == nil || !known || !nativeProofWork(work, int64(len(value.Class)+1)) {
		return false
	}
	if value.Kind == frametransfer.Null {
		return true
	}
	if value.Kind != frametransfer.Ref || value.Class == "" {
		return false
	}
	actual := value.Class
	if actual[0] != '[' {
		actual = "L" + actual + ";"
	}
	params, result, err := callbinding.Descriptor("(" + actual + ")V")
	return err == nil && result == "V" && len(params) == 1 && widening.assignable(actual, component)
}

func nativeMemberFrameArrayStoreComponent(array, index frametransfer.Type, work *workbudget.Budget) (string, bool) {
	if array.Kind != frametransfer.Ref || index.Kind != frametransfer.Int || len(array.Class) < 2 || array.Class[0] != '[' || !nativeProofWork(work, int64(len(array.Class)+1)) {
		return "", false
	}
	component := array.Class[1:]
	params, result, err := callbinding.Descriptor("(" + component + ")V")
	return component, err == nil && result == "V" && len(params) == 1 && callbinding.Reference(component)
}

func nativeMemberFrameEnclosingOrigin(ir *methodir.MethodIR, frames *ssabuild.Function, origin ssabuild.Origin, path *nativeMemberLexicalRead, work *workbudget.Budget) bool {
	parameter := ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: 1}
	if path == nil {
		return origin == parameter && nativeProofWork(work, 1)
	}
	if ir == nil || frames == nil {
		return false
	}
	seen := map[*nativeMemberLexicalRead]bool{}
	for read := path; read != nil; read = read.prior {
		if seen[read] || len(seen) >= 64 || read.pc < 0 || read.pc > 65535 || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(int64(len(seen)+1)*128) != nil || origin != (ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: uint16(read.pc)}) {
			return false
		}
		seen[read] = true
		field, known := ir.InstrByID(methodir.InstrID(read.pc))
		if !known || field.Opcode != core.OP_GETFIELD || field.Class != read.owner || field.Member != read.field || field.Desc != read.descriptor {
			return false
		}
		found := false
		for _, record := range frames.Instructions {
			if !nativeProofWork(work, 1) {
				return false
			}
			if int(record.PC) != read.pc {
				continue
			}
			if found || len(record.Uses) != 1 {
				return false
			}
			found, origin = true, record.Uses[0]
		}
		if !found {
			return false
		}
		if read.prior == nil {
			base, known := ir.InstrByID(methodir.InstrID(read.basePC))
			params, result, err := callbinding.Descriptor(ir.Descriptor)
			return origin == parameter && read.basePC >= 0 && read.basePC < read.pc && known && base.Local == 1 && constructorMotionLoad(&core.OpCode{Instr: core.InstrInfos[base.Opcode]}, "Ljava/lang/Object;") && err == nil && result == "V" && len(params) > 0 && params[0] == "L"+read.parameterOwner+";"
		}
		if read.prior.pc >= read.pc {
			return false
		}
	}
	return false
}
