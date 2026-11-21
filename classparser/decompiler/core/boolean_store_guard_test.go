package core

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestLocalWebNumericConsumerClassification(t *testing.T) {
	scenarios := []struct {
		name string
		want bool
	}{
		{name: "if_icmplt", want: true},
		{name: "iadd", want: true},
		{name: "putfield_int", want: true},
		{name: "ireturn_int", want: true},
		{name: "invoke_int_arg", want: true},
		{name: "bastore_byte_array", want: true},
		{name: "transitive_istore_copy", want: true},
		{name: "slot_iinc", want: true},
		{name: "putfield_bool", want: false},
		{name: "ireturn_bool", want: false},
		{name: "ifeq", want: false},
		{name: "bastore_bool_array", want: false},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			intType := types.NewJavaPrimer(types.JavaInteger)
			boolType := types.NewJavaPrimer(types.JavaBoolean)
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
			ref.Id.SetName("v")

			store := op(OP_ISTORE_1, 1)
			store.stackConsumed = []values.JavaValue{values.NewJavaLiteral(1, intType)}

			loadToPut := op(OP_ILOAD_1, 2)
			loadSlot := values.NewSlotValue(ref, intType)
			loadToPut.stackProduced = []values.JavaValue{loadSlot}
			putBool := op(OP_PUTFIELD, 3)
			putBool.Data = []byte{0, 1}
			putBool.stackConsumed = []values.JavaValue{loadSlot, values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner"))}

			loadUse := op(OP_ILOAD_1, 4)
			useSlot := values.NewSlotValue(ref, intType)
			loadUse.stackProduced = []values.JavaValue{useSlot}
			consumer := op(OP_IRETURN, 5)
			consumer.stackConsumed = []values.JavaValue{useSlot}

			loadToPut.Target = []*OpCode{putBool}
			putBool.Source = []*OpCode{loadToPut}
			loadUse.Target = []*OpCode{consumer}
			consumer.Source = []*OpCode{loadUse}

			cpFields := map[int]values.JavaValue{
				1: values.NewJavaClassMember("Owner", "flag", "Z", boolType),
				2: values.NewJavaClassMember("Owner", "count", "I", intType),
			}
			webs := &slotWeb{
				webOf: map[*OpCode]int{
					store:     1,
					loadToPut: 1,
					loadUse:   1,
				},
			}
			fnType := &types.JavaFuncType{ReturnType: boolType}
			d := &Decompiler{
				FunctionType:    fnType,
				FunctionContext: &class_context.ClassContext{FunctionType: fnType},
				cachedSlotWebs:  webs,
				opCodes:         []*OpCode{store, loadToPut, putBool, loadUse, consumer},
				opcodeIdToRef:   map[*OpCode][][2]any{store: {{ref, true}}},
				constantPoolGetter: func(id int) values.JavaValue {
					return cpFields[id]
				},
			}

			switch sc.name {
			case "if_icmplt":
				consumer.Instr.OpCode = OP_IF_ICMPLT
				consumer.stackConsumed = []values.JavaValue{values.NewJavaLiteral(2, intType), useSlot}
			case "iadd":
				consumer.Instr.OpCode = OP_IADD
				consumer.stackConsumed = []values.JavaValue{values.NewJavaLiteral(1, intType), useSlot}
			case "putfield_int":
				consumer.Instr.OpCode = OP_PUTFIELD
				consumer.Data = []byte{0, 2}
				consumer.stackConsumed = []values.JavaValue{useSlot, values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner"))}
			case "ireturn_int":
				fnType := &types.JavaFuncType{ReturnType: intType}
				d.FunctionType = fnType
				d.FunctionContext.FunctionType = fnType
			case "invoke_int_arg":
				consumer.Instr.OpCode = OP_INVOKESTATIC
				consumer.Data = []byte{0, 3}
				mt, err := types.ParseMethodDescriptor("(I)V")
				if err != nil {
					t.Fatal(err)
				}
				cpFields[3] = values.NewJavaClassMember("Owner", "takeInt", "(I)V", mt)
			case "bastore_byte_array":
				consumer.Instr.OpCode = OP_BASTORE
				consumer.stackConsumed = []values.JavaValue{
					useSlot,
					values.NewJavaLiteral(0, intType),
					values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(types.NewJavaPrimer(types.JavaByte))),
				}
			case "transitive_istore_copy":
				consumer.Instr.OpCode = OP_ISTORE_2
				ref2 := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
				d.opcodeIdToRef[consumer] = [][2]any{{ref2, true}}
				webs.webOf[consumer] = 2

				load2 := op(OP_ILOAD_2, 6)
				slot2 := values.NewSlotValue(ref2, intType)
				load2.stackProduced = []values.JavaValue{slot2}
				webs.webOf[load2] = 2

				addOp := op(OP_IADD, 7)
				addOp.stackConsumed = []values.JavaValue{values.NewJavaLiteral(1, intType), slot2}
				load2.Target = []*OpCode{addOp}
				addOp.Source = []*OpCode{load2}
				d.opCodes = append(d.opCodes, load2, addOp)
			case "slot_iinc":
				consumer.Instr.OpCode = OP_POP
				iincOp := op(OP_IINC, 6)
				iincOp.Data = []byte{1, 1}
				store.Target = append(store.Target, iincOp)
				iincOp.Source = []*OpCode{store}
				webs.webOf[iincOp] = 1
				d.opCodes = append(d.opCodes, iincOp)
			case "putfield_bool":
				consumer.Instr.OpCode = OP_PUTFIELD
				consumer.Data = []byte{0, 1}
				consumer.stackConsumed = []values.JavaValue{useSlot, values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner"))}
			case "ireturn_bool":
				// default OP_IRETURN with boolType ReturnType
			case "ifeq":
				consumer.Instr.OpCode = OP_IFEQ
			case "bastore_bool_array":
				consumer.Instr.OpCode = OP_BASTORE
				consumer.stackConsumed = []values.JavaValue{
					useSlot,
					values.NewJavaLiteral(0, intType),
					values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(boolType)),
				}
			}

			got := d.localWebHasNumericConsumer(putBool, loadSlot)
			if got != sc.want {
				t.Fatalf("localWebHasNumericConsumer(%s) = %v, want %v", sc.name, got, sc.want)
			}
		})
	}

	t.Run("kill_switch", func(t *testing.T) {
		t.Setenv("JDEC_BOOL_STORE_GUARD_OFF", "1")
		intType := types.NewJavaPrimer(types.JavaInteger)
		ref := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		store := op(OP_ISTORE_1, 1)
		load := op(OP_ILOAD_1, 2)
		cmp := op(OP_IF_ICMPLT, 3)
		slot := values.NewSlotValue(ref, intType)
		load.stackProduced = []values.JavaValue{slot}
		cmp.stackConsumed = []values.JavaValue{values.NewJavaLiteral(2, intType), slot}
		put := op(OP_PUTFIELD, 4)
		put.Data = []byte{0, 1}
		put.stackConsumed = []values.JavaValue{slot, values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner"))}
		load.Target = []*OpCode{put}
		put.Source = []*OpCode{load}

		d := &Decompiler{
			cachedSlotWebs: &slotWeb{webOf: map[*OpCode]int{store: 1, load: 1}},
			opCodes:        []*OpCode{store, load, cmp, put},
		}
		if d.localWebHasNumericConsumer(put, slot) {
			t.Fatal("expected guard disabled when JDEC_BOOL_STORE_GUARD_OFF=1")
		}
	})
}

func TestBoolStoreGuardTypeIsolationAndPreserveAssign(t *testing.T) {
	t.Run("new_var_copies_type_wrap_for_java_ref", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		srcRef := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		srcRef.Id.SetName("src")
		srcSlot := values.NewSlotValue(srcRef, srcRef.Type())

		sim := NewStackSimulation(nil, map[int]*values.JavaRef{}, utils.NewRootVariableId())
		dstRef := sim.NewVar(srcSlot)
		dstRef.Type().ResetType(types.NewJavaPrimer(types.JavaBoolean))

		if !isExactPrimer(srcRef.Type(), types.JavaInteger) {
			t.Fatalf("expected source ref type to remain int after dst mutation, got %s", srcRef.Type().String(&class_context.ClassContext{}))
		}
		if !isExactPrimer(dstRef.Type(), types.JavaBoolean) {
			t.Fatalf("expected dst ref type boolean, got %s", dstRef.Type().String(&class_context.ClassContext{}))
		}
	})

	t.Run("preserve_source_type_assignment_does_not_poison_int_rhs", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		boolType := types.NewJavaPrimer(types.JavaBoolean)
		srcRef := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		srcRef.Id.SetName("var5")
		owner := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner"))
		owner.Id.SetName("this")
		field := values.NewRefMember(owner, "flag", boolType)

		stmt := statements.NewAssignStatementPreserveSourceType(field, srcRef, false)
		ctx := &class_context.ClassContext{}
		rendered := stmt.String(ctx)

		if !isExactPrimer(srcRef.Type(), types.JavaInteger) {
			t.Fatalf("expected srcRef to stay int, got %s", srcRef.Type().String(ctx))
		}
		if !strings.Contains(rendered, "flag") || !strings.Contains(rendered, "var5") {
			t.Fatalf("unexpected assignment rendering: %s", rendered)
		}
	})
}

func TestBoolStoreGuardSharedRefAcrossDisjointWebsAndStoreWebCheck(t *testing.T) {
	t.Run("shared_ref_across_disjoint_webs_detected", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		boolType := types.NewJavaPrimer(types.JavaBoolean)
		sharedRef := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		sharedRef.Id.SetName("var1")

		// Web 1: stores SDK_INT into sharedRef and compares with if_icmplt.
		store1 := op(OP_ISTORE_3, 1)
		store1.stackConsumed = []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, intType)}
		load1 := op(OP_ILOAD_3, 2)
		slot1 := values.NewSlotValue(sharedRef, intType)
		load1.stackProduced = []values.JavaValue{slot1}
		cmp := op(OP_IF_ICMPLT, 3)
		cmp.stackConsumed = []values.JavaValue{values.NewJavaLiteral(21, intType), slot1}
		load1.Target, cmp.Source = []*OpCode{cmp}, []*OpCode{load1}

		// Web 2: stores 0/1 into SAME sharedRef and writes putstatic Z.
		store2 := op(OP_ISTORE_3, 4)
		store2.stackConsumed = []values.JavaValue{values.NewJavaLiteral(0, intType)}
		load2 := op(OP_ILOAD_3, 5)
		slot2 := values.NewSlotValue(sharedRef, intType)
		load2.stackProduced = []values.JavaValue{slot2}
		putZ := op(OP_PUTSTATIC, 6)
		putZ.Data = []byte{0, 1}
		putZ.stackConsumed = []values.JavaValue{slot2}
		load2.Target, putZ.Source = []*OpCode{putZ}, []*OpCode{load2}

		webs := &slotWeb{webOf: map[*OpCode]int{
			store1: 1, load1: 1,
			store2: 2, load2: 2,
		}}
		d := &Decompiler{
			FunctionContext: &class_context.ClassContext{},
			cachedSlotWebs:  webs,
			opCodes:         []*OpCode{store1, load1, cmp, store2, load2, putZ},
			opcodeIdToRef:   map[*OpCode][][2]any{store1: {{sharedRef, true}}, store2: {{sharedRef, false}}},
			constantPoolGetter: func(int) values.JavaValue {
				return values.NewJavaClassMember("Owner", "c0", "Z", boolType)
			},
		}
		if !d.localWebHasNumericConsumer(putZ, slot2) {
			t.Fatal("expected localWebHasNumericConsumer to detect numeric web sharing same JavaRef")
		}
		if !d.storeWebHasNumericConsumer(store2) {
			t.Fatal("expected storeWebHasNumericConsumer to detect numeric web sharing same JavaRef")
		}
	})

	t.Run("store_web_has_numeric_consumer_boolean_vs_numeric", func(t *testing.T) {
		for _, sc := range []string{"numeric_own_web", "boolean_only", "kill_switch"} {
			t.Run(sc, func(t *testing.T) {
				if sc == "kill_switch" {
					t.Setenv("JDEC_BOOL_STORE_GUARD_OFF", "1")
				}
				intType := types.NewJavaPrimer(types.JavaInteger)
				boolType := types.NewJavaPrimer(types.JavaBoolean)
				ref := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
				store := op(OP_ISTORE_1, 1)
				store.stackConsumed = []values.JavaValue{values.NewJavaLiteral(1, intType)}
				load := op(OP_ILOAD_1, 2)
				slot := values.NewSlotValue(ref, intType)
				load.stackProduced = []values.JavaValue{slot}

				cons := op(OP_IRETURN, 3)
				cons.stackConsumed = []values.JavaValue{slot}
				load.Target, cons.Source = []*OpCode{cons}, []*OpCode{load}

				fnType := &types.JavaFuncType{ReturnType: boolType}
				if sc != "boolean_only" {
					cons.Instr.OpCode = OP_IADD
					cons.stackConsumed = []values.JavaValue{values.NewJavaLiteral(1, intType), slot}
				}
				d := &Decompiler{
					FunctionType:    fnType,
					FunctionContext: &class_context.ClassContext{FunctionType: fnType},
					cachedSlotWebs:  &slotWeb{webOf: map[*OpCode]int{store: 1, load: 1}},
					opCodes:         []*OpCode{store, load, cons},
					opcodeIdToRef:   map[*OpCode][][2]any{store: {{ref, true}}},
				}
				want := sc == "numeric_own_web"
				if got := d.storeWebHasNumericConsumer(store); got != want {
					t.Fatalf("storeWebHasNumericConsumer(%s) = %v, want %v", sc, got, want)
				}
			})
		}
	})

	t.Run("first_int_decl_from_coerced_bool_field_renders_int_type", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		boolType := types.NewJavaPrimer(types.JavaBoolean)
		intRef := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		intRef.Id.SetName("var15")
		recv := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Owner"))
		recv.Id.SetName("this")
		member := values.NewJavaClassMember("Owner", "flag", "Z", boolType)
		field := values.NewRefMember(recv, "flag", boolType)
		field.OriginPC = 7
		field.HasOriginPC = true
		field.MarkOriginalFieldRead(member, 7)

		stmt := statements.NewAssignStatementPreserveSourceType(intRef, field, true)
		rendered := stmt.String(&class_context.ClassContext{})
		if !strings.HasPrefix(rendered, "int var15 = ") || !strings.Contains(rendered, "? (1) : (0)") {
			t.Fatalf("expected int declaration with boolean-to-int ternary coercion, got: %s", rendered)
		}
	})
}
