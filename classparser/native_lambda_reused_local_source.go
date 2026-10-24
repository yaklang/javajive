package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A source variable may represent several dead physical lifetimes. A captured
// single definition need not be its declaration: the exact retained STORE can
// dominate the original factory snapshot later in the same source sequence.
// Earlier writes cannot reach that snapshot. No intervening source write may
// replace the value, and both the STORE's RHS and dynamic operand stay sealed.
func nativeLambdaReusedLocalSource(operand *nativeEnumSelectorProducer, source *nativeLambdaLocalCaptureSource, index int, work *workbudget.Budget) bool {
	if operand == nil || operand.local == nil || source == nil || len(source.values) > 64 || index < 0 || index >= len(source.values) || !source.hasFactoryPC || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(4096*128) != nil {
		return false
	}
	read := operand.local
	if !read.referenceAssignable || !callbinding.Reference(operand.result) || operand.owner != "" || read.storePC < 0 || len(read.storePCs) != 0 || read.pc != operand.pc || read.opcode != operand.opcode || read.slot != operand.slot || read.descriptor != operand.result {
		return false
	}
	ctx := &source.context
	v, known := nativeMemberEnclosingUnpack(source.values[index], work)
	snapshot, ok := v.(*values.JavaRef)
	if !known || !ok || snapshot == nil {
		return false
	}
	pc, ordinal, sealed := snapshot.OriginalDynamicOperandWitness(snapshot.Val)
	if !sealed || pc != source.factoryPC || ordinal != index {
		return false
	}
	v, known = nativeMemberEnclosingUnpack(snapshot.Val, work)
	ref, ok := v.(*values.JavaRef)
	if !known || !ok || ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil {
		return false
	}
	widening := newConstructorWideningQuery(ctx.InvocationMetadata)
	assignable := func(actual, formal string) bool {
		return callbinding.Reference(actual) && callbinding.Reference(formal) && nativeProofWork(work, int64(len(actual)+len(formal))) && widening.assignable(actual, formal)
	}
	localType, localKnown := values.SourceTypeErasure(ref.Type(), ctx)
	snapshotType, snapshotKnown := values.SourceTypeErasure(snapshot.Type(), ctx)
	if !localKnown || !snapshotKnown || !assignable(localType, snapshotType) || !assignable(snapshotType, operand.result) || !nativeLambdaFactoryRetained(source, work) {
		return false
	}
	checkpoint := func(st statements.Statement) bool {
		a, ok := st.(*statements.AssignStatement)
		if !ok || a == nil || a.ArrayMember != nil || !a.HasOriginPC || a.OriginPC != source.factoryPC || !(a.IsDeclare || a.IsFirst) {
			return false
		}
		left, ok := a.LeftValue.(*values.JavaRef)
		if !ok || left == nil || left.Id == nil || left.Id == ref.Id || !left.OriginalDynamicOperandDeclarationOf(snapshot, a.JavaValue) {
			return false
		}
		seed, known := nativeMemberEnclosingUnpack(a.JavaValue, work)
		return known && seed == ref
	}
	stores, snapshots, pairs, remaining, depth := 0, 0, 0, 4096, 0
	active := map[statements.Statement]bool{}
	var walk func([]statements.Statement, bool) bool
	walk = func(body []statements.Statement, eligible bool) bool {
		if depth >= 64 || len(body) > remaining || !nativeProofWork(work, 1) || work != nil && work.Enter(workbudget.CounterASTDepth) != nil {
			return false
		}
		depth++
		defer func() { depth-- }()
		if work != nil {
			defer work.Leave(workbudget.CounterASTDepth)
		}
		for i, st := range body {
			remaining--
			if remaining < 0 || sourceProofNil(st) || active[st] || !nativeProofWork(work, 1) {
				return false
			}
			active[st] = true
			if custom, ok := st.(*statements.CustomStatement); ok {
				if _, sealed := custom.SourceThrowOperand(); !sealed && !custom.SourceTransferOnly() {
					return false // A diagnostic annotation does not seal an opaque callback.
				}
			}
			if checkpoint(st) {
				snapshots++
				if snapshots > 1 {
					return false
				}
			}
			if a, ok := st.(*statements.AssignStatement); ok {
				storePC, slot, sealed := a.OriginalLocalStore()
				if sealed && storePC == read.storePC && slot == read.slot {
					stores++
					if stores > 1 {
						return false
					}
					left, isRef := a.LeftValue.(*values.JavaRef)
					if !isRef || left == nil || left.Id == nil || left.Id != ref.Id || left != ref && !left.SameOriginalLocalDeclaration(ref) && !ref.SameOriginalLocalWeb(left) && !left.SharesSourceDeclaration(ref) {
						return false
					}
					seed, known := nativeMemberEnclosingUnpack(a.JavaValue, work)
					if !known || sourceProofNil(seed) {
						return false
					}
					if seed != values.JavaNull && !values.IsNullLiteral(seed) {
						descriptor, typed := values.SourceTypeErasure(seed.Type(), ctx)
						if !typed || !assignable(descriptor, localType) || !assignable(descriptor, operand.result) {
							return false
						}
					}
					if eligible {
						for j := i + 1; j < len(body); j++ {
							if !nativeProofWork(work, 1) {
								return false
							}
							if checkpoint(body[j]) {
								for _, between := range body[i+1 : j] {
									if !nativeProofWork(work, 1) {
										return false
									}
									switch between.(type) {
									case *statements.ReturnStatement, *statements.CustomStatement:
										// A direct sealed throw/transfer is abrupt; an
										// unsealed custom leaf has unknown completion.
										return false
									}
								}
								before := remaining
								unwritten := nativeCaptureParameterUnwritten(body[i+1:j], ref, &remaining)
								if !nativeProofWork(work, int64(before-remaining)) || !unwritten {
									return false
								}
								pairs++
							}
						}
					}
				}
			}
			_, children, known := nativeSourceNameChildren(st)
			if !known {
				return false
			}
			// A switch arm can have interior label entries/fallthrough. A
			// sequence-local dominator cannot certify those entry edges.
			_, multiEntry := st.(*statements.SwitchStatement)
			for _, child := range children {
				if !walk(child, eligible && !multiEntry) {
					return false
				}
			}
			delete(active, st)
		}
		return true
	}
	return walk(source.body, true) && stores == 1 && snapshots == 1 && pairs == 1
}
