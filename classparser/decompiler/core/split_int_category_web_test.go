package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func makeWitnessedBoolField(name string, pc int) *values.RefMember {
	boolType := types.NewJavaPrimer(types.JavaBoolean)
	recv := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner"))
	member := values.NewJavaClassMember("Owner", name, "Z", boolType)
	f := values.NewRefMember(recv, name, boolType)
	f.OriginPC = pc
	f.HasOriginPC = true
	f.MarkOriginalFieldRead(member, pc)
	return f
}

func TestSplitIntCategoryWebBooleanUnification(t *testing.T) {
	t.Run("closed_field_and_depth4_iload_chain", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		boolType := types.NewJavaPrimer(types.JavaBoolean)

		// Build a 3-intermediate non-entry chain ending at store2 (total depth 4):
		// c0 (iconst_1) -> w100 -> iload -> w101 -> iload -> w102 -> iload -> store2 in target web 1
		var chainOps []*OpCode
		chainRefs := make([]*values.JavaRef, 3)
		chainLoads := make([]*OpCode, 3)
		chainSlots := make([]*values.SlotValue, 3)
		webs := &slotWeb{webOf: map[*OpCode]int{}, entryWeb: map[int]int{}}
		idToRef := map[*OpCode][][2]any{}

		for i := 0; i < 3; i++ {
			st := op(OP_ISTORE, uint16(10+i*2))
			ld := op(OP_ILOAD, uint16(11+i*2))
			r := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
			r.Id.SetName("tmp")
			sl := values.NewSlotValue(r, intType)
			ld.stackProduced = []values.JavaValue{sl}
			chainRefs[i] = r
			chainLoads[i] = ld
			chainSlots[i] = sl
			webs.webOf[st] = 100 + i
			webs.webOf[ld] = 100 + i
			idToRef[st] = [][2]any{{r, true}}
			if i == 0 {
				st.stackConsumed = []values.JavaValue{values.NewJavaLiteral(1, intType)}
			} else {
				st.stackConsumed = []values.JavaValue{chainSlots[i-1]}
			}
			chainOps = append(chainOps, st, ld)
		}

		// Target split web 1 on slot 0x1b: var18 (boolean, RefMember Z), var15 (int, chainSlots[2]), var17 (boolean)
		ref1 := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		ref1.Id.SetName("var18")
		ref2 := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		ref2.Id.SetName("var15")
		ref3 := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		ref3.Id.SetName("var17")

		s1 := op(OP_ISTORE, 1)
		s1.Data = []byte{0x1b}
		s1.stackConsumed = []values.JavaValue{makeWitnessedBoolField("z", 1)}

		s2 := op(OP_ISTORE, 2)
		s2.Data = []byte{0x1b}
		s2.stackConsumed = []values.JavaValue{chainSlots[2]}

		l1 := op(OP_ILOAD, 3)
		l1.Data = []byte{0x1b}
		outSlot := values.NewSlotValue(ref3, boolType)
		l1.stackProduced = []values.JavaValue{outSlot}

		ret := op(OP_IRETURN, 4)
		ret.stackConsumed = []values.JavaValue{outSlot}

		webs.webOf[s1] = 1
		webs.webOf[s2] = 1
		webs.webOf[l1] = 1
		idToRef[s1] = [][2]any{{ref1, true}}
		idToRef[s2] = [][2]any{{ref2, false}}

		fnType := &types.JavaFuncType{ReturnType: boolType}
		d := &Decompiler{
			FunctionType:    fnType,
			FunctionContext: &class_context.ClassContext{FunctionType: fnType},
			cachedSlotWebs:  webs,
			opCodes:         append(chainOps, s1, s2, l1, ret),
			opcodeIdToRef:   idToRef,
		}
		d.unifySplitIntCategoryWebs()

		got1, ok := d.opcodeIdToRef[s1][0][0].(*values.JavaRef)
		if !ok || got1 != d.opcodeIdToRef[s2][0][0] || outSlot.GetValue() != got1 {
			t.Fatal("expected split web stores and load to unify onto one canonical ref")
		}
		if !isExactPrimer(got1.Type(), types.JavaBoolean) || got1.SolvedWebIdentity == nil {
			t.Fatalf("expected unified boolean canonical ref, got %v", got1.Type().String(&class_context.ClassContext{}))
		}
		// Other webs must NOT be mutated.
		for i, r := range chainRefs {
			if !isExactPrimer(r.Type(), types.JavaInteger) || d.opcodeIdToRef[chainOps[i*2]][0][0] != r {
				t.Fatalf("source chain web %d was mutated", i)
			}
		}
	})

	t.Run("depth_over_4_fails_closed_for_boolean_chain", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		boolType := types.NewJavaPrimer(types.JavaBoolean)
		webs := &slotWeb{webOf: map[*OpCode]int{}, entryWeb: map[int]int{}}
		idToRef := map[*OpCode][][2]any{}
		var ops []*OpCode
		var prevSlot values.JavaValue = values.NewJavaLiteral(1, intType)

		// 5-hop chain into target web exceeds depth 4.
		for i := 0; i < 5; i++ {
			st := op(OP_ISTORE, uint16(20+i*2))
			ld := op(OP_ILOAD, uint16(21+i*2))
			r := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
			r.Id.SetName("c")
			sl := values.NewSlotValue(r, intType)
			ld.stackProduced = []values.JavaValue{sl}
			st.stackConsumed = []values.JavaValue{prevSlot}
			prevSlot = sl
			webs.webOf[st] = 100 + i
			webs.webOf[ld] = 100 + i
			idToRef[st] = [][2]any{{r, true}}
			ops = append(ops, st, ld)
		}

		ref1 := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		ref1.Id.SetName("a")
		ref2 := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		ref2.Id.SetName("b")
		s1 := op(OP_ISTORE_1, 1)
		s1.stackConsumed = []values.JavaValue{makeWitnessedBoolField("z", 1)}
		s2 := op(OP_ISTORE_1, 2)
		s2.stackConsumed = []values.JavaValue{prevSlot}
		l1 := op(OP_ILOAD_1, 3)
		outSlot := values.NewSlotValue(ref2, intType)
		l1.stackProduced = []values.JavaValue{outSlot}

		// Use INVOKE Z so int mode is rejected too, isolating the depth-4 boolean limit.
		inv := op(OP_INVOKESTATIC, 4)
		inv.stackConsumed = []values.JavaValue{outSlot}

		webs.webOf[s1], webs.webOf[s2], webs.webOf[l1] = 1, 1, 1
		idToRef[s1] = [][2]any{{ref1, true}}
		idToRef[s2] = [][2]any{{ref2, false}}

		d := &Decompiler{
			FunctionContext: &class_context.ClassContext{},
			cachedSlotWebs:  webs,
			opCodes:         append(ops, s1, s2, l1, inv),
			opcodeIdToRef:   idToRef,
			invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{
				inv: {FunctionName: "useBool", Descriptor: "(Z)V", FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{boolType}, ReturnType: types.NewJavaPrimer(types.JavaVoid)}},
			},
		}
		d.unifySplitIntCategoryWebs()

		if d.opcodeIdToRef[s1][0][0] != ref1 || d.opcodeIdToRef[s2][0][0] != ref2 {
			t.Fatal("expected depth > 4 chain to fail closed without unifying")
		}
	})

	t.Run("cycle_without_root_rejected", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		boolType := types.NewJavaPrimer(types.JavaBoolean)
		ref1 := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		ref1.Id.SetName("a")
		ref2 := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		ref2.Id.SetName("b")
		otherRef := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		otherRef.Id.SetName("o")

		s1 := op(OP_ISTORE_1, 1)
		s2 := op(OP_ISTORE_1, 2)
		l1 := op(OP_ILOAD_1, 3)
		outSlot := values.NewSlotValue(ref2, intType)
		l1.stackProduced = []values.JavaValue{outSlot}

		otherStore := op(OP_ISTORE_2, 4)
		otherLoad := op(OP_ILOAD_2, 5)
		otherSlot := values.NewSlotValue(otherRef, intType)
		otherLoad.stackProduced = []values.JavaValue{otherSlot}

		s1.stackConsumed = []values.JavaValue{makeWitnessedBoolField("z", 1)}
		s2.stackConsumed = []values.JavaValue{otherSlot}
		otherStore.stackConsumed = []values.JavaValue{outSlot}

		inv := op(OP_INVOKESTATIC, 6)
		inv.stackConsumed = []values.JavaValue{outSlot}

		webs := &slotWeb{webOf: map[*OpCode]int{s1: 1, s2: 1, l1: 1, otherStore: 2, otherLoad: 2}}
		d := &Decompiler{
			FunctionContext: &class_context.ClassContext{},
			cachedSlotWebs:  webs,
			opCodes:         []*OpCode{s1, s2, l1, otherStore, otherLoad, inv},
			opcodeIdToRef:   map[*OpCode][][2]any{s1: {{ref1, true}}, s2: {{ref2, false}}, otherStore: {{otherRef, true}}},
			invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{
				inv: {FunctionName: "useBool", Descriptor: "(Z)V", FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{boolType}, ReturnType: types.NewJavaPrimer(types.JavaVoid)}},
			},
		}
		d.unifySplitIntCategoryWebs()

		if d.opcodeIdToRef[s1][0][0] != ref1 || d.opcodeIdToRef[s2][0][0] != ref2 {
			t.Fatal("expected cyclic load chain to fail closed")
		}
	})

	t.Run("swap_unrelated_operand_isolated", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		boolType := types.NewJavaPrimer(types.JavaBoolean)
		ref1 := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		ref1.Id.SetName("v1")
		ref2 := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		ref2.Id.SetName("v2")

		s1 := op(OP_ISTORE_1, 1)
		s1.stackConsumed = []values.JavaValue{makeWitnessedBoolField("z", 1)}
		s2 := op(OP_ISTORE_1, 2)
		s2.stackConsumed = []values.JavaValue{values.NewJavaLiteral(0, intType)}
		l1 := op(OP_ILOAD_1, 3)
		slotX := values.NewSlotValue(ref2, intType)
		l1.stackProduced = []values.JavaValue{slotX}

		otherY := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		otherY.Id.SetName("y")
		slotY := values.NewSlotValue(otherY, intType)

		swap := op(OP_SWAP, 4)
		swap.stackConsumed = []values.JavaValue{slotX, slotY}
		swapOutX := values.NewStackLifetimeUseView(slotX)
		swapOutY := values.NewStackLifetimeUseView(slotY)
		swap.stackProduced = []values.JavaValue{swapOutY, swapOutX}

		inv := op(OP_INVOKESTATIC, 5)
		inv.stackConsumed = []values.JavaValue{swapOutX}
		iadd := op(OP_IADD, 6)
		iadd.stackConsumed = []values.JavaValue{values.NewJavaLiteral(3, intType), swapOutY}

		webs := &slotWeb{webOf: map[*OpCode]int{s1: 1, s2: 1, l1: 1}}
		d := &Decompiler{
			FunctionContext: &class_context.ClassContext{},
			cachedSlotWebs:  webs,
			opCodes:         []*OpCode{s1, s2, l1, swap, inv, iadd},
			opcodeIdToRef:   map[*OpCode][][2]any{s1: {{ref1, true}}, s2: {{ref2, false}}},
			invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{
				inv: {FunctionName: "useBool", Descriptor: "(Z)V", FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{boolType}, ReturnType: types.NewJavaPrimer(types.JavaVoid)}},
			},
		}
		d.unifySplitIntCategoryWebs()

		got := d.opcodeIdToRef[s1][0][0].(*values.JavaRef)
		if got == ref1 || got != d.opcodeIdToRef[s2][0][0] || !isExactPrimer(got.Type(), types.JavaBoolean) {
			t.Fatal("expected OP_SWAP unrelated operand isolation to allow boolean unification")
		}
	})

	t.Run("cfg_exact_reaching_def_avoids_undirected_cycle", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		boolType := types.NewJavaPrimer(types.JavaBoolean)

		srcRef := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		srcRef.Id.SetName("src")
		store0 := op(OP_ISTORE_2, 1)
		store0.stackConsumed = []values.JavaValue{values.NewJavaLiteral(0, intType)}

		s1 := op(OP_ISTORE_1, 2)
		s1.stackConsumed = []values.JavaValue{makeWitnessedBoolField("z", 2)}
		load2 := op(OP_ILOAD_2, 3)
		srcSlot := values.NewSlotValue(srcRef, boolType)
		load2.stackProduced = []values.JavaValue{srcSlot}

		s2 := op(OP_ISTORE_1, 4)
		s2.stackConsumed = []values.JavaValue{srcSlot}

		ref1 := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		ref1.Id.SetName("var18")
		ref2 := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		ref2.Id.SetName("var17")

		// Subsequent store into web 2 consumes web 1 AFTER load2, creating a false
		// undirected cycle between web 1 and web 2 that exact reaching-defs break.
		postLoad1 := op(OP_ILOAD_1, 5)
		postSlot1 := values.NewSlotValue(ref2, intType)
		postLoad1.stackProduced = []values.JavaValue{postSlot1}
		postStore2 := op(OP_ISTORE_2, 6)
		postStore2.stackConsumed = []values.JavaValue{postSlot1}

		l1 := op(OP_ILOAD_1, 7)
		outSlot := values.NewSlotValue(ref1, boolType)
		l1.stackProduced = []values.JavaValue{outSlot}
		ret := op(OP_IRETURN, 8)
		ret.stackConsumed = []values.JavaValue{outSlot}

		store0.Target, s1.Source = []*OpCode{s1}, []*OpCode{store0}
		s1.Target, load2.Source = []*OpCode{load2}, []*OpCode{s1}
		load2.Target, s2.Source = []*OpCode{s2}, []*OpCode{load2}
		s2.Target, postLoad1.Source = []*OpCode{postLoad1}, []*OpCode{s2}
		postLoad1.Target, postStore2.Source = []*OpCode{postStore2}, []*OpCode{postLoad1}
		postStore2.Target, l1.Source = []*OpCode{l1}, []*OpCode{postStore2}
		l1.Target, ret.Source = []*OpCode{ret}, []*OpCode{l1}

		webs := &slotWeb{webOf: map[*OpCode]int{
			s1: 1, s2: 1, postLoad1: 1, l1: 1,
			store0: 2, load2: 2, postStore2: 2,
		}}
		fnType := &types.JavaFuncType{ReturnType: boolType}
		d := &Decompiler{
			FunctionType:    fnType,
			FunctionContext: &class_context.ClassContext{FunctionType: fnType},
			cachedSlotWebs:  webs,
			opCodes:         []*OpCode{store0, s1, load2, s2, postLoad1, postStore2, l1, ret},
			opcodeIdToRef:   map[*OpCode][][2]any{s1: {{ref1, true}}, s2: {{ref2, false}}, store0: {{srcRef, true}}, postStore2: {{srcRef, false}}},
		}
		d.unifySplitIntCategoryWebs()

		got := d.opcodeIdToRef[s1][0][0].(*values.JavaRef)
		if got == ref1 || got != d.opcodeIdToRef[s2][0][0] || outSlot.GetValue() != got || !isExactPrimer(got.Type(), types.JavaBoolean) {
			t.Fatal("expected exact reaching definitions to break false undirected cycle and unify web 1 to boolean")
		}
	})

	t.Run("incremented_source_slot_rejected_for_boolean", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		boolType := types.NewJavaPrimer(types.JavaBoolean)
		srcRef := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		srcRef.Id.SetName("cnt")
		srcStore := op(OP_ISTORE, 1)
		srcStore.Data = []byte{6}
		srcStore.stackConsumed = []values.JavaValue{values.NewJavaLiteral(0, intType)}
		iincOp := op(OP_IINC, 2)
		iincOp.Data = []byte{6, 1}
		srcLoad := op(OP_ILOAD, 3)
		srcLoad.Data = []byte{6}
		srcSlot := values.NewSlotValue(srcRef, intType)
		srcLoad.stackProduced = []values.JavaValue{srcSlot}

		ref1 := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		ref1.Id.SetName("a")
		ref2 := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		ref2.Id.SetName("b")
		s1 := op(OP_ISTORE_1, 4)
		s1.stackConsumed = []values.JavaValue{makeWitnessedBoolField("z", 4)}
		s2 := op(OP_ISTORE_1, 5)
		s2.stackConsumed = []values.JavaValue{srcSlot}
		l1 := op(OP_ILOAD_1, 6)
		outSlot := values.NewSlotValue(ref2, intType)
		l1.stackProduced = []values.JavaValue{outSlot}
		inv := op(OP_INVOKESTATIC, 7)
		inv.stackConsumed = []values.JavaValue{outSlot}

		srcStore.Target, iincOp.Source = []*OpCode{iincOp}, []*OpCode{srcStore}
		iincOp.Target, srcLoad.Source = []*OpCode{srcLoad}, []*OpCode{iincOp}
		srcLoad.Target, s2.Source = []*OpCode{s2}, []*OpCode{srcLoad}

		webs := &slotWeb{webOf: map[*OpCode]int{srcStore: 2, srcLoad: 2, s1: 1, s2: 1, l1: 1}}
		d := &Decompiler{
			FunctionContext: &class_context.ClassContext{},
			cachedSlotWebs:  webs,
			opCodes:         []*OpCode{srcStore, iincOp, srcLoad, s1, s2, l1, inv},
			opcodeIdToRef:   map[*OpCode][][2]any{srcStore: {{srcRef, true}}, s1: {{ref1, true}}, s2: {{ref2, false}}},
			invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{
				inv: {FunctionName: "useBool", Descriptor: "(Z)V", FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{boolType}, ReturnType: types.NewJavaPrimer(types.JavaVoid)}},
			},
		}
		d.unifySplitIntCategoryWebs()

		if d.opcodeIdToRef[s1][0][0] != ref1 || d.opcodeIdToRef[s2][0][0] != ref2 {
			t.Fatal("expected incremented source slot to prevent boolean unification")
		}
	})

	t.Run("retyped_int01_boolean_literal_copy_unifies", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		boolType := types.NewJavaPrimer(types.JavaBoolean)

		// Local 2 initialized with JavaLiteral{Data: 0, JavaType: boolean} (retyped by earlier pass).
		src0Ref := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		src0Ref.Id.SetName("flag0")
		src0Store := op(OP_ISTORE_2, 1)
		src0Store.stackConsumed = []values.JavaValue{values.NewJavaLiteral(0, boolType)}
		src0Load := op(OP_ILOAD_2, 2)
		src0Slot := values.NewSlotValue(src0Ref, boolType)
		src0Load.stackProduced = []values.JavaValue{src0Slot}

		// Local 3 initialized with int 1.
		src1Ref := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		src1Ref.Id.SetName("flag1")
		src1Store := op(OP_ISTORE_3, 3)
		src1Store.stackConsumed = []values.JavaValue{values.NewJavaLiteral(1, intType)}
		src1Load := op(OP_ILOAD_3, 4)
		src1Slot := values.NewSlotValue(src1Ref, intType)
		src1Load.stackProduced = []values.JavaValue{src1Slot}

		// Split web 1 mixes getfield Z, iload(src1), iload(src0), and writes to destWeb (slot 4)
		// that already carries WebDeclType & SolvedWebIdentity = boolean.
		ref1 := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		ref1.Id.SetName("var18")
		ref2 := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		ref2.Id.SetName("var17")

		s1 := op(OP_ISTORE_1, 5)
		s1.stackConsumed = []values.JavaValue{makeWitnessedBoolField("M", 5)}
		s2 := op(OP_ISTORE_1, 6)
		s2.stackConsumed = []values.JavaValue{src1Slot}
		s3 := op(OP_ISTORE_1, 7)
		s3.stackConsumed = []values.JavaValue{src0Slot}
		l1 := op(OP_ILOAD_1, 8)
		outSlot := values.NewSlotValue(ref2, intType)
		l1.stackProduced = []values.JavaValue{outSlot}

		destRef := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		destRef.Id.SetName("var12_1")
		destRef.WebDeclType = boolType.Copy()
		destRef.SolvedWebIdentity = destRef.Id
		destStore := op(OP_ISTORE, 9)
		destStore.Data = []byte{4}
		destStore.stackConsumed = []values.JavaValue{outSlot}
		destLoad := op(OP_ILOAD, 10)
		destLoad.Data = []byte{4}
		destSlot := values.NewSlotValue(destRef, boolType)
		destLoad.stackProduced = []values.JavaValue{destSlot}
		destIf := op(OP_IFEQ, 11)
		destIf.stackConsumed = []values.JavaValue{destSlot}

		webs := &slotWeb{webOf: map[*OpCode]int{
			src0Store: 2, src0Load: 2,
			src1Store: 3, src1Load: 3,
			s1: 1, s2: 1, s3: 1, l1: 1,
			destStore: 4, destLoad: 4,
		}}
		d := &Decompiler{
			FunctionContext: &class_context.ClassContext{},
			cachedSlotWebs:  webs,
			opCodes:         []*OpCode{src0Store, src0Load, src1Store, src1Load, s1, s2, s3, l1, destStore, destLoad, destIf},
			opcodeIdToRef:   map[*OpCode][][2]any{src0Store: {{src0Ref, true}}, src1Store: {{src1Ref, true}}, s1: {{ref1, true}}, s2: {{ref2, false}}, s3: {{ref2, false}}, destStore: {{destRef, true}}},
		}
		d.unifySplitIntCategoryWebs()

		got := d.opcodeIdToRef[s1][0][0].(*values.JavaRef)
		if got == ref1 || got != d.opcodeIdToRef[s2][0][0] || got != d.opcodeIdToRef[s3][0][0] || outSlot.GetValue() != got {
			t.Fatal("expected split web with retyped int-0/1 boolean literal and SolvedWebIdentity dest to unify")
		}
		if !isExactPrimer(got.Type(), types.JavaBoolean) || got.SolvedWebIdentity == nil {
			t.Fatalf("expected boolean unified ref, got %s", got.Type().String(&class_context.ClassContext{}))
		}
	})

	t.Run("retyped_boolean_literal_non01_rejected", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		boolType := types.NewJavaPrimer(types.JavaBoolean)

		srcRef := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		srcRef.Id.SetName("badLit")
		srcStore := op(OP_ISTORE_2, 1)
		srcStore.stackConsumed = []values.JavaValue{values.NewJavaLiteral(2, boolType)}
		srcLoad := op(OP_ILOAD_2, 2)
		srcSlot := values.NewSlotValue(srcRef, boolType)
		srcLoad.stackProduced = []values.JavaValue{srcSlot}

		ref1 := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		ref1.Id.SetName("a")
		ref2 := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		ref2.Id.SetName("b")
		s1 := op(OP_ISTORE_1, 3)
		s1.stackConsumed = []values.JavaValue{makeWitnessedBoolField("z", 3)}
		s2 := op(OP_ISTORE_1, 4)
		s2.stackConsumed = []values.JavaValue{srcSlot}
		l1 := op(OP_ILOAD_1, 5)
		outSlot := values.NewSlotValue(ref2, intType)
		l1.stackProduced = []values.JavaValue{outSlot}

		inv := op(OP_INVOKESTATIC, 6)
		inv.stackConsumed = []values.JavaValue{outSlot}

		webs := &slotWeb{webOf: map[*OpCode]int{srcStore: 2, srcLoad: 2, s1: 1, s2: 1, l1: 1}}
		d := &Decompiler{
			FunctionContext: &class_context.ClassContext{},
			cachedSlotWebs:  webs,
			opCodes:         []*OpCode{srcStore, srcLoad, s1, s2, l1, inv},
			opcodeIdToRef:   map[*OpCode][][2]any{srcStore: {{srcRef, true}}, s1: {{ref1, true}}, s2: {{ref2, false}}},
			invokeFuncCall: map[*OpCode]*values.FunctionCallExpression{
				inv: {FunctionName: "useBool", Descriptor: "(Z)V", FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{boolType}, ReturnType: types.NewJavaPrimer(types.JavaVoid)}},
			},
		}
		d.unifySplitIntCategoryWebs()

		if d.opcodeIdToRef[s1][0][0] != ref1 || d.opcodeIdToRef[s2][0][0] != ref2 || outSlot.GetValue() != ref2 {
			t.Fatal("expected non-0/1 boolean-typed int literal to fail closed without unifying")
		}
	})

	t.Run("chained_bool_into_int_copy_webs_fixpoint", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		boolType := types.NewJavaPrimer(types.JavaBoolean)

		// w1: split int+bool web whose load is typed boolean and feeds w2.
		r1a := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		r1a.Id.SetName("v1a")
		r1b := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		r1b.Id.SetName("v1b")
		w1s1 := op(OP_ISTORE_1, 1)
		w1s1.stackConsumed = []values.JavaValue{makeWitnessedBoolField("z1", 1)}
		w1s2 := op(OP_ISTORE_1, 2)
		w1s2.stackConsumed = []values.JavaValue{values.NewJavaLiteral(0, intType)}
		w1l := op(OP_ILOAD_1, 3)
		w1Slot := values.NewSlotValue(r1a, boolType)
		w1l.stackProduced = []values.JavaValue{w1Slot}

		// w2: single int ref receiving w1Slot (boolean) + int 1, feeding w3.
		r2 := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		r2.Id.SetName("v2")
		w2s1 := op(OP_ISTORE_2, 4)
		w2s1.stackConsumed = []values.JavaValue{w1Slot}
		w2s2 := op(OP_ISTORE_2, 5)
		w2s2.stackConsumed = []values.JavaValue{values.NewJavaLiteral(1, intType)}
		w2l := op(OP_ILOAD_2, 6)
		w2Slot := values.NewSlotValue(r2, intType)
		w2l.stackProduced = []values.JavaValue{w2Slot}

		// w3: single int ref receiving w2Slot + witnessed bool field, feeding OP_IFEQ.
		r3 := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		r3.Id.SetName("v3")
		w3s1 := op(OP_ISTORE_3, 7)
		w3s1.stackConsumed = []values.JavaValue{w2Slot}
		w3s2 := op(OP_ISTORE_3, 8)
		w3s2.stackConsumed = []values.JavaValue{makeWitnessedBoolField("z3", 8)}
		w3l := op(OP_ILOAD_3, 9)
		w3Slot := values.NewSlotValue(r3, intType)
		w3l.stackProduced = []values.JavaValue{w3Slot}
		br := op(OP_IFEQ, 10)
		br.stackConsumed = []values.JavaValue{w3Slot}

		webs := &slotWeb{webOf: map[*OpCode]int{
			w1s1: 1, w1s2: 1, w1l: 1,
			w2s1: 2, w2s2: 2, w2l: 2,
			w3s1: 3, w3s2: 3, w3l: 3,
		}}
		d := &Decompiler{
			FunctionContext: &class_context.ClassContext{},
			cachedSlotWebs:  webs,
			// Put w1 before w2 before w3 to prove multi-pass fixpoint resolves bottom-up.
			opCodes:       []*OpCode{w1s1, w1s2, w1l, w2s1, w2s2, w2l, w3s1, w3s2, w3l, br},
			opcodeIdToRef: map[*OpCode][][2]any{w1s1: {{r1a, true}}, w1s2: {{r1b, false}}, w2s1: {{r2, true}}, w2s2: {{r2, false}}, w3s1: {{r3, true}}, w3s2: {{r3, false}}},
		}
		d.unifySplitIntCategoryWebs()

		for name, info := range map[string]struct {
			st   *OpCode
			slot *values.SlotValue
			old  *values.JavaRef
		}{
			"w1": {w1s1, w1Slot, r1a},
			"w2": {w2s1, w2Slot, r2},
			"w3": {w3s1, w3Slot, r3},
		} {
			got := d.opcodeIdToRef[info.st][0][0].(*values.JavaRef)
			if got == info.old || info.slot.GetValue() != got || !isExactPrimer(got.Type(), types.JavaBoolean) || got.SolvedWebIdentity == nil {
				t.Fatalf("expected %s to unify to fresh boolean canonical ref, got %v", name, got.Type())
			}
		}
	})

	t.Run("chained_copy_web_with_numeric_sink_rejected", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		boolType := types.NewJavaPrimer(types.JavaBoolean)

		r1a := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		r1a.Id.SetName("v1a")
		r1b := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		r1b.Id.SetName("v1b")
		w1s1 := op(OP_ISTORE_1, 1)
		w1s1.stackConsumed = []values.JavaValue{makeWitnessedBoolField("z", 1)}
		w1s2 := op(OP_ISTORE_1, 2)
		w1s2.stackConsumed = []values.JavaValue{values.NewJavaLiteral(0, intType)}
		w1l := op(OP_ILOAD_1, 3)
		w1Slot := values.NewSlotValue(r1a, boolType)
		w1l.stackProduced = []values.JavaValue{w1Slot}

		r2 := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		r2.Id.SetName("v2")
		w2s := op(OP_ISTORE_2, 4)
		w2s.stackConsumed = []values.JavaValue{w1Slot}
		w2l := op(OP_ILOAD_2, 5)
		w2Slot := values.NewSlotValue(r2, intType)
		w2l.stackProduced = []values.JavaValue{w2Slot}
		iadd := op(OP_IADD, 6)
		iadd.stackConsumed = []values.JavaValue{values.NewJavaLiteral(1, intType), w2Slot}

		webs := &slotWeb{webOf: map[*OpCode]int{w1s1: 1, w1s2: 1, w1l: 1, w2s: 2, w2l: 2}}
		d := &Decompiler{
			FunctionContext: &class_context.ClassContext{},
			cachedSlotWebs:  webs,
			opCodes:         []*OpCode{w1s1, w1s2, w1l, w2s, w2l, iadd},
			opcodeIdToRef:   map[*OpCode][][2]any{w1s1: {{r1a, true}}, w1s2: {{r1b, false}}, w2s: {{r2, true}}},
		}
		d.unifySplitIntCategoryWebs()

		if d.opcodeIdToRef[w2s][0][0] != r2 || !isExactPrimer(r2.Type(), types.JavaInteger) {
			t.Fatal("expected w2 with iadd sink to stay int")
		}
		got1 := d.opcodeIdToRef[w1s1][0][0].(*values.JavaRef)
		if isExactPrimer(got1.Type(), types.JavaBoolean) || !isExactPrimer(got1.Type(), types.JavaInteger) {
			t.Fatalf("expected w1 feeding numeric w2 not to unify to boolean, got %s", got1.Type().String(&class_context.ClassContext{}))
		}
	})

	t.Run("single_ref_bool_into_int_web_isolated_from_numeric_sibling_web", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		sharedRef := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		sharedRef.Id.SetName("var10")

		// Web 1 (same slot 1): numeric consumer via iadd.
		storeA := op(OP_ISTORE_1, 1)
		storeA.stackConsumed = []values.JavaValue{values.NewJavaLiteral(4, intType)}
		loadA := op(OP_ILOAD_1, 2)
		slotA := values.NewSlotValue(sharedRef, intType)
		loadA.stackProduced = []values.JavaValue{slotA}
		iadd := op(OP_IADD, 3)
		iadd.stackConsumed = []values.JavaValue{values.NewJavaLiteral(1, intType), slotA}

		// Web 2 (disjoint web on same slot 1, reusing sharedRef): bool copy + int 0 -> OP_IFEQ.
		storeB1 := op(OP_ISTORE_1, 4)
		storeB1.stackConsumed = []values.JavaValue{makeWitnessedBoolField("flag", 4)}
		storeB2 := op(OP_ISTORE_1, 5)
		storeB2.stackConsumed = []values.JavaValue{values.NewJavaLiteral(0, intType)}
		loadB := op(OP_ILOAD_1, 6)
		slotB := values.NewSlotValue(sharedRef, intType)
		loadB.stackProduced = []values.JavaValue{slotB}
		ifeq := op(OP_IFEQ, 7)
		ifeq.stackConsumed = []values.JavaValue{slotB}

		webs := &slotWeb{webOf: map[*OpCode]int{
			storeA: 1, loadA: 1,
			storeB1: 2, storeB2: 2, loadB: 2,
		}}
		d := &Decompiler{
			FunctionContext: &class_context.ClassContext{},
			cachedSlotWebs:  webs,
			opCodes:         []*OpCode{storeA, loadA, iadd, storeB1, storeB2, loadB, ifeq},
			opcodeIdToRef:   map[*OpCode][][2]any{storeA: {{sharedRef, true}}, storeB1: {{sharedRef, false}}, storeB2: {{sharedRef, false}}},
		}
		d.unifySplitIntCategoryWebs()

		gotB := d.opcodeIdToRef[storeB1][0][0].(*values.JavaRef)
		if gotB == sharedRef || gotB != d.opcodeIdToRef[storeB2][0][0] || slotB.GetValue() != gotB || !isExactPrimer(gotB.Type(), types.JavaBoolean) {
			t.Fatal("expected Web B to be extracted to fresh boolean canonical ref")
		}
		if d.opcodeIdToRef[storeA][0][0] != sharedRef || slotA.GetValue() != sharedRef || !isExactPrimer(sharedRef.Type(), types.JavaInteger) {
			t.Fatal("expected Web A to retain shared int ref and int type")
		}
	})
}

func TestSplitIntCategoryWebIntegerAndFailClosed(t *testing.T) {
	t.Run("mixed_witnessed_z_field_and_int_store_with_numeric_consumer", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		boolType := types.NewJavaPrimer(types.JavaBoolean)

		ref1 := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
		ref1.Id.SetName("var18")
		ref2 := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		ref2.Id.SetName("var15")
		intSrc := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		intSrc.Id.SetName("var13")

		s1 := op(OP_ISTORE_1, 1)
		s1.stackConsumed = []values.JavaValue{makeWitnessedBoolField("z", 1)}
		s2 := op(OP_ISTORE_1, 2)
		s2.stackConsumed = []values.JavaValue{intSrc}
		l1 := op(OP_ILOAD_1, 3)
		outSlot := values.NewSlotValue(ref1, boolType)
		l1.stackProduced = []values.JavaValue{outSlot}

		ret := op(OP_IRETURN, 4)
		ret.stackConsumed = []values.JavaValue{outSlot}

		fnType := &types.JavaFuncType{ReturnType: intType}
		d := &Decompiler{
			FunctionType:    fnType,
			FunctionContext: &class_context.ClassContext{FunctionType: fnType},
			cachedSlotWebs:  &slotWeb{webOf: map[*OpCode]int{s1: 1, s2: 1, l1: 1}},
			opCodes:         []*OpCode{s1, s2, l1, ret},
			opcodeIdToRef:   map[*OpCode][][2]any{s1: {{ref1, true}}, s2: {{ref2, false}}},
		}
		d.unifySplitIntCategoryWebs()

		got := d.opcodeIdToRef[s1][0][0].(*values.JavaRef)
		if got == ref1 || got != d.opcodeIdToRef[s2][0][0] || outSlot.GetValue() != got {
			t.Fatal("expected unified canonical ref across stores and load")
		}
		if !isExactPrimer(got.Type(), types.JavaInteger) || got.SolvedWebIdentity == nil {
			t.Fatalf("expected unified int canonical ref, got %v", got.Type().String(&class_context.ClassContext{}))
		}
		if !isExactPrimer(ref1.Type(), types.JavaBoolean) {
			t.Fatal("old ref type must remain unmutated")
		}
	})

	negatives := []string{
		"bare_boolean_ref_fails_closed",
		"unwitnessed_ref_member_fails_closed",
		"bastore_boolean_array_rejected",
		"invoke_z_param_rejected",
		"entry_web_rejected",
		"iinc_in_web_rejected",
		"iinc_outside_slot_rejected",
		"ternary_escape_rejected",
		"missing_return_type_rejected",
		"kill_switch",
	}
	for _, scenario := range negatives {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "kill_switch" {
				t.Setenv("JDEC_SPLIT_INTCAT_WEB_OFF", "1")
			}
			intType := types.NewJavaPrimer(types.JavaInteger)
			boolType := types.NewJavaPrimer(types.JavaBoolean)
			ref1 := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
			ref1.Id.SetName("r1")
			ref2 := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
			ref2.Id.SetName("r2")

			s1 := op(OP_ISTORE_1, 1)
			s1.stackConsumed = []values.JavaValue{makeWitnessedBoolField("z", 1)}
			s2 := op(OP_ISTORE_1, 2)
			s2.stackConsumed = []values.JavaValue{values.NewJavaLiteral(2, intType)}
			l1 := op(OP_ILOAD_1, 3)
			outSlot := values.NewSlotValue(ref2, intType)
			l1.stackProduced = []values.JavaValue{outSlot}
			cons := op(OP_IRETURN, 4)
			cons.stackConsumed = []values.JavaValue{outSlot}

			webs := &slotWeb{webOf: map[*OpCode]int{s1: 1, s2: 1, l1: 1}, entryWeb: map[int]int{}}
			d := &Decompiler{
				FunctionType:    &types.JavaFuncType{ReturnType: intType},
				FunctionContext: &class_context.ClassContext{},
				cachedSlotWebs:  webs,
				opCodes:         []*OpCode{s1, s2, l1, cons},
				opcodeIdToRef:   map[*OpCode][][2]any{s1: {{ref1, true}}, s2: {{ref2, false}}},
			}

			switch scenario {
			case "bare_boolean_ref_fails_closed":
				bare := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
				bare.Id.SetName("bareBool")
				s1.stackConsumed = []values.JavaValue{values.NewSlotValue(bare, boolType)}
			case "unwitnessed_ref_member_fails_closed":
				recv := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner"))
				s1.stackConsumed = []values.JavaValue{values.NewRefMember(recv, "fakeBool", boolType)}
			case "bastore_boolean_array_rejected":
				cons.Instr.OpCode = OP_BASTORE
				cons.stackConsumed = []values.JavaValue{
					outSlot,
					values.NewJavaLiteral(0, intType),
					values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(boolType)),
				}
			case "invoke_z_param_rejected":
				cons.Instr.OpCode = OP_INVOKESTATIC
				cons.stackConsumed = []values.JavaValue{outSlot}
				d.invokeFuncCall = map[*OpCode]*values.FunctionCallExpression{
					cons: {FunctionName: "takeBool", Descriptor: "(Z)V", FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{boolType}, ReturnType: types.NewJavaPrimer(types.JavaVoid)}},
				}
			case "entry_web_rejected":
				webs.entryWeb[1] = 1
			case "iinc_in_web_rejected":
				iincOp := op(OP_IINC, 5)
				iincOp.Data = []byte{1, 1}
				webs.webOf[iincOp] = 1
				d.opCodes = append(d.opCodes, iincOp)
			case "iinc_outside_slot_rejected":
				iincOp := op(OP_IINC, 5)
				iincOp.Data = []byte{1, 1}
				webs.webOf[iincOp] = 2
				d.opCodes = append(d.opCodes, iincOp)
			case "ternary_escape_rejected":
				cons.stackConsumed = []values.JavaValue{
					&values.TernaryExpression{
						Condition:  values.NewJavaLiteral(true, boolType),
						TrueValue:  outSlot,
						FalseValue: values.NewJavaLiteral(false, boolType),
					},
				}
			case "missing_return_type_rejected":
				d.FunctionType = nil
			}

			d.unifySplitIntCategoryWebs()

			if d.opcodeIdToRef[s1][0][0] != ref1 || d.opcodeIdToRef[s2][0][0] != ref2 || outSlot.GetValue() != ref2 {
				t.Fatalf("expected fail-closed no-op for scenario %s", scenario)
			}
		})
	}
}
