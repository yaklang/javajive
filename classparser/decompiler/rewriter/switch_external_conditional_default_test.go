package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

func TestSharedSwitchDefaultRequiresExactEnclosingConditionalExit(t *testing.T) {
	for _, variant := range []string{"original", "same protection", "effect before shared exit", "certified return", "unproved return", "competing normal exit", "oversized boundary", "different protection", "missing owner pc", "missing target pc", "back edge", "owned default", "foreign condition", "hidden", "encoded", "loop", "catch", "try anchor", "ordinary predecessor"} {
		t.Run(variant, func(t *testing.T) {
			root := jumpTestNode("root")
			condition := core.NewNode(&statements.ConditionStatement{})
			owner := core.NewNode(&statements.MiddleStatement{Flag: "switch"})
			target := jumpTestNode("commonInvocation()")
			owner.OriginPC, owner.HasOriginPC = 10, true
			target.OriginPC, target.HasOriginPC = 30, true
			root.AddNext(condition)
			condition.AddNext(owner)
			condition.AddNext(target)
			owner.AddNext(target)
			switch variant {
			case "effect before shared exit":
				pre := jumpTestNode("alternateEffect()")
				condition.ReplaceNextSliceKeepOrder(target, []*core.Node{pre})
				pre.AddNext(target)
			case "certified return", "unproved return":
				ret := core.NewNode(&statements.ReturnStatement{OriginPC: 20, HasOriginPC: variant == "certified return"})
				ret.OriginPC, ret.HasOriginPC = 20, true
				owner.AddNext(ret)
				ret.AddNext(core.NewNode(&statements.MiddleStatement{Flag: "end"}))
			case "competing normal exit":
				competitor := jumpTestNode("otherContinuation()")
				condition.AddNext(competitor)
				owner.AddNext(competitor)
			case "oversized boundary":
				owner.RemoveNext(target)
				cursor := owner
				for i := 0; i < 257; i++ {
					next := jumpTestNode("effect()")
					cursor.AddNext(next)
					cursor = next
				}
				cursor.AddNext(target)
			case "same protection", "different protection":
				root.HasProtectedRange = true
				root.ProtectedStartPC, root.ProtectedEndPC = 5, 40
				if variant == "different protection" {
					root.ProtectedEndPC = 30
				}
			case "missing owner pc":
				owner.HasOriginPC = false
			case "missing target pc":
				target.HasOriginPC = false
			case "back edge":
				target.OriginPC = 5
			case "owned default":
				condition.RemoveNext(target)
			case "foreign condition":
				root.AddNext(owner)
			case "hidden":
				target.HideNext = root
			case "encoded":
				condition.EncodedJumps = map[*core.Node]bool{target: true}
			case "loop":
				target.IsInCircle = true
			case "catch":
				target.IsCatchStart = true
			case "try anchor":
				target.IsTryCatch = true
			case "ordinary predecessor":
				condition.Statement = statements.NewExpressionStatement(nil)
			}
			manager := NewRootStatementManager(root)
			manager.DominatorMap = GenerateDominatorTree(root)
			if got := externalSharedSwitchDefaultContinuation(manager, owner, target); got != (variant == "original" || variant == "same protection" || variant == "effect before shared exit" || variant == "certified return") {
				t.Fatalf("shared default certificate=%v", got)
			}
			if condition.Next[0] != owner || (variant != "oversized boundary" && owner.Next[0] != target) {
				t.Fatal("certificate changed the original graph")
			}
		})
	}
}
