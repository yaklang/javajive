package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func buildInitMemberCP(className, methodName, desc string) *values.JavaClassMember {
	jt, err := types.ParseMethodDescriptor(desc)
	if err != nil {
		panic(err)
	}
	return values.NewJavaClassMember(className, methodName, desc, jt)
}

func TestAllocationInitBindingSingleAndMultiBranch(t *testing.T) {
	t.Run("single_init_with_intervening_arg", func(t *testing.T) {
		newOp := op(OP_NEW, 1)
		rootStore := op(OP_ASTORE_1, 2)
		prepArg := op(OP_ICONST_1, 3)
		aload := op(OP_ALOAD_1, 4)
		initOp := op(OP_INVOKESPECIAL, 5)
		initOp.Data = []byte{0, 1}
		retOp := op(OP_RETURN, 6)

		newOp.Target, rootStore.Source = []*OpCode{rootStore}, []*OpCode{newOp}
		rootStore.Target, prepArg.Source = []*OpCode{prepArg}, []*OpCode{rootStore}
		prepArg.Target, aload.Source = []*OpCode{aload}, []*OpCode{prepArg}
		aload.Target, initOp.Source = []*OpCode{initOp}, []*OpCode{aload}
		initOp.Target, retOp.Source = []*OpCode{retOp}, []*OpCode{initOp}

		ne := values.NewNewExpression(types.NewJavaClass("pkg.Box"))
		rootStore.stackConsumed = []values.JavaValue{ne}

		arg := values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
		boxRef := values.NewJavaRef(utils.NewRootVariableId(), ne, types.NewJavaClass("pkg.Box"))
		boxRef.Id.SetName("box")
		loaded := values.NewSlotValue(boxRef, types.NewJavaClass("pkg.Box"))
		aload.stackProduced = []values.JavaValue{loaded}
		initOp.stackConsumed = []values.JavaValue{arg, loaded}

		d := &Decompiler{
			opCodes:       []*OpCode{newOp, rootStore, prepArg, aload, initOp, retOp},
			opcodeIdToRef: map[*OpCode][][2]any{rootStore: {{boxRef, true}}},
			constantPoolGetter: func(int) values.JavaValue {
				return buildInitMemberCP("pkg/Box", "<init>", "(I)V")
			},
			stackValueProducers: map[values.JavaValue]*OpCode{
				loaded: aload,
			},
		}

		plan := d.planConstructorAllocations(nil)
		entry := plan.entryFor(initOp)
		if entry == nil {
			t.Fatal("expected single-init allocation plan entry")
		}
		if entry.rootStore != rootStore || entry.ne != ne || entry.multiInit {
			t.Fatalf("unexpected plan entry: rootMatch=%v neMatch=%v multi=%v", entry.rootStore == rootStore, entry.ne == ne, entry.multiInit)
		}
		if !entry.closureSet[rootStore] || len(entry.inits) != 1 || entry.inits[0] != initOp {
			t.Fatal("expected closure to contain rootStore and initOp")
		}
	})

	t.Run("multi_init_mutually_exclusive_branches", func(t *testing.T) {
		newOp := op(OP_NEW, 1)
		rootStore := op(OP_ASTORE_1, 2)
		branch := op(OP_IFEQ, 3)
		leftLoad := op(OP_ALOAD_1, 4)
		leftInit := op(OP_INVOKESPECIAL, 5)
		leftInit.Data = []byte{0, 1}
		rightLoad := op(OP_ALOAD_1, 6)
		rightInit := op(OP_INVOKESPECIAL, 7)
		rightInit.Data = []byte{0, 1}
		join := op(OP_RETURN, 8)

		newOp.Target, rootStore.Source = []*OpCode{rootStore}, []*OpCode{newOp}
		rootStore.Target, branch.Source = []*OpCode{branch}, []*OpCode{rootStore}
		branch.Target = []*OpCode{leftLoad, rightLoad}
		leftLoad.Source, rightLoad.Source = []*OpCode{branch}, []*OpCode{branch}
		leftLoad.Target, leftInit.Source = []*OpCode{leftInit}, []*OpCode{leftLoad}
		rightLoad.Target, rightInit.Source = []*OpCode{rightInit}, []*OpCode{rightLoad}
		leftInit.Target, rightInit.Target = []*OpCode{join}, []*OpCode{join}
		join.Source = []*OpCode{leftInit, rightInit}

		ne := values.NewNewExpression(types.NewJavaClass("pkg.Box"))
		rootStore.stackConsumed = []values.JavaValue{ne}

		boxRef := values.NewJavaRef(utils.NewRootVariableId(), ne, types.NewJavaClass("pkg.Box"))
		boxRef.Id.SetName("box")
		leftSlot := values.NewSlotValue(boxRef, types.NewJavaClass("pkg.Box"))
		rightSlot := values.NewSlotValue(boxRef, types.NewJavaClass("pkg.Box"))
		leftLoad.stackProduced = []values.JavaValue{leftSlot}
		rightLoad.stackProduced = []values.JavaValue{rightSlot}
		leftInit.stackConsumed = []values.JavaValue{leftSlot}
		rightInit.stackConsumed = []values.JavaValue{rightSlot}

		d := &Decompiler{
			opCodes:       []*OpCode{newOp, rootStore, branch, leftLoad, leftInit, rightLoad, rightInit, join},
			opcodeIdToRef: map[*OpCode][][2]any{rootStore: {{boxRef, true}}},
			constantPoolGetter: func(int) values.JavaValue {
				return buildInitMemberCP("pkg/Box", "<init>", "()V")
			},
			stackValueProducers: map[values.JavaValue]*OpCode{
				leftSlot:  leftLoad,
				rightSlot: rightLoad,
			},
		}

		plan := d.planConstructorAllocations(nil)
		leftEntry := plan.entryFor(leftInit)
		rightEntry := plan.entryFor(rightInit)
		if leftEntry == nil || rightEntry == nil || leftEntry != rightEntry {
			t.Fatalf("expected shared multi-init entry, got left=%v right=%v", leftEntry, rightEntry)
		}
		if !leftEntry.multiInit || len(leftEntry.inits) != 2 {
			t.Fatalf("expected multiInit=true with 2 inits, got multi=%v len=%d", leftEntry.multiInit, len(leftEntry.inits))
		}
	})

	t.Run("kill_switch", func(t *testing.T) {
		t.Setenv("JDEC_ALLOC_INIT_BIND_OFF", "1")
		newOp := op(OP_NEW, 1)
		rootStore := op(OP_ASTORE_1, 2)
		aload := op(OP_ALOAD_1, 3)
		initOp := op(OP_INVOKESPECIAL, 4)
		initOp.Data = []byte{0, 1}
		newOp.Target, rootStore.Source = []*OpCode{rootStore}, []*OpCode{newOp}
		rootStore.Target, aload.Source = []*OpCode{aload}, []*OpCode{rootStore}
		aload.Target, initOp.Source = []*OpCode{initOp}, []*OpCode{aload}

		ne := values.NewNewExpression(types.NewJavaClass("pkg.Box"))
		rootStore.stackConsumed = []values.JavaValue{ne}
		loaded := values.NewSlotValue(nil, types.NewJavaClass("pkg.Box"))
		aload.stackProduced = []values.JavaValue{loaded}
		initOp.stackConsumed = []values.JavaValue{loaded}

		d := &Decompiler{
			opCodes: []*OpCode{newOp, rootStore, aload, initOp},
			constantPoolGetter: func(int) values.JavaValue {
				return buildInitMemberCP("pkg/Box", "<init>", "()V")
			},
			stackValueProducers: map[values.JavaValue]*OpCode{loaded: aload},
		}
		if plan := d.planConstructorAllocations(nil); plan != nil {
			t.Fatal("expected nil plan when JDEC_ALLOC_INIT_BIND_OFF=1")
		}
	})
}

func TestConstructorAllocationNegativeFailClosed(t *testing.T) {
	scenarios := []string{
		"receiver_escapes_before_init",
		"sequential_dual_init_not_exclusive",
		"protected_root_store",
		"constructor_class_mismatch",
		"multiple_reaching_stores",
		"missing_stack_producer",
	}
	for _, scenario := range scenarios {
		t.Run(scenario, func(t *testing.T) {
			newOp := op(OP_NEW, 1)
			rootStore := op(OP_ASTORE_1, 2)
			midLoad := op(OP_ALOAD_1, 3)
			midSink := op(OP_PUTFIELD, 4)
			aload := op(OP_ALOAD_1, 5)
			initOp := op(OP_INVOKESPECIAL, 6)
			initOp.Data = []byte{0, 1}
			retOp := op(OP_RETURN, 7)

			var extraInit *OpCode
			var altStore *OpCode
			targetClass := "pkg/Box"
			var protected map[*OpCode]bool

			ne := values.NewNewExpression(types.NewJavaClass("pkg.Box"))
			rootStore.stackConsumed = []values.JavaValue{ne}
			boxRef := values.NewJavaRef(utils.NewRootVariableId(), ne, types.NewJavaClass("pkg.Box"))
			boxRef.Id.SetName("box")
			midSlot := values.NewSlotValue(boxRef, types.NewJavaClass("pkg.Box"))
			loaded := values.NewSlotValue(boxRef, types.NewJavaClass("pkg.Box"))
			midLoad.stackProduced = []values.JavaValue{midSlot}
			aload.stackProduced = []values.JavaValue{loaded}
			initOp.stackConsumed = []values.JavaValue{loaded}

			d := &Decompiler{
				opcodeIdToRef: map[*OpCode][][2]any{rootStore: {{boxRef, true}}},
				constantPoolGetter: func(int) values.JavaValue {
					return buildInitMemberCP(targetClass, "<init>", "()V")
				},
				stackValueProducers: map[values.JavaValue]*OpCode{
					loaded: aload,
				},
			}

			switch scenario {
			case "receiver_escapes_before_init":
				newOp.Target, rootStore.Source = []*OpCode{rootStore}, []*OpCode{newOp}
				rootStore.Target, midLoad.Source = []*OpCode{midLoad}, []*OpCode{rootStore}
				midLoad.Target, midSink.Source = []*OpCode{midSink}, []*OpCode{midLoad}
				midSink.Target, aload.Source = []*OpCode{aload}, []*OpCode{midSink}
				aload.Target, initOp.Source = []*OpCode{initOp}, []*OpCode{aload}
				initOp.Target, retOp.Source = []*OpCode{retOp}, []*OpCode{initOp}
				midSink.stackConsumed = []values.JavaValue{midSlot}
				d.opCodes = []*OpCode{newOp, rootStore, midLoad, midSink, aload, initOp, retOp}
			case "sequential_dual_init_not_exclusive":
				extraLoad := op(OP_ALOAD_1, 7)
				extraInit = op(OP_INVOKESPECIAL, 8)
				extraInit.Data = []byte{0, 1}
				extraSlot := values.NewSlotValue(nil, types.NewJavaClass("pkg.Box"))
				extraLoad.stackProduced = []values.JavaValue{extraSlot}
				extraInit.stackConsumed = []values.JavaValue{extraSlot}
				d.stackValueProducers[extraSlot] = extraLoad

				newOp.Target, rootStore.Source = []*OpCode{rootStore}, []*OpCode{newOp}
				rootStore.Target, aload.Source = []*OpCode{aload}, []*OpCode{rootStore}
				aload.Target, initOp.Source = []*OpCode{initOp}, []*OpCode{aload}
				initOp.Target, extraLoad.Source = []*OpCode{extraLoad}, []*OpCode{initOp}
				extraLoad.Target, extraInit.Source = []*OpCode{extraInit}, []*OpCode{extraLoad}
				extraInit.Target, retOp.Source = []*OpCode{retOp}, []*OpCode{extraInit}
				retOp.CurrentOffset = 9
				d.opCodes = []*OpCode{newOp, rootStore, aload, initOp, extraLoad, extraInit, retOp}
			case "protected_root_store":
				newOp.Target, rootStore.Source = []*OpCode{rootStore}, []*OpCode{newOp}
				rootStore.Target, aload.Source = []*OpCode{aload}, []*OpCode{rootStore}
				aload.Target, initOp.Source = []*OpCode{initOp}, []*OpCode{aload}
				initOp.Target, retOp.Source = []*OpCode{retOp}, []*OpCode{initOp}
				protected = map[*OpCode]bool{rootStore: true}
				d.opCodes = []*OpCode{newOp, rootStore, aload, initOp, retOp}
			case "constructor_class_mismatch":
				targetClass = "pkg/Other"
				newOp.Target, rootStore.Source = []*OpCode{rootStore}, []*OpCode{newOp}
				rootStore.Target, aload.Source = []*OpCode{aload}, []*OpCode{rootStore}
				aload.Target, initOp.Source = []*OpCode{initOp}, []*OpCode{aload}
				initOp.Target, retOp.Source = []*OpCode{retOp}, []*OpCode{initOp}
				d.opCodes = []*OpCode{newOp, rootStore, aload, initOp, retOp}
			case "multiple_reaching_stores":
				branch := op(OP_IFEQ, 2)
				altStore = op(OP_ASTORE_1, 3)
				altStore.stackConsumed = []values.JavaValue{values.NewNewExpression(types.NewJavaClass("pkg.Box"))}
				rootStore.CurrentOffset = 4
				newOp.Target, branch.Source = []*OpCode{branch}, []*OpCode{newOp}
				branch.Target = []*OpCode{altStore, rootStore}
				altStore.Source, rootStore.Source = []*OpCode{branch}, []*OpCode{branch}
				altStore.Target, rootStore.Target = []*OpCode{aload}, []*OpCode{aload}
				aload.Source = []*OpCode{altStore, rootStore}
				aload.Target, initOp.Source = []*OpCode{initOp}, []*OpCode{aload}
				initOp.Target, retOp.Source = []*OpCode{retOp}, []*OpCode{initOp}
				d.opCodes = []*OpCode{newOp, branch, altStore, rootStore, aload, initOp, retOp}
			case "missing_stack_producer":
				newOp.Target, rootStore.Source = []*OpCode{rootStore}, []*OpCode{newOp}
				rootStore.Target, aload.Source = []*OpCode{aload}, []*OpCode{rootStore}
				aload.Target, initOp.Source = []*OpCode{initOp}, []*OpCode{aload}
				initOp.Target, retOp.Source = []*OpCode{retOp}, []*OpCode{initOp}
				delete(d.stackValueProducers, loaded)
				d.opCodes = []*OpCode{newOp, rootStore, aload, initOp, retOp}
			}

			plan := d.planConstructorAllocations(protected)
			if plan.entryFor(initOp) != nil {
				t.Fatalf("expected fail-closed nil entry for scenario %s", scenario)
			}
			if extraInit != nil && plan.entryFor(extraInit) != nil {
				t.Fatalf("expected fail-closed nil entry for sequential extraInit")
			}
		})
	}
}

func TestConstructorThisAliasDelegationRendering(t *testing.T) {
	intType := types.NewJavaPrimer(types.JavaInteger)
	voidType := types.NewJavaPrimer(types.JavaVoid)

	for _, scenario := range []string{"this_alias_renders_this_call", "non_alias_falls_back_to_ref_name", "kill_switch_disables_this_alias"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "kill_switch_disables_this_alias" {
				t.Setenv("JDEC_THIS_ALIAS_SUPER_OFF", "1")
			}
			recv := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("pkg.Box"))
			recv.Id.SetName("var6")
			recv.IsThisAlias = scenario != "non_alias_falls_back_to_ref_name"

			call := &values.FunctionCallExpression{
				ClassName:       "pkg.Box",
				FunctionName:    "<init>",
				Descriptor:      "(I)V",
				IsSpecialInvoke: true,
				Kind:            values.InvokeSpecial,
				Object:          values.NewSlotValue(recv, recv.Type()),
				Arguments:       []values.JavaValue{values.NewJavaLiteral(1, intType)},
				FuncType:        &types.JavaFuncType{ParamTypes: []types.JavaType{intType}, ReturnType: voidType},
			}
			ctx := &class_context.ClassContext{
				ClassName:         "pkg.Box",
				FunctionName:      "<init>",
				CurrentMethodDesc: "()V",
				FunctionType:      &types.JavaFuncType{ReturnType: voidType},
			}
			got := call.String(ctx)
			want := "var6(1)"
			if scenario == "this_alias_renders_this_call" {
				want = "this(1)"
			}
			if got != want {
				t.Fatalf("call.String() = %q, want %q", got, want)
			}
		})
	}
}

func TestOpsBetweenStraightLineFastPathAndFallback(t *testing.T) {
	d := &Decompiler{}
	if between, ok := d.opsBetween(nil, nil); ok || between != nil {
		t.Fatalf("expected false for nil inputs")
	}

	root := op(OP_ASTORE_1, 1)
	if between, ok := d.opsBetween(root, []*OpCode{root}); !ok || len(between) != 1 || !between[root] {
		t.Fatalf("expected single-op match, got ok=%v len=%d", ok, len(between))
	}

	mid1 := op(OP_ICONST_1, 2)
	mid2 := op(OP_ALOAD_1, 3)
	initOp := op(OP_INVOKESPECIAL, 4)
	root.Target, mid1.Source = []*OpCode{mid1}, []*OpCode{root}
	mid1.Target, mid2.Source = []*OpCode{mid2}, []*OpCode{mid1}
	mid2.Target, initOp.Source = []*OpCode{initOp}, []*OpCode{mid2}
	between, ok := d.opsBetween(root, []*OpCode{initOp})
	if !ok || len(between) != 4 || !between[root] || !between[mid1] || !between[mid2] || !between[initOp] {
		t.Fatalf("straight-line fast path mismatch: ok=%v len=%d", ok, len(between))
	}

	// Branched region misses fast path (len(Target) != 1) and must fall back to general BFS.
	branch := op(OP_IFEQ, 10)
	left := op(OP_ICONST_0, 11)
	right := op(OP_ICONST_1, 12)
	joinInit := op(OP_INVOKESPECIAL, 13)
	root2 := op(OP_ASTORE_1, 9)
	root2.Target, branch.Source = []*OpCode{branch}, []*OpCode{root2}
	branch.Target = []*OpCode{left, right}
	left.Source, right.Source = []*OpCode{branch}, []*OpCode{branch}
	left.Target, right.Target = []*OpCode{joinInit}, []*OpCode{joinInit}
	joinInit.Source = []*OpCode{left, right}
	between2, ok2 := d.opsBetween(root2, []*OpCode{joinInit})
	if !ok2 || len(between2) != 5 || !between2[left] || !between2[right] {
		t.Fatalf("branch fallback mismatch: ok=%v len=%d", ok2, len(between2))
	}
}

func TestPlanConstructorAllocationsLargeMethodOver8192(t *testing.T) {
	ops := make([]*OpCode, 0, 8505)
	var prev *OpCode
	for i := 0; i < 8496; i++ {
		nop := op(OP_NOP, uint16(i))
		if prev != nil {
			prev.Target = []*OpCode{nop}
			nop.Source = []*OpCode{prev}
		}
		ops = append(ops, nop)
		prev = nop
	}

	newOp := op(OP_NEW, 9000)
	rootStore := op(OP_ASTORE_1, 9001)
	prepArg := op(OP_ICONST_1, 9002)
	aload := op(OP_ALOAD_1, 9003)
	initOp := op(OP_INVOKESPECIAL, 9004)
	initOp.Data = []byte{0, 1}
	retOp := op(OP_RETURN, 9005)

	prev.Target, newOp.Source = []*OpCode{newOp}, []*OpCode{prev}
	newOp.Target, rootStore.Source = []*OpCode{rootStore}, []*OpCode{newOp}
	rootStore.Target, prepArg.Source = []*OpCode{prepArg}, []*OpCode{rootStore}
	prepArg.Target, aload.Source = []*OpCode{aload}, []*OpCode{prepArg}
	aload.Target, initOp.Source = []*OpCode{initOp}, []*OpCode{aload}
	initOp.Target, retOp.Source = []*OpCode{retOp}, []*OpCode{initOp}
	ops = append(ops, newOp, rootStore, prepArg, aload, initOp, retOp)

	ne := values.NewNewExpression(types.NewJavaClass("pkg.Box"))
	rootStore.stackConsumed = []values.JavaValue{ne}
	boxRef := values.NewJavaRef(utils.NewRootVariableId(), ne, types.NewJavaClass("pkg.Box"))
	boxRef.Id.SetName("box")
	loaded := values.NewSlotValue(boxRef, types.NewJavaClass("pkg.Box"))
	aload.stackProduced = []values.JavaValue{loaded}
	initOp.stackConsumed = []values.JavaValue{values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)), loaded}

	d := &Decompiler{
		opCodes:       ops,
		opcodeIdToRef: map[*OpCode][][2]any{rootStore: {{boxRef, true}}},
		constantPoolGetter: func(int) values.JavaValue {
			return buildInitMemberCP("pkg/Box", "<init>", "(I)V")
		},
		stackValueProducers: map[values.JavaValue]*OpCode{
			loaded: aload,
		},
	}

	plan := d.planConstructorAllocations(nil)
	if plan.entryFor(initOp) == nil {
		t.Fatalf("expected large-method (%d ops) allocation plan entry", len(d.opCodes))
	}

	d.opCodes = make([]*OpCode, 32769)
	if plan := d.planConstructorAllocations(nil); plan != nil {
		t.Fatalf("expected nil allocation plan above 32768 opcodes")
	}
}
