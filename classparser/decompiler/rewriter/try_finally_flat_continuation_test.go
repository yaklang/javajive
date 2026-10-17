package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func flatFinallyContinuationTree() *statements.TryCatchStatement {
	tr := conditionalFinallyContinuationTree()
	normal := tr.TryBody[1].(*statements.IfStatement)
	tr.TryBody = []statements.Statement{tr.TryBody[0], normal.IfBody[0], normal.IfBody[1]}
	for i, handler := range tr.CatchBodies {
		tr.CatchBodies[i] = append([]statements.Statement{}, handler[0].(*statements.IfStatement).IfBody...)
	}
	return tr
}

func TestFlatFinallyContinuationRequiresCompleteEffectAndExitDomains(t *testing.T) {
	for _, variant := range []string{"original", "effectful continuation", "unsealed throw", "missing PC", "negative PC", "protected throw", "catch-local operand", "unproved enclosing local", "protected operand", "different cleanup", "protected cleanup", "missing normal cleanup", "normal path lacks continuation", "different original continuations", "intervening cleanup effect", "missing handler coverage", "statement budget"} {
		t.Run(variant, func(t *testing.T) {
			tr := flatFinallyContinuationTree()
			after := tr.CatchBodies[0][1].(*statements.CustomStatement)
			switch variant {
			case "unsealed throw":
				tr.CatchBodies[0][1] = &statements.CustomStatement{ThrownValue: after.ThrownValue, HasOriginPC: true, OriginPC: after.OriginPC}
			case "missing PC":
				after.HasOriginPC = false
			case "negative PC":
				after.OriginPC = -1
			case "protected throw":
				after.OriginPC = 39
			case "catch-local operand":
				after.ThrownValue = tr.Exception[0]
			case "unproved enclosing local":
				after.ThrownValue.(*values.JavaRef).IsParam = false
			case "protected operand", "effectful continuation":
				call := *tr.CatchBodies[0][0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
				call.FunctionName = "after"
				call.Descriptor = "()Ljava/lang/Throwable;"
				call.FuncType = types.NewJavaFuncType(call.Descriptor, nil, types.NewJavaClass("java.lang.Throwable"))
				call.OriginPC = 45
				if variant == "protected operand" {
					call.OriginPC = 35
				}
				tr.CatchBodies[0][1] = statements.NewThrowStatement(&call)
				tr.CatchBodies[0][1].(*statements.CustomStatement).HasOriginPC = true
				tr.CatchBodies[0][1].(*statements.CustomStatement).OriginPC = 48
				if variant == "effectful continuation" {
					after = tr.CatchBodies[0][1].(*statements.CustomStatement)
				}
			case "different cleanup":
				tr.CatchBodies[0][0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).FunctionName = "other"
			case "protected cleanup":
				tr.CatchBodies[0][0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = 35
			case "missing normal cleanup":
				tr.TryBody = []statements.Statement{tr.TryBody[0], tr.TryBody[2]}
			case "normal path lacks continuation":
				tr.TryBody = tr.TryBody[:2]
			case "different original continuations":
				x := statements.NewThrowStatement(after.ThrownValue)
				x.HasOriginPC, x.OriginPC = true, 26
				tr.TryBody[2] = x
			case "intervening cleanup effect":
				tr.CatchBodies[0] = []statements.Statement{tr.CatchBodies[0][0], tr.TryBody[0], after}
			case "missing handler coverage":
				tr.Handlers[1].ProtectedRanges = tr.Handlers[1].ProtectedRanges[:1]
			case "statement budget":
				for i := 0; i < 300; i++ {
					tr.TryBody = append([]statements.Statement{tr.TryBody[0]}, tr.TryBody...)
				}
			}
			view, known := RecoverCatchAllFinally(tr)
			if known != (variant == "original" || variant == "effectful continuation") {
				t.Fatalf("continuation accepted=%v", known)
			}
			if known && (view.Continuation != after || len(view.TryBody) != 2 || len(view.CatchBodies) != 1 || len(view.CatchBodies[0]) != 0 || len(view.Cleanup) != 1) {
				t.Fatal("lost the exact continuation or protected return")
			}
			if len(tr.CatchBodies[0]) < 2 || len(tr.CatchBodies[1]) != 2 || len(tr.TryBody) < 2 {
				t.Fatal("projection mutated the original tree")
			}
		})
	}
}

func TestFlatFinallyContinuationNeedsExplicitRenderingOwner(t *testing.T) {
	tr := flatFinallyContinuationTree()
	handler := tr.Handlers[1]
	proof := finallyProof{cleanup: tr.CatchBodies[1][:1], remaining: 256}
	_, _, accepted := proof.block(tr.CatchBodies[0], true, func(pc int) bool { return finallyContains(handler.ProtectedRanges, pc) })
	if accepted || proof.continuation != nil {
		t.Fatal("query consumer silently consumed a continuation it cannot render")
	}
}
