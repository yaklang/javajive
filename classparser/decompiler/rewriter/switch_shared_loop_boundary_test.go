package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

func TestNativeSharedSwitchLoopBoundaryRequiresClosedPaths(t *testing.T) {
	for _, variant := range []string{"same loop", "shared arm", "certified return", "dead end", "cycle", "unproved return", "terminal reaches tail", "foreign continue", "hidden continue", "encoded continue", "unregistered loop", "irreducible loop", "after loop", "nested loop", "different protection", "missing pc", "oversized path", "oversized loop set"} {
		t.Run(variant, func(t *testing.T) {
			root := jumpTestNode("root")
			loop := core.NewNode(&statements.DoWhileStatement{})
			condition := core.NewNode(&statements.ConditionStatement{})
			owner := core.NewNode(&statements.MiddleStatement{Flag: "switch"})
			target, retry := jumpTestNode("sharedEffect()"), jumpTestNode("retryEffect()")
			owner.OriginPC, owner.HasOriginPC = 10, true
			target.OriginPC, target.HasOriginPC = 30, true
			owner.IsInCircle, target.IsInCircle, retry.IsInCircle = true, true, true
			root.AddNext(loop)
			loop.AddNext(condition)
			condition.AddNext(owner)
			condition.AddNext(target)
			owner.AddNext(target)
			owner.AddNext(retry)
			retry.AddNext(loop)
			target.AddNext(loop)
			manager := NewRootStatementManager(root)
			manager.WhileNode = []*core.Node{loop}
			manager.LoopRegionReducible = true
			want := false
			switch variant {
			case "same loop":
				want = true
			case "shared arm":
				arm := jumpTestNode("sharedRetryEffect()")
				owner.AddNext(arm)
				arm.AddNext(retry)
				want = true
			case "certified return", "unproved return", "terminal reaches tail":
				ret := core.NewNode(&statements.ReturnStatement{OriginPC: 20, HasOriginPC: variant != "unproved return"})
				ret.OriginPC, ret.HasOriginPC = 20, true
				owner.AddNext(ret)
				if variant == "terminal reaches tail" {
					ret.AddNext(target)
				}
				want = variant == "certified return"
			case "dead end":
				owner.AddNext(jumpTestNode("unfinishedEffect()"))
			case "cycle":
				cycle := jumpTestNode("unrecordedCycle()")
				owner.AddNext(cycle)
				cycle.AddNext(cycle)
				cycle.AddNext(target)
			case "foreign continue":
				retry.RemoveNext(loop)
				retry.AddNext(root)
			case "hidden continue":
				retry.HideNext = loop
			case "encoded continue":
				retry.EncodedJumps = map[*core.Node]bool{loop: true}
			case "unregistered loop":
				manager.WhileNode = nil
			case "irreducible loop":
				manager.LoopRegionReducible = false
			case "after loop":
				target.RemoveNext(loop)
				target.AddNext(core.NewNode(&statements.MiddleStatement{Flag: "end"}))
			case "nested loop":
				inner := core.NewNode(&statements.DoWhileStatement{})
				condition.ReplaceNextSliceKeepOrder(owner, []*core.Node{inner})
				inner.AddNext(owner)
				retry.ReplaceNextSliceKeepOrder(loop, []*core.Node{inner})
				manager.WhileNode = append(manager.WhileNode, inner)
			case "different protection":
				root.HasProtectedRange = true
				root.ProtectedStartPC, root.ProtectedEndPC = 5, 30
			case "missing pc":
				owner.HasOriginPC = false
			case "oversized path":
				retry.RemoveNext(loop)
				cursor := retry
				for i := 0; i < 257; i++ {
					next := jumpTestNode("effect()")
					cursor.AddNext(next)
					cursor = next
				}
				cursor.AddNext(loop)
			case "oversized loop set":
				for len(manager.WhileNode) < 65 {
					manager.WhileNode = append(manager.WhileNode, loop)
				}
			}
			manager.DominatorMap = GenerateDominatorTree(root)
			if got := externalSharedSwitchDefaultContinuation(manager, owner, target); got != want {
				t.Fatalf("closed boundary=%v, want %v", got, want)
			}
			if owner.Next[0] != target || owner.Next[1] != retry {
				t.Fatal("certificate changed original edge order")
			}
		})
	}
}
