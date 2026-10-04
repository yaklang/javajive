package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNullLocalFlowRequiresAllReachingDefinitions(t *testing.T) {
	for _, name := range []string{"null copies", "entry", "non-null arm", "missing definition", "primitive store", "cycle", "reused ref", "exception edge"} {
		t.Run(name, func(t *testing.T) {
			typ := types.NewJavaClass("java.lang.Object")
			ref := values.NewJavaRef(utils.NewRootVariableId(), values.NewJavaLiteral("null", typ), typ)
			source := op(OP_ASTORE_1, 1)
			load := op(OP_ALOAD_1, 2)
			copyStore := op(OP_ASTORE_2, 3)
			copyLoad := op(OP_ALOAD_2, 4)
			value := values.NewSlotValue(ref, typ)
			copied := values.NewSlotValue(values.NewJavaRef(utils.NewRootVariableId(), nil, typ), typ)
			source.stackConsumed = []values.JavaValue{values.NewJavaLiteral("null", typ)}
			load.stackProduced = []values.JavaValue{value}
			copyStore.stackConsumed = []values.JavaValue{value}
			copyLoad.stackProduced = []values.JavaValue{copied}
			webs := &slotWeb{webOf: map[*OpCode]int{source: 1, load: 1, copyStore: 2, copyLoad: 2}, entryWeb: map[int]int{}}
			if name == "entry" {
				webs.entryWeb[1] = 1
			}
			if name == "missing definition" {
				webs.webOf[op(OP_ASTORE_1, 8)] = 1
			}
			if name == "non-null arm" {
				other := op(OP_ASTORE_1, 8)
				other.stackConsumed = []values.JavaValue{values.NewNewExpression(typ)}
				webs.webOf[other] = 1
			}
			if name == "primitive store" {
				source.Instr = op(OP_ISTORE_1, 1).Instr
			}
			if name == "cycle" {
				source.stackConsumed = []values.JavaValue{copied}
			}
			if name == "reused ref" {
				other := op(OP_ASTORE_3, 9)
				other.stackConsumed = []values.JavaValue{values.NewNewExpression(typ)}
				webs.webOf[other] = 3
			}
			d := &Decompiler{FunctionContext: &class_context.ClassContext{}, cachedSlotWebs: webs}
			if name == "exception edge" {
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 2, HandlerPc: 8}}
			}
			d.propagateNullOnlyLocalLoads()
			want := name == "null copies" || name == "reused ref"
			if values.IsNullLiteral(values.UnpackSoltValue(copied)) != want {
				t.Fatalf("null proof=%v want=%v", values.IsNullLiteral(values.UnpackSoltValue(copied)), want)
			}
			if ref.Val == nil || !values.IsNullLiteral(ref.Val) {
				t.Fatal("mutated simulator identity")
			}
		})
	}
}

func TestNullLocalFlowCaptureRequiresUnambiguousDefinitionIdentity(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		typ := types.NewJavaClass("java.lang.Object")
		target := types.NewJavaClass("probe.Token")
		first := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
		second := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
		a, b, load, capture := op(OP_ASTORE_1, 1), op(OP_ASTORE_2, 2), op(OP_ALOAD_2, 3), op(OP_INVOKEDYNAMIC, 4)
		a.stackConsumed = []values.JavaValue{values.NewJavaLiteral("null", typ)}
		b.stackConsumed = []values.JavaValue{first}
		load.stackProduced = []values.JavaValue{second}
		snap := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
		webs := &slotWeb{webOf: map[*OpCode]int{a: 1, b: 2, load: 2}}
		d := &Decompiler{FunctionContext: &class_context.ClassContext{}, opCodes: []*OpCode{a, b, load, capture}, cachedSlotWebs: webs, opcodeIdToRef: map[*OpCode][][2]any{a: {{first, true}}, b: {{second, true}}}, evaluationSnapshots: map[*OpCode][]EvaluationSnapshot{capture: {{Ref: snap, Value: second, Operand: true, ExpectedType: target}}}}
		if ambiguous {
			other := op(OP_ALOAD_3, 8)
			other.stackProduced = []values.JavaValue{second}
			webs.webOf[other] = 3
		}
		d.propagateNullOnlyLocalLoads()
		d.refreshReferenceOperandSnapshotTypes()
		got := values.IsNullLiteral(d.evaluationSnapshots[capture][0].Value)
		if got == ambiguous {
			t.Fatal("wrong capture proof", ambiguous, got)
		}
		if !ambiguous && snap.Type().String(d.FunctionContext) != "Token" {
			t.Fatal("capture lost call-site type", snap.Type())
		}
	}
}
