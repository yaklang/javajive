package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func conditionalFinallyContinuationTree() *statements.TryCatchStatement {
	ref := func(name string) *values.JavaRef {
		return values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(name))
	}
	caught, primary, token := ref("java.lang.Throwable"), ref("java.lang.Throwable"), ref("java.lang.Throwable")
	token.IsParam = true
	condition := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
	condition.IsParam = true
	call := func(pc int, name, descriptor string) *values.FunctionCallExpression {
		result := types.NewJavaPrimer(types.JavaVoid)
		if descriptor != "()V" {
			result = types.NewJavaClass("java.lang.RuntimeException")
		}
		return &values.FunctionCallExpression{ClassName: "example.Actions", FunctionName: name, Descriptor: descriptor,
			IsStatic: true, Kind: values.InvokeStatic, Object: values.NewJavaClassValue(types.NewJavaClass("example.Actions")),
			HasOriginPC: true, OriginPC: pc, FuncType: types.NewJavaFuncType(descriptor, nil, result)}
	}
	throw := func(pc int, value values.JavaValue) *statements.CustomStatement {
		st := statements.NewThrowStatement(value)
		st.HasOriginPC, st.OriginPC = true, pc
		return st
	}
	cleanup := func(pc int, completion statements.Statement) statements.Statement {
		return statements.NewIfStatement(condition,
			[]statements.Statement{statements.NewExpressionStatement(call(pc, "finish", "()V")), completion},
			[]statements.Statement{throw(pc+3, call(pc+2, "fail", "()Ljava/lang/RuntimeException;"))})
	}
	ret := statements.NewReturnStatement(values.NewJavaLiteral(29, types.NewJavaPrimer(types.JavaInteger)))
	ret.HasOriginPC, ret.OriginPC = true, 25
	tr := statements.NewTryCatchStatement(
		[]statements.Statement{statements.NewExpressionStatement(call(12, "body", "()V")), cleanup(22, ret)},
		[][]statements.Statement{{cleanup(42, throw(48, token))}, {cleanup(82, throw(88, primary))}})
	tr.Exception = []*values.JavaRef{caught, primary}
	tr.Handlers = []statements.CatchHandler{{EntryPC: 30, ProtectedRanges: [][2]int{{10, 20}}},
		{EntryPC: 70, CatchAll: true, ProtectedRanges: [][2]int{{10, 20}, {30, 40}}}}
	return tr
}

func TestConditionalFinallyContinuationRequiresClosedEffectAndExitDomains(t *testing.T) {
	for _, variant := range []string{"original", "unsealed continuation", "missing continuation PC", "negative continuation PC", "protected continuation", "catch-local operand", "unproved enclosing local", "folded protected operand", "unsealed primary prefix", "wrong primary", "different cleanup", "different condition", "normal path lacks continuation", "different original continuations", "missing handler coverage", "statement budget"} {
		t.Run(variant, func(t *testing.T) {
			tr := conditionalFinallyContinuationTree()
			normal := tr.TryBody[1].(*statements.IfStatement)
			caught := tr.CatchBodies[0][0].(*statements.IfStatement)
			cleanup := tr.CatchBodies[1][0].(*statements.IfStatement)
			after := caught.IfBody[1].(*statements.CustomStatement)
			switch variant {
			case "unsealed continuation":
				caught.IfBody[1] = &statements.CustomStatement{ThrownValue: after.ThrownValue, HasOriginPC: true, OriginPC: after.OriginPC}
			case "missing continuation PC":
				after.HasOriginPC = false
			case "negative continuation PC":
				after.OriginPC = -1
			case "protected continuation":
				after.OriginPC = 39
			case "catch-local operand":
				after.ThrownValue = tr.Exception[0]
			case "unproved enclosing local":
				after.ThrownValue.(*values.JavaRef).IsParam = false
			case "folded protected operand":
				x := *caught.ElseBody[0].(*statements.CustomStatement).ThrownValue.(*values.FunctionCallExpression)
				x.OriginPC = 35
				after.ThrownValue = &x
			case "unsealed primary prefix":
				x := cleanup.IfBody[1].(*statements.CustomStatement)
				cleanup.IfBody[1] = &statements.CustomStatement{ThrownValue: x.ThrownValue, HasOriginPC: true, OriginPC: x.OriginPC}
			case "wrong primary":
				cleanup.IfBody[1].(*statements.CustomStatement).ThrownValue = tr.Exception[0]
			case "different cleanup":
				caught.IfBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).FunctionName = "other"
			case "different condition":
				caught.Condition = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
			case "normal path lacks continuation":
				normal.IfBody = normal.IfBody[:1]
			case "different original continuations":
				x := statements.NewThrowStatement(after.ThrownValue)
				x.HasOriginPC, x.OriginPC = true, 26
				normal.IfBody[1] = x
			case "missing handler coverage":
				tr.Handlers[1].ProtectedRanges = tr.Handlers[1].ProtectedRanges[:1]
			case "statement budget":
				for i := 0; i < 300; i++ {
					tr.TryBody = append([]statements.Statement{tr.TryBody[0]}, tr.TryBody...)
				}
			}
			view, known := RecoverCatchAllFinally(tr)
			if known != (variant == "original") {
				t.Fatalf("continuation accepted=%v", known)
			}
			if known && (view.Continuation != after || len(view.TryBody) != 2 || len(view.CatchBodies) != 1 || len(view.CatchBodies[0]) != 0 || len(view.Cleanup) != 1) {
				t.Fatal("lost the exact continuation or protected exits")
			}
			if len(tr.CatchBodies) != 2 || len(tr.TryBody) < 2 || caught.IfBody[1] == nil || len(cleanup.IfBody) != 2 {
				t.Fatal("projection mutated the original tree")
			}
		})
	}
}

func TestFinallyContinuationRequiresAnExplicitRenderingOwner(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		tr := conditionalFinallyContinuationTree()
		handler := tr.Handlers[1]
		cleanup, known := finallyHandlerCleanup(tr.CatchBodies[1], tr.Exception[1], handler.ProtectedRanges)
		if !known {
			t.Fatal("original cleanup missing")
		}
		var rows []core.HandlerRange
		for _, h := range tr.Handlers {
			for _, interval := range h.ProtectedRanges {
				rows = append(rows, core.HandlerRange{StartPc: uint16(interval[0]), EndPc: uint16(interval[1])})
			}
		}
		proof := finallyProof{cleanup: cleanup, rows: rows, excluded: tr.Exception, remaining: 256, allowContinuation: enabled}
		_, _, accepted := proof.block(tr.CatchBodies[0], true, func(pc int) bool { return finallyContains(handler.ProtectedRanges, pc) })
		if accepted != enabled || (proof.continuation != nil) != enabled {
			t.Fatal("a resource/query consumer silently consumed a continuation it cannot render")
		}
	}
}
