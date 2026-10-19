package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func monitorContinuationFixture() (*statements.TryCatchStatement, *statements.IfStatement) {
	condition := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
	condition.IsParam = true
	call := func(pc int) statements.Statement {
		return statements.NewExpressionStatement(&values.FunctionCallExpression{ClassName: "p.Actions", FunctionName: "effect", Descriptor: "()V", IsStatic: true, Kind: values.InvokeStatic, HasOriginPC: true, OriginPC: pc})
	}
	ret := func(pc int) statements.Statement {
		return &statements.ReturnStatement{HasOriginPC: true, OriginPC: pc}
	}
	branch := statements.NewIfStatement(condition,
		[]statements.Statement{call(12), statements.NewOriginalMonitorStatement("monitor_exit", nil, 20, 3), call(21), ret(22)},
		[]statements.Statement{call(32), statements.NewOriginalMonitorStatement("monitor_exit", nil, 40, 3), call(41), ret(42)})
	primary := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
	tr := &statements.TryCatchStatement{TryBody: []statements.Statement{branch}, Exception: []*values.JavaRef{primary},
		Handlers:    []statements.CatchHandler{{CatchAll: true, EntryPC: 100, ProtectedRanges: [][2]int{{10, 21}, {30, 41}, {100, 104}}}},
		CatchBodies: [][]statements.Statement{{statements.NewOriginalMonitorStatement("monitor_exit", nil, 102, 3), &statements.CustomStatement{ThrownValue: primary, HasOriginPC: true, OriginPC: 105}}}}
	return tr, branch
}

func TestMonitorContinuationsPartitionEveryCertifiedReleaseWithoutInventingValues(t *testing.T) {
	for _, variant := range []string{"two tails", "one branch falls into shared suffix", "throw-only branch", "unknown handler", "unknown release", "wrong owner", "release outside held domain", "call outside held domain", "tail inside held domain", "missing tail call origin", "missing return origin", "return before release", "path holds monitor", "held local escapes", "opaque tail", "repeated release", "depth limit"} {
		t.Run(variant, func(t *testing.T) {
			tr, branch := monitorContinuationFixture()
			originalA, originalB := branch.IfBody, branch.ElseBody
			switch variant {
			case "one branch falls into shared suffix":
				tr.TryBody = append(tr.TryBody, branch.ElseBody...)
				branch.ElseBody = nil
			case "throw-only branch":
				thrown := statements.NewThrowStatement(values.JavaNull)
				thrown.HasOriginPC, thrown.OriginPC = true, 13
				branch.IfBody = []statements.Statement{thrown}
				// A third path keeps two distinct released continuations.
				copy := *branch
				copy.IfBody = originalA
				branch.ElseBody = []statements.Statement{&copy}
			case "unknown handler":
				tr.Handlers[0].CatchAll = false
			case "unknown release":
				branch.IfBody[1] = statements.NewMiddleStatement("monitor_exit", nil)
			case "wrong owner":
				branch.IfBody[1] = statements.NewOriginalMonitorStatement("monitor_exit", nil, 20, 4)
			case "release outside held domain":
				tr.Handlers[0].ProtectedRanges[0][1] = 20
			case "call outside held domain":
				branch.IfBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = 9
			case "tail inside held domain":
				branch.IfBody[2].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = 19
			case "missing tail call origin":
				branch.IfBody[2].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).HasOriginPC = false
			case "missing return origin":
				branch.IfBody[3].(*statements.ReturnStatement).HasOriginPC = false
			case "return before release":
				branch.IfBody = branch.IfBody[3:]
			case "path holds monitor":
				branch.ElseBody = branch.ElseBody[:1]
			case "held local escapes":
				ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
				store := statements.NewAssignStatement(ref, values.NewJavaLiteral(7, ref.Type()), true)
				store.HasOriginPC, store.OriginPC = true, 11
				branch.IfBody = append([]statements.Statement{store}, branch.IfBody...)
				branch.IfBody[len(branch.IfBody)-1].(*statements.ReturnStatement).JavaValue = ref
			case "opaque tail":
				branch.IfBody[2] = statements.NewCustomStatement(nil, nil)
			case "repeated release":
				branch.IfBody = append(branch.IfBody[:2], append([]statements.Statement{branch.IfBody[1]}, branch.IfBody[2:]...)...)
			case "depth limit":
				for i := 0; i < 40; i++ {
					tr.TryBody = []statements.Statement{statements.NewIfStatement(branch.Condition, tr.TryBody, nil)}
				}
			}
			prefix, held, tail, known := originalMonitorContinuations(tr, 3)
			want := variant == "two tails" || variant == "one branch falls into shared suffix" || variant == "throw-only branch"
			if known != want {
				t.Fatalf("proved=%v expected=%v", known, want)
			}
			if !known {
				return
			}
			decl := prefix[0].(*statements.AssignStatement)
			if len(prefix) != 1 || !decl.IsDeclare || decl.IsFirst || decl.JavaValue != nil || len(held) != 1 || len(tail) != 1 {
				t.Fatal("partition invented initialization or lost source ownership")
			}
			if variant == "two tails" {
				decision := held[0].(*statements.IfStatement)
				dispatch := tail[0].(*statements.IfStatement)
				if decision.Condition != branch.Condition || decision.IfBody[0] != originalA[0] || decision.ElseBody[0] != originalB[0] || dispatch.IfBody[0] != originalA[2] || dispatch.ElseBody[0] != originalB[2] || len(branch.IfBody) != 4 {
					t.Fatal("partition moved evaluation or mutated the original graph")
				}
			}
		})
	}
}
