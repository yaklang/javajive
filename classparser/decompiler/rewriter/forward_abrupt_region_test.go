package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestForwardRegionAllowsOriginalAbruptPathsWithoutInteriorReentry(t *testing.T) {
	for _, kind := range []string{"return", "throw", "missing origin", "false origin", "terminal continuation", "outside entry", "exit reenters interior"} {
		t.Run(kind, func(t *testing.T) {
			m, n := forwardRegionFixture()
			owner, inner, work, exit := n[1], n[2], n[5], n[6]
			condition := m.NewNode(&statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))})
			condition.HasOriginPC, condition.OriginPC = true, 54
			ret := &statements.ReturnStatement{JavaValue: values.NewJavaLiteral(-37, types.NewJavaPrimer(types.JavaInteger)), HasOriginPC: true, OriginPC: 58}
			terminal := m.NewNode(ret)
			terminal.HasOriginPC, terminal.OriginPC = true, 58
			end := m.NewNode(&statements.MiddleStatement{Flag: "end"})
			terminal.AddNext(end)
			work.RemoveAllNext()
			work.AddNext(condition)
			condition.AddNext(terminal)
			condition.AddNext(exit)
			condition.TrueNode = func() *core.Node { return condition.Next[0] }
			condition.FalseNode = func() *core.Node { return condition.Next[1] }
			want := kind == "return" || kind == "throw"
			switch kind {
			case "throw":
				throw := statements.NewThrowStatement(values.NewJavaLiteral(nil, types.NewJavaClass("java.lang.RuntimeException")))
				throw.HasOriginPC, throw.OriginPC = true, 58
				terminal.Statement = throw
			case "missing origin":
				ret.HasOriginPC = false
			case "false origin":
				ret.OriginPC = 59
			case "terminal continuation":
				terminal.AddNext(exit)
			case "outside entry":
				n[0].AddNext(terminal)
			case "exit reenters interior":
				exit.AddNext(terminal)
			}
			m.DominatorMap = GenerateDominatorTree(m.RootNode)
			if got := closedForwardRegion(m, owner, exit); got != want {
				t.Fatalf("closed=%v want=%v", got, want)
			}
			if !want {
				return
			}
			// The earlier shared computation is not an exit: its successor
			// reenters the proposed body through the common final continuation.
			if closedForwardRegion(m, owner, work) {
				t.Fatal("normal exit reenters the proposed interior")
			}
			materializeForwardRegionExits(m, inner)
			region := m.forwardRegions[owner]
			if region == nil || region.exit != exit {
				t.Fatal("bypass lost its enclosing continuation")
			}
			materializeForwardRegionExits(m, condition)
			if len(condition.Next) != 2 || condition.Next[0] != terminal || condition.Next[1] != exit {
				t.Fatal("method END became a normal frontier and inserted a spurious bypass")
			}
			if err := IfRewriter(m, condition); err != nil {
				t.Fatal(err)
			}
			m.DominatorMap = GenerateDominatorTree(m.RootNode)
			if err := IfRewriter(m, inner); err != nil {
				t.Fatal(err)
			}
			m.DominatorMap = GenerateDominatorTree(m.RootNode)
			if err := IfRewriter(m, owner); err != nil {
				t.Fatal(err)
			}
			wrapper := n[0].Next[0]
			block, ok := wrapper.Statement.(*statements.DoWhileStatement)
			if !ok || block.Label != region.label || len(wrapper.Next) != 1 || wrapper.Next[0] != exit {
				t.Fatal("normal continuation was changed")
			}
			count := 0
			var walk func([]statements.Statement)
			walk = func(body []statements.Statement) {
				for _, st := range body {
					if st == terminal.Statement {
						count++
					}
					if branch, ok := st.(*statements.IfStatement); ok {
						walk(branch.IfBody)
						walk(branch.ElseBody)
					}
				}
			}
			walk(block.Body)
			if count != 1 {
				t.Fatalf("original abrupt exit emitted %d times", count)
			}
		})
	}
}

func TestForwardRegionFinalAbruptBoundaryNeedsNoNormalFallthrough(t *testing.T) {
	for _, kind := range []string{"encoded exit", "foreign target", "no continuation", "two continuations"} {
		t.Run(kind, func(t *testing.T) {
			m, n := forwardRegionFixture()
			owner, exit := n[1], n[6]
			region := &forwardRegion{exit: exit, label: "retained_region"}
			m.forwardRegions = map[*core.Node]*forwardRegion{owner: region}
			ret := statements.NewReturnStatement(values.NewJavaLiteral(29, types.NewJavaPrimer(types.JavaInteger)))
			branch := statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{ret}, []statements.Statement{statements.NewSourceTransferStatement("break", region.label)})
			start := m.NewNode(branch)
			start.AddNext(exit)
			start.EncodedJumps = map[*core.Node]bool{exit: true}
			m.RootNode = start
			switch kind {
			case "foreign target":
				start.RemoveAllNext()
				start.AddNext(n[7])
				start.EncodedJumps[n[7]] = true
			case "no continuation":
				start.RemoveAllNext()
			case "two continuations":
				start.EncodedJumps = nil
				start.AddNext(n[7])
			}
			err := finishForwardRegion(m, owner, start)
			if (err == nil) != (kind == "encoded exit") {
				t.Fatalf("finish: %v", err)
			}
			if err == nil {
				block, ok := m.RootNode.Statement.(*statements.DoWhileStatement)
				if !ok || len(block.Body) != 1 || block.Body[0] != branch || len(m.RootNode.Next) != 1 || m.RootNode.Next[0] != exit || len(m.forwardRegions) != 0 {
					t.Fatal("lost abrupt body or exact continuation")
				}
			}
		})
	}
}

func TestForwardRegionStopsAtOriginalMethodExitBeforeSentinel(t *testing.T) {
	for _, kind := range []string{"return", "throw", "missing origin", "unexpected continuation"} {
		t.Run(kind, func(t *testing.T) {
			m, n := forwardRegionFixture()
			owner, exit := n[1], n[6]
			m.forwardRegions = map[*core.Node]*forwardRegion{owner: {exit: exit, label: "retained_region"}}
			ret := statements.NewReturnStatement(values.NewJavaLiteral(29, types.NewJavaPrimer(types.JavaInteger)))
			ret.HasOriginPC, ret.OriginPC = true, 54
			terminal := m.NewNode(ret)
			terminal.HasOriginPC, terminal.OriginPC = true, 54
			if kind == "throw" {
				st := statements.NewThrowStatement(values.NewJavaLiteral(nil, types.NewJavaClass("java.lang.Exception")))
				st.HasOriginPC, st.OriginPC = true, 54
				terminal.Statement = st
			}
			end := m.NewNode(&statements.MiddleStatement{Flag: "end"})
			terminal.AddNext(end)
			m.RootNode = terminal
			if kind == "missing origin" {
				terminal.HasOriginPC = false
			}
			if kind == "unexpected continuation" {
				terminal.AddNext(exit)
			}
			err := finishForwardRegion(m, owner, terminal)
			want := kind == "return" || kind == "throw"
			if (err == nil) != want {
				t.Fatalf("finish: %v", err)
			}
			if want {
				block := m.RootNode.Statement.(*statements.DoWhileStatement)
				if len(block.Body) != 1 || block.Body[0] != terminal.Statement {
					t.Fatal("original method exit changed or END became a source statement")
				}
			}
		})
	}
}
