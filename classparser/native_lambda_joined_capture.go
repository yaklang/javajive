package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// One source declaration must account for every original branch STORE and
// initialize exactly once on every normal predecessor of the capture. The
// actual dynamic snapshot declaration is the checkpoint; text/Id spelling is
// not a capture witness. Producer expressions remain at their original STOREs.
func nativeLambdaJoinedLocalSource(operand *nativeEnumSelectorProducer, captured values.JavaValue, pc, index int, body []statements.Statement, ctx *class_context.ClassContext, work *workbudget.Budget) bool {
	if operand == nil || operand.local == nil || len(operand.local.storePCs) < 2 || len(operand.local.storePCs) > 64 || !nativeProofWork(work, int64(len(operand.local.storePCs))) {
		return false
	}
	read := operand.local
	if operand.owner != "" || read.storePC != -1 || read.pc != operand.pc || read.opcode != operand.opcode || read.slot != operand.slot || read.descriptor != operand.result {
		return false
	}
	v, known := nativeMemberEnclosingUnpack(captured, work)
	snapshot, ok := v.(*values.JavaRef)
	if !known || !ok || snapshot == nil {
		return false
	}
	location, position, sealed := snapshot.OriginalDynamicOperandWitness(snapshot.Val)
	erasure, typed := values.SourceTypeErasure(snapshot.Type(), ctx)
	if !sealed || location != pc || position != index || !typed || erasure != operand.result {
		return false
	}
	v, known = nativeMemberEnclosingUnpack(snapshot.Val, work)
	ref, ok := v.(*values.JavaRef)
	if !known || !ok || ref == nil || ref.Id == nil || ref.IsThis || ref.IsParam || ref.StackVar != nil || ref.CustomValue != nil {
		return false
	}
	erasure, typed = values.SourceTypeErasure(ref.Type(), ctx)
	if !typed || erasure != operand.result {
		return false
	}
	expected, seen := map[int]bool{}, map[int]bool{}
	for _, storePC := range operand.local.storePCs {
		if storePC < 0 || storePC > 65535 || expected[storePC] {
			return false
		}
		expected[storePC] = true
	}
	checkpoint := func(st statements.Statement) bool {
		a, ok := st.(*statements.AssignStatement)
		if !ok || a == nil || a.ArrayMember != nil || !a.HasOriginPC || a.OriginPC != pc || !(a.IsDeclare || a.IsFirst) {
			return false
		}
		left, ok := a.LeftValue.(*values.JavaRef)
		if !ok || !left.OriginalDynamicOperandDeclarationOf(snapshot, a.JavaValue) {
			return false
		}
		seed, known := nativeMemberEnclosingUnpack(a.JavaValue, work)
		return known && seed == ref
	}
	store := func(a *statements.AssignStatement) bool {
		storePC, slot, sealed := a.OriginalLocalStore()
		if !sealed || a.LeftValue != ref || slot != operand.local.slot || !expected[storePC] || seen[storePC] {
			return false
		}
		seed, known := nativeMemberEnclosingUnpack(a.JavaValue, work)
		if !known {
			return false
		}
		if values.IsNullLiteral(seed) || seed == values.JavaNull {
			if !callbinding.Reference(operand.result) {
				return false
			}
			seen[storePC] = true
			return true
		}
		descriptor, typed := values.SourceTypeErasure(a.JavaValue.Type(), ctx)
		if !typed || descriptor != operand.result {
			return false
		}
		seen[storePC] = true
		return true
	}
	_, closed := nativeCaptureJoinedSource(body, ref, work, nil, checkpoint, store)
	return closed && len(seen) == len(expected)
}
