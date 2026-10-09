package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func nativeAnonymousLambdaCaptureSource(original *nativeEnumSelectorProducer, value values.JavaValue, pc, index int, context *class_context.ClassContext, work *workbudget.Budget) bool {
	if !nativeLambdaLocalCaptureValue(original, value, pc, index, context, work) {
		return false
	}
	v, known := nativeMemberEnclosingUnpack(value, work)
	if snapshot, ok := v.(*values.JavaRef); ok && snapshot != nil {
		position, ordinal, sealed := snapshot.OriginalDynamicOperandWitness(snapshot.Val)
		if !sealed || position != pc || ordinal != index {
			return false
		}
		v, known = nativeMemberEnclosingUnpack(snapshot.Val, work)
	}
	for depth := 0; known && depth < 32; depth++ {
		cast, ok := v.(*values.CastExpression)
		if !ok {
			field, ok := v.(*values.RefMember)
			if !ok || field == nil {
				return false
			}
			receiver, valid := nativeMemberEnclosingUnpack(field.Object, work)
			ref, ok := receiver.(*values.JavaRef)
			if !valid || !ok || ref == nil {
				return false
			}
			slot, sealed := ref.OriginalReceiverSlot()
			return sealed && slot == 0
		}
		v, known = nativeMemberEnclosingUnpack(cast.Value, work)
	}
	return false
}

// The constructor packet has already proved the exact immutable hidden field
// and original parameter assignment. Only its own THIS may supply that read;
// ordinary fields, foreign receivers, conversions and computed values retain
// the conservative refusal. Actual emitted AST operands are checked separately.
func nativeAnonymousLambdaCaptureField(object *ClassObject, captures *nativeAnonymousClass, ops []*core.OpCode, at int, descriptor string, work *workbudget.Budget) *nativeEnumSelectorProducer {
	if captures == nil || captures.object != object || at < 1 || at >= len(ops) || !nativeProofWork(work, 2) {
		return nil
	}
	field := constructorMotionMember(object, ops[at], core.OP_GETFIELD)
	if field == nil || field.Name != object.GetClassName() || field.Description != descriptor || !constructorMotionLoad(ops[at-1], "L"+field.Name+";") || core.GetRetrieveIdx(ops[at-1]) != 0 {
		return nil
	}
	if _, known := captures.fields[field.Member]; !known || !constructorMotionField(object, field, true, work) {
		return nil
	}
	return &nativeEnumSelectorProducer{opcode: core.OP_GETFIELD, pc: int(ops[at].CurrentOffset), owner: field.Name, member: field.Member, descriptor: descriptor, result: descriptor, operands: []*nativeEnumSelectorProducer{{opcode: ops[at-1].Instr.OpCode, pc: int(ops[at-1].CurrentOffset), slot: 0, result: "L" + field.Name + ";"}}}
}

// Regenerating an anonymous body does not license arbitrary handles to its
// unnameable type. The only new handle domain is its own already proved static
// metafactory implementation. Foreign references and ordinary method/field or
// constructor handles keep the existing refusal.
func nativeAnonymousLambdaHandlesClosed(child *nativeAnonymousClass, index *nativeMemberIndex, work *workbudget.Budget) bool {
	if child == nil || child.object == nil || index == nil || !index.valid || !nativeProofWork(work, 1) {
		return false
	}
	owner := child.object.GetClassName()
	if !index.handles[owner] {
		return child.lambdaImplementation == nil
	}
	view := child.lambdaImplementation
	if view == nil || view.object != child.object || len(view.lambdaImplementations) == 0 || len(view.lambdaImplementations) > 64 {
		return false
	}
	targets := index.handleTargets[owner]
	if len(targets) == 0 || len(targets) > 4096 || !nativeProofWork(work, int64(len(targets)+len(view.lambdaImplementations))) || work != nil && work.CheckAlloc(int64(len(view.lambdaImplementations))*128) != nil {
		return false
	}
	methods := map[[2]string]*MemberInfo{}
	for method := range view.lambdaImplementations {
		if method == nil || !nativeProofWork(work, 1) {
			return false
		}
		name, nok := sourceBridgeUTF8(child.object, method.NameIndex)
		descriptor, dok := sourceBridgeUTF8(child.object, method.DescriptorIndex)
		key := [2]string{name, descriptor}
		if !nok || !dok || methods[key] != nil {
			return false
		}
		methods[key] = method
	}
	// A cached planning permission cannot replace the current original
	// flags, code, bootstrap and SAM declaration. Reprove once per distinct
	// target in this handle closure, with an empty local-capture proof cache.
	context := view.lambdaContext
	context.localCaptures = nil
	context.factorySites = nil
	fresh := &nativeMemberClass{object: child.object, lambdaContext: context}
	for _, target := range targets {
		if !nativeProofWork(work, 1) || target.referencer != owner || !target.methodRef || target.kind != 6 {
			return false
		}
		method := methods[[2]string{target.name, target.descriptor}]
		if method == nil || !nativeMemberLambdaImplementation(fresh, method, work) {
			return false
		}
	}
	return true
}

func (z *JarFS) nativeAnonymousLambdaChildArchiveClosed(child *nativeAnonymousClass, index *nativeMemberIndex, work *workbudget.Budget) bool {
	if child == nil {
		return false
	}
	if child.lambdaImplementation == nil {
		return true
	}
	return nativeAnonymousLambdaHandlesClosed(child, index, work) && z.nativeMemberLambdaArchiveClosed(child.lambdaImplementation, index, work)
}
