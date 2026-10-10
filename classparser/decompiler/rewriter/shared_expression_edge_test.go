package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestSharedExpressionEdgeSplitPreservesPathsAndExceptionDomain(t *testing.T) {
	for _, kind := range []string{"split", "reverse", "same protected range", "different protected range", "shared range gap", "missing range", "missing pc", "assignment", "different continuation", "hidden", "encoded", "loop", "owned"} {
		t.Run(kind, func(t *testing.T) {
			root, external, continuation := jumpTestNode("root"), jumpTestNode("external"), jumpTestNode("continuation")
			condition := core.NewNode(&statements.ConditionStatement{})
			value := values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger))
			target := core.NewNode(statements.NewExpressionStatement(value))
			condition.OriginPC, condition.HasOriginPC = 10, true
			target.OriginPC, target.HasOriginPC = 20, true
			root.AddNext(condition)
			root.AddNext(external)
			condition.AddNext(target)
			condition.AddNext(continuation)
			condition.TrueNode = func() *core.Node { return condition.Next[0] }
			condition.FalseNode = func() *core.Node { return condition.Next[1] }
			external.AddNext(target)
			target.AddNext(continuation)
			switch kind {
			case "reverse":
				condition.TrueNode, condition.FalseNode = condition.FalseNode, condition.TrueNode
			case "same protected range", "different protected range", "shared range gap", "missing range":
				root.Statement = statements.NewMiddleStatement(statements.MiddleTryStart, nil)
				root.IsTryCatch, root.HasProtectedRange = true, true
				root.ProtectedStartPC, root.ProtectedEndPC = 10, 30
				if kind == "different protected range" {
					root.ProtectedEndPC = 20
				} else if kind == "shared range gap" {
					root.SharedProtectedRanges = []core.HandlerRange{{StartPc: 10, EndPc: 15}, {StartPc: 25, EndPc: 30}}
				} else if kind == "missing range" {
					root.HasProtectedRange = false
				}
			case "missing pc":
				target.HasOriginPC = false
			case "assignment":
				target.Statement = &statements.AssignStatement{}
			case "different continuation":
				target.ReplaceNext(continuation, jumpTestNode("other"))
			case "hidden":
				target.HideNext = continuation
			case "encoded":
				target.EncodedJumps = map[*core.Node]bool{continuation: true}
			case "loop":
				target.IsCircle = true
			case "owned":
				root.RemoveNext(external)
				condition.AddNext(external)
			}
			manager := NewRootStatementManager(root)
			manager.DominatorMap = GenerateDominatorTree(root)
			splitSharedFallthroughExpression(manager, condition)
			shouldSplit := kind == "split" || kind == "reverse" || kind == "same protected range"
			if !shouldSplit {
				if condition.Next[0] != target || external.Next[0] != target {
					t.Fatal("unproved shared edge was changed")
				}
				return
			}
			clone := condition.Next[0]
			st, ok := clone.Statement.(*statements.ExpressionStatement)
			if clone == target || !ok || st == target.Statement || st.Expression != value ||
				clone.OriginPC != target.OriginPC || !clone.HasOriginPC ||
				len(clone.Next) != 1 || clone.Next[0] != continuation || external.Next[0] != target ||
				condition.Next[1] != continuation || len(target.Source) != 1 || target.Source[0] != external {
				t.Fatal("edge split lost expression, PC, successor order or original entrance")
			}
		})
	}
}
