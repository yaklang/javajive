package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestTypedCatchLayerRequiresCompletePriorityAndCoverage(t *testing.T) {
	for _, scenario := range []string{"covered", "same type", "missing handler", "catch all", "sibling", "partial interval", "uncovered invoke", "missing invoke PC", "missing constructor", "uncovered constructor", "opaque", "budget"} {
		t.Run(scenario, func(t *testing.T) {
			exception := func(name string) *values.JavaRef {
				return values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(name))
			}
			call := &values.FunctionCallExpression{IsStatic: true, Kind: values.InvokeStatic, FunctionName: "read", Descriptor: "()Ljava/lang/Object;", OriginPC: 2, HasOriginPC: true}
			ctor := &values.FunctionCallExpression{Kind: values.InvokeSpecial, FunctionName: "<init>", Descriptor: "()V", OriginPC: 12, HasOriginPC: true}
			allocation := values.NewNewExpression(types.NewJavaClass("example.Choice"))
			allocation.OriginPC = 11
			allocation.HasOriginPC = true
			allocation.ConstructorCall = ctor
			tr := &statements.TryCatchStatement{TryBody: []statements.Statement{&statements.ReturnStatement{JavaValue: call}}, Exception: []*values.JavaRef{exception("java.lang.IllegalArgumentException"), exception("java.lang.Exception")}, CatchBodies: [][]statements.Statement{{&statements.ReturnStatement{JavaValue: allocation}}, {&statements.ReturnStatement{JavaValue: values.JavaNull}}}, Handlers: []statements.CatchHandler{{EntryPC: 10, ProtectedRanges: [][2]int{{0, 9}}}, {EntryPC: 20, ProtectedRanges: [][2]int{{0, 9}, {10, 19}}}}}
			switch scenario {
			case "same type":
				tr.Exception[0] = exception("java.lang.Exception")
			case "missing handler":
				tr.Handlers = tr.Handlers[:1]
			case "catch all":
				tr.Handlers[1].CatchAll = true
			case "sibling":
				tr.Handlers[1].ProtectedRanges[1] = [2]int{13, 19}
			case "partial interval":
				tr.Handlers[1].ProtectedRanges[0] = [2]int{0, 8}
			case "uncovered invoke":
				call.OriginPC = 19
			case "missing invoke PC":
				call.HasOriginPC = false
			case "missing constructor":
				allocation.ConstructorCall = nil
			case "uncovered constructor":
				ctor.OriginPC = 19
			case "opaque":
				tr.CatchBodies[0] = []statements.Statement{&statements.CustomStatement{}}
			case "budget":
				for i := 0; i < 512; i++ {
					tr.TryBody = append(tr.TryBody, &statements.ExpressionStatement{Expression: call})
				}
			}
			out, ok := RecoverCoveredTypedCatchLayer(tr)
			want := scenario == "covered" || scenario == "same type"
			if ok != want {
				t.Fatalf("proved=%v want=%v", ok, want)
			}
			if ok {
				inner, valid := out.TryBody[0].(*statements.TryCatchStatement)
				if !valid || len(out.Exception) != 1 || out.Exception[0] != tr.Exception[1] || len(inner.Exception) != 1 || inner.TryBody[0] != tr.TryBody[0] || inner.CatchBodies[0][0] != tr.CatchBodies[0][0] || len(tr.Exception) != 2 {
					t.Fatal("changed priority, evaluation or original tree")
				}
			}
		})
	}
}

func TestTypedCatchCoverageKeepsSizedAllocationAndExactStore(t *testing.T) {
	for _, scenario := range []string{"sized", "folded", "missing allocation", "uncovered length", "missing store", "uncovered store", "opaque index", "missing folded end"} {
		t.Run(scenario, func(t *testing.T) {
			array := values.NewNewArrayExpression(types.NewJavaArrayType(types.NewJavaClass("java.lang.Object")), values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))
			array.OriginPC = 2
			array.HasOriginPC = true
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, array.Type())
			allocation := statements.NewAssignStatement(ref, array, true)
			store := statements.NewAssignStatement(ref, values.JavaNull, false)
			store.ArrayMember = &values.JavaArrayMember{Object: ref, Index: values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))}
			store.OriginPC = 5
			store.HasOriginPC = true
			body := []statements.Statement{allocation, store}
			switch scenario {
			case "folded", "missing folded end":
				array.Initializer = []values.JavaValue{values.JavaNull}
				array.EvaluationEndPC = 5
				array.HasEvaluationEndPC = scenario == "folded"
				body = body[:1]
			case "missing allocation":
				array.HasOriginPC = false
			case "uncovered length":
				array.Length = []values.JavaValue{&values.FunctionCallExpression{OriginPC: 11, HasOriginPC: true, Descriptor: "()I", IsStatic: true, Kind: values.InvokeStatic}}
			case "missing store":
				store.HasOriginPC = false
			case "uncovered store":
				store.OriginPC = 11
			case "opaque index":
				store.ArrayMember.Index = &values.CustomValue{}
			}
			proof := typedCatchCoverage{covered: func(pc int) bool { return pc >= 0 && pc < 10 }, remaining: 512}
			got := proof.block(body)
			want := scenario == "sized" || scenario == "folded"
			if got != want {
				t.Fatalf("covered=%v want=%v", got, want)
			}
		})
	}
}
