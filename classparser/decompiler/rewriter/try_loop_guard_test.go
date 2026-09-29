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
