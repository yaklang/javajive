package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestOwnedMonitorSourceRequiresPairedInertAbruptCompletion(t *testing.T) {
	for _, scenario := range []string{"return", "throw", "direct continuation", "direct materialized return", "wrong owner", "unknown exit", "effectful return", "effect after release", "dangling release", "loop transfer", "nested uncertified monitor", "opaque statement", "depth cap"} {
		t.Run(scenario, func(t *testing.T) {
			exit := statements.NewOriginalMonitorStatement("monitor_exit", nil, 20, 3)
			terminal := statements.NewReturnStatement(values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger)))
			terminal.HasOriginPC = true
			terminal.OriginPC = 21
			arm := []statements.Statement{exit, terminal}
			switch scenario {
			case "throw":
				st := &statements.CustomStatement{ThrownValue: values.JavaNull, HasOriginPC: true, OriginPC: 21}
				arm[1] = st
			case "wrong owner":
				arm[0] = statements.NewOriginalMonitorStatement("monitor_exit", nil, 20, 4)
			case "unknown exit":
				arm[0] = statements.NewMiddleStatement("monitor_exit", nil)
			case "effectful return":
				terminal.JavaValue = &values.FunctionCallExpression{FunctionName: "event"}
			case "effect after release":
				arm = append(arm[:1], &statements.ExpressionStatement{}, terminal)
			case "dangling release":
				arm = arm[:1]
			case "loop transfer":
				arm[1] = statements.NewSourceTransferStatement("break", "")
			case "nested uncertified monitor":
				arm = []statements.Statement{statements.NewSynchronizedStatement(values.JavaNull, nil)}
			case "opaque statement":
				arm = []statements.Statement{statements.NewCustomStatement(nil, nil)}
			case "depth cap":
				for i := 0; i < 35; i++ {
					arm = []statements.Statement{statements.NewIfStatement(values.JavaNull, arm, nil)}
				}
			}
			branch := statements.NewIfStatement(values.JavaNull, arm, nil)
			input := []statements.Statement{branch}
			if scenario == "direct materialized return" {
				input = arm
			}
			if scenario == "direct continuation" {
				input = []statements.Statement{exit, &statements.ExpressionStatement{}}
			}
			got, tail, known := originalMonitorSourceBody(input, 3)
			want := scenario == "return" || scenario == "throw" || scenario == "direct continuation" || scenario == "direct materialized return"
			if known != want {
				t.Fatalf("known=%v", known)
			}
			if known {
				if scenario == "direct continuation" {
					if len(got) != 0 || len(tail) != 1 || tail[0] != input[1] {
						t.Fatal("continuation moved")
					}
				} else if scenario == "direct materialized return" {
					if len(got) != 1 || got[0] != terminal || len(tail) != 0 || len(input) != 2 {
						t.Fatal("snapshot return left monitor scope")
					}
				} else {
					if len(got[0].(*statements.IfStatement).IfBody) != 1 || len(branch.IfBody) != 2 {
						t.Fatal("original/abrupt completion changed")
					}
				}
			}
		})
	}
}
func TestFinallyMonitorCleanupNeedsOriginalDomainAndCompleteBody(t *testing.T) {
	for _, scenario := range []string{"paired", "unknown acquire", "protected acquire", "wrong receiver", "changed body", "unknown body", "missing body origin", "edited receiver"} {
		t.Run(scenario, func(t *testing.T) {
			tr := finallyFixture()
			ref := values.JavaNull
			monitor := func(pc int, body []statements.Statement) *statements.SynchronizedStatement {
				return statements.NewSynchronizedStatementFromMonitor(statements.NewOriginalMonitorStatement("monitor_enter", ref, pc, pc), body)
			}
			normal, handler := monitor(13, []statements.Statement{tr.TryBody[1]}), monitor(43, []statements.Statement{tr.CatchBodies[1][0]})
			switch scenario {
			case "unknown acquire":
				normal = statements.NewSynchronizedStatement(ref, normal.Body)
			case "protected acquire":
				normal = monitor(11, normal.Body)
			case "wrong receiver":
				normal = statements.NewSynchronizedStatementFromMonitor(statements.NewOriginalMonitorStatement("monitor_enter", values.NewJavaLiteral("other", types.NewJavaClass("Object")), 13, 13), normal.Body)
			case "changed body":
				normal.Body = nil
			case "unknown body":
				normal.Body = []statements.Statement{statements.NewCustomStatement(nil, nil)}
			case "missing body origin":
				normal.Body[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).HasOriginPC = false
			case "edited receiver":
				normal.Argument = values.NewJavaLiteral("edited", types.NewJavaClass("Object"))
			}
			tr.TryBody[1] = normal
			tr.CatchBodies[1][0] = handler
			got, known := RecoverCatchAllFinally(tr)
			if known != (scenario == "paired") {
				t.Fatalf("known=%v", known)
			}
			if known && (len(got.Cleanup) != 1 || got.Cleanup[0] != handler || len(tr.TryBody) != 3) {
				t.Fatal("original cleanup graph changed")
			}
		})
	}
}

func TestOriginalMonitorHandlerRequiresExactReleaseAndPrimary(t *testing.T) {
	for _, scenario := range []string{"closed", "typed handler", "missing rows", "invalid rows", "acquire covered", "release uncovered", "wrong acquisition", "wrong primary", "missing throw origin", "throw covered", "wrong entry"} {
		t.Run(scenario, func(t *testing.T) {
			tr := finallyFixture()
			primary := tr.Exception[1]
			exit := statements.NewOriginalMonitorStatement("monitor_exit", nil, 42, 3)
			thrown := tr.CatchBodies[1][1].(*statements.CustomStatement)
			tr.Exception = tr.Exception[1:]
			tr.CatchBodies = [][]statements.Statement{{exit, thrown}}
			tr.Handlers = []statements.CatchHandler{{CatchAll: true, EntryPC: 40, ProtectedRanges: [][2]int{{10, 12}, {40, 44}}}}
			switch scenario {
			case "typed handler":
				tr.Handlers[0].CatchAll = false
			case "missing rows":
				tr.Handlers[0].ProtectedRanges = nil
			case "invalid rows":
				tr.Handlers[0].ProtectedRanges[0] = [2]int{12, 10}
			case "acquire covered":
				tr.Handlers[0].ProtectedRanges[0] = [2]int{3, 4}
			case "release uncovered":
				tr.Handlers[0].ProtectedRanges[1] = [2]int{40, 42}
			case "wrong acquisition":
				tr.CatchBodies[0][0] = statements.NewOriginalMonitorStatement("monitor_exit", nil, 42, 4)
			case "wrong primary":
				thrown.ThrownValue = values.JavaNull
			case "missing throw origin":
				thrown.HasOriginPC = false
			case "throw covered":
				tr.Handlers[0].ProtectedRanges[1][1] = 46
			case "wrong entry":
				tr.Handlers[0].EntryPC = 43
			}
			if originalMonitorHandlerClosed(tr, 3) != (scenario == "closed") {
				t.Fatal("handler closure")
			}
			if tr.Exception[0] != primary {
				t.Fatal("exception identity changed")
			}
		})
	}
}
