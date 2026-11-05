package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

func TestSwitchJoinOrderUsesOwnedPathsInsteadOfNodeIDs(t *testing.T) {
	for _, variant := range []string{"join numbered first", "step numbered first", "permuted candidates", "loop backedge", "cycle", "unrelated joins", "foreign join", "hidden", "exception entry", "pending try", "oversized"} {
		t.Run(variant, func(t *testing.T) {
			root, owner, left, right, bypass, join, step, exit := jumpTestNode("root"), jumpTestNode("switch"), jumpTestNode("a"), jumpTestNode("b"), jumpTestNode("skip"), jumpTestNode("join"), jumpTestNode("step"), jumpTestNode("exit")
			root.AddNext(owner)
			owner.AddNext(left)
			owner.AddNext(right)
			owner.AddNext(bypass)
			left.AddNext(join)
			right.AddNext(join)
			bypass.AddNext(step)
			join.AddNext(step)
			step.AddNext(exit)
			join.Id, step.Id = 1, 90
			candidates := []*core.Node{step, join}
			want := join
			switch variant {
			case "step numbered first":
				join.Id, step.Id = 90, 1
			case "permuted candidates":
				candidates = []*core.Node{join, step}
			case "loop backedge":
				step.AddNext(owner)
			case "cycle":
				step.AddNext(join)
				want = nil
			case "unrelated joins":
				join.RemoveNext(step)
				join.AddNext(exit)
				want = nil
			case "foreign join":
				root.AddNext(step)
				want = nil
			case "hidden":
				join.HideNext = exit
				want = nil
			case "exception entry":
				join.IsCatchStart = true
				want = nil
			case "pending try":
				join.IsTryCatch = true
				want = nil
			case "oversized":
				join.RemoveNext(step)
				cursor := join
				for i := 0; i < 513; i++ {
					n := core.NewNode(&statements.MiddleStatement{})
					cursor.AddNext(n)
					cursor = n
				}
				cursor.AddNext(step)
				want = nil
			}
			m := NewRootStatementManager(root)
			m.DominatorMap = GenerateDominatorTree(root)
			if got := firstSwitchJoin(m, owner, candidates); got != want {
				t.Fatalf("join=%p want=%p", got, want)
			}
		})
	}
}
