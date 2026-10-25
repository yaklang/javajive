package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/omap"
	"slices"
	"testing"
)

func TestSwitchWrappingRetainsAbruptEdgeWithoutDuplicatingLoopTransfer(t *testing.T) {
	for _, kind := range []string{"continue", "break"} {
		t.Run(kind, func(t *testing.T) {
			root := core.NewNode(&statements.MiddleStatement{Flag: "start"})
			loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
			exit := core.NewNode(&statements.ReturnStatement{})
			end := core.NewNode(&statements.MiddleStatement{Flag: "end"})
			exit.AddNext(end)
			indices := omap.NewEmptyOrderedMap[int, int]()
			indices.Set(1, 0)
			indices.Set(2, 1)
			offsets := omap.NewEmptyOrderedMap[int, int32]()
			offsets.Set(1, 10)
			offsets.Set(2, 20)
			owner := core.NewNode(&statements.MiddleStatement{Flag: "switch", Data: []any{indices, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)), offsets, statements.SwitchDefault{Index: 2, Offset: 30}}})
			abrupt := core.NewNode(statements.NewSourceTransferStatement(kind, "OUTER"))
			abrupt.IsJmp = true
			target := loop
			if kind == "break" {
				target = exit
				root.AddNext(exit)
			}
			abrupt.AddNext(target)
			normal := jumpTestNode("caseEffect()")
			merge := jumpTestNode("commit()")
			leave := core.NewNode(statements.NewSourceTransferStatement("break", ""))
			leave.HideNext = merge
			normal.AddNext(leave)
			terminal := core.NewNode(&statements.ReturnStatement{})
			terminal.AddNext(end)
			root.AddNext(loop)
			loop.AddNext(owner)
			owner.AddNext(abrupt)
			owner.AddNext(normal)
			owner.AddNext(terminal)
			owner.AddNext(merge)
			merge.AddNext(loop)
			owner.SwitchPrepared = true
			owner.MergeNode = merge
			manager := NewRootStatementManager(root)
			manager.WhileNode = []*core.Node{loop}
			manager.SwitchNode = []*core.Node{owner}
			manager.currentNodeId = 100
			manager.DominatorMap = GenerateDominatorTree(root)
			if err := SwitchRewriter(manager, owner); err != nil {
				t.Fatal(err)
			}
			wrapped := loop.Next[0]
			if !wrapped.EncodedJumps[target] || wrapped.EncodedJumps[merge] || !slices.Contains(wrapped.Next, merge) {
				t.Fatal("switch lost normal/abrupt frontier")
			}
			sw := wrapped.Statement.(*statements.SwitchStatement)
			if sw.Cases[0].Body[0] != abrupt.Statement {
				t.Fatal("original branch-local transfer changed")
			}
			for i := 0; i < 2; i++ {
				manager.DominatorMap = GenerateDominatorTree(root)
				if err := LoopJmpRewriter(manager, loop); err != nil {
					t.Fatal(err)
				}
				if !slices.Contains(wrapped.Next, target) || !slices.Contains(wrapped.Next, merge) || !wrapped.EncodedJumps[target] {
					t.Fatal("loop replaced an already encoded transfer")
				}
			}
		})
	}
}
