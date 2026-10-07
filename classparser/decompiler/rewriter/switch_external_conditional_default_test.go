package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

func TestSharedSwitchDefaultRequiresExactEnclosingConditionalExit(t *testing.T) {
	for _, variant := range []string{"original", "same protection", "different protection", "missing owner pc", "missing target pc", "back edge", "owned default", "foreign condition", "hidden", "encoded", "loop", "catch", "try anchor", "ordinary predecessor"} {
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
			if got := externalConditionalSwitchDefault(manager, owner, target); got != (variant == "original" || variant == "same protection") {
				t.Fatalf("shared default certificate=%v", got)
			}
			if condition.Next[0] != owner || owner.Next[0] != target {
				t.Fatal("certificate changed the original graph")
			}
		})
	}
}
