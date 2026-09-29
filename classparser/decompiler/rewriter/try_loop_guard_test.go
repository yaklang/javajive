package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

func TestLoopHeaderGuardsTryRequiresExitOnOtherArm(t *testing.T) {
	for _, scenario := range []string{"guarded retry", "other body arm", "not header body", "not condition"} {
		t.Run(scenario, func(t *testing.T) {
			loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
			header := core.NewNode(&statements.ConditionStatement{})
			tr := core.NewNode(&statements.MiddleStatement{Flag: "try"})
			success, handler, step := core.NewNode(&statements.MiddleStatement{}), core.NewNode(&statements.MiddleStatement{}), core.NewNode(&statements.MiddleStatement{})
			exit := core.NewNode(&statements.ReturnStatement{})
			handler.IsCatchStart = true
			tr.ProtectedEnd = exit
			loop.AddNext(header)
			header.AddNext(tr)
			header.AddNext(exit)
			tr.AddNext(success)
			tr.AddNext(handler)
			success.AddNext(exit)
			handler.AddNext(step)
			step.AddNext(loop)
			switch scenario {
			case "other body arm":
				header.ReplaceNext(exit, step)
			case "not header body":
				header.ReplaceNext(tr, success)
			case "not condition":
				header.Statement = &statements.MiddleStatement{}
			}
			manager := NewRootStatementManager(loop)
			manager.LoopRegionReducible = true
			manager.DominatorMap = GenerateDominatorTree(loop)
			if got := loopHeaderGuardsTry(manager, loop, tr); got != (scenario == "guarded retry") {
				t.Fatalf("guarded retry=%v", got)
			}
		})
	}
}

func TestRetryProtectedPathDistinguishesSharedCleanup(t *testing.T) {
	for _, sharedRetry := range []bool{false, true} {
		loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
		tr := core.NewNode(&statements.MiddleStatement{Flag: "try"})
		call := core.NewNode(&statements.MiddleStatement{})
		end := core.NewNode(&statements.ReturnStatement{})
		retry := core.NewNode(&statements.MiddleStatement{})
		cleanup := core.NewNode(&statements.ReturnStatement{})
		tr.ProtectedEnd, tr.SharedProtectedHandler = end, true
		retry.IsCatchStart, retry.SharedProtectedHandler = true, sharedRetry
		cleanup.IsCatchStart, cleanup.SharedProtectedHandler = true, true
		loop.AddNext(tr)
		tr.AddNext(call)
		tr.AddNext(retry)
		tr.AddNext(cleanup)
		call.AddNext(end)
		retry.AddNext(loop)
		set := circleElementSet(loop, tr, GenerateDominatorTree(loop), true)
		if set.Has(call) == sharedRetry || set.Has(end) {
			t.Fatalf("sharedRetry=%t: incorrect protected-path boundary", sharedRetry)
		}
	}
}
