package core

import (
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestReferenceCopyWebsAndCaptureCopiesReachFixedPoint(t *testing.T) {
	base := types.NewJavaClass("example.Base")
	narrow := types.NewJavaClass("example.FirstArm")
	ids := utils.NewRootVariableId()
	newRef := func(typ types.JavaType) *values.JavaRef {
		ids = ids.Next()
		return values.NewJavaRef(ids, nil, typ)
	}
	parameter, first, second, afterCapture := newRef(base), newRef(narrow), newRef(narrow), newRef(narrow)
	parameter.IsParam = true
	// Val is an earlier definition, not the declaration of the current source.
	parameter.Val = newRef(narrow)
	firstCopy := &OpCode{Instr: InstrInfos[OP_ASTORE_1], stackConsumed: []values.JavaValue{second}}
	secondCopy := &OpCode{Instr: InstrInfos[OP_ASTORE_2], stackConsumed: []values.JavaValue{parameter}}
	indy := &OpCode{Instr: InstrInfos[OP_INVOKEDYNAMIC], CurrentOffset: 42}
	d := &Decompiler{FunctionContext: &class_context.ClassContext{}}
	sim := NewStackSimulation(nil, map[int]*values.JavaRef{}, ids.Next())
	firstLoad := values.NewSlotValue(first, narrow)
	args, err := d.snapshotDynamicOperands(indy, sim, []values.JavaValue{firstLoad}, []types.JavaType{base})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := args[0].(*values.JavaRef)
	tailCopy := &OpCode{Instr: InstrInfos[OP_ASTORE_3], stackConsumed: []values.JavaValue{snapshot}}
	// Deliberately reverse the dependency order, as a DFS through a loop can.
	d.opCodes = []*OpCode{tailCopy, firstCopy, secondCopy, indy}
	d.opcodeIdToRef = map[*OpCode][][2]any{tailCopy: {{afterCapture, true}}, firstCopy: {{first, true}}, secondCopy: {{second, true}}}
	d.cachedSlotWebs = &slotWeb{webOf: map[*OpCode]int{tailCopy: 1, firstCopy: 2, secondCopy: 3}, entryWeb: map[int]int{}}
	d.unifyReferenceWebs()
	for _, ref := range []*values.JavaRef{parameter, first, second, snapshot, afterCapture} {
		if !reflect.DeepEqual(ref.Type().RawType(), base.RawType()) {
			t.Fatalf("a copy kept the DFS branch type: %s", ref.Type().String(d.FunctionContext))
		}
	}
	if snapshot.Val != firstLoad || values.SameLocal(snapshot, first) || snapshot == afterCapture {
		t.Fatal("declaration propagation collapsed capture identity")
	}
}

func TestDisjointWebsSharingRefDoNotInventGenericElementType(t *testing.T) {
	for _, kind := range []string{"mixed", "same generic", "parameter", "one web"} {
		t.Run(kind, func(t *testing.T) {
			generic := types.NewParameterizedType("java.util.Iterator", []types.JavaType{types.NewJavaClass("E")})
			raw := types.NewJavaClass("java.util.Iterator")
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, generic)
			first := types.JavaType(raw)
			if kind == "same generic" {
				first = generic
			}
			ref.IsParam = kind == "parameter"
			a := &OpCode{stackConsumed: []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, first)}}
			b := &OpCode{stackConsumed: []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, generic)}}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, opcodeIdToRef: map[*OpCode][][2]any{a: {{ref, true}}, b: {{ref, false}}}}
			owners := map[*values.JavaRef]map[int]bool{ref: {1: true, 2: true}}
			if kind == "one web" {
				delete(owners[ref], 2)
			}
			d.eraseConflictingSharedWebTypes(map[int][]*OpCode{1: {a}, 2: {b}}, []int{1, 2}, owners)
			_, stillParameterized := types.AsParameterizedType(ref.Type())
			if stillParameterized != (kind != "mixed") {
				t.Fatalf("declared type=%s", ref.Type().String(d.FunctionContext))
			}
			if kind == "mixed" {
				call := &values.FunctionCallExpression{Object: ref, FunctionName: "next", FuncType: types.NewJavaFuncType("()Ljava/lang/Object;", nil, types.NewJavaClass("java.lang.Object"))}
				if got := call.Type().String(d.FunctionContext); got != "Object" {
					t.Fatalf("a raw reaching definition must not claim E: %s", got)
				}
			}
		})
	}
}
