package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestSharedLiteralReturnRequiresTerminalOriginAndCoverage(t *testing.T) {
	for _, kind := range []string{"literal", "null", "same coverage", "different coverage", "missing pc", "wrong pc", "reference", "invocation", "hidden", "encoded", "nonterminal", "loop", "owned"} {
		t.Run(kind, func(t *testing.T) {
			root, external, tail := jumpTestNode("root"), jumpTestNode("external"), jumpTestNode("tail")
			condition := core.NewNode(&statements.ConditionStatement{})
			value := values.JavaValue(values.NewJavaLiteral("", types.NewJavaPrimer(types.JavaString)))
			if kind == "null" {
				value = values.JavaNull
			} else if kind == "reference" {
				value = values.NewJavaRef(nil, nil, types.NewJavaClass("java.lang.String"))
			} else if kind == "invocation" {
				value = &values.FunctionCallExpression{}
			}
			ret := &statements.ReturnStatement{JavaValue: value, OriginPC: 20, HasOriginPC: true}
			target := core.NewNode(ret)
			condition.OriginPC, condition.HasOriginPC = 10, true
			target.OriginPC, target.HasOriginPC = 20, true
			root.AddNext(condition)
			root.AddNext(external)
			condition.AddNext(target)
			condition.AddNext(tail)
			external.AddNext(target)
			switch kind {
			case "same coverage", "different coverage":
				root.HasProtectedRange = true
				root.ProtectedStartPC, root.ProtectedEndPC = 10, 30
				if kind == "different coverage" {
					root.ProtectedEndPC = 20
				}
			case "missing pc":
				ret.HasOriginPC = false
			case "wrong pc":
				ret.OriginPC = 21
			case "hidden":
				target.HideNext = tail
			case "encoded":
				condition.EncodedJumps = map[*core.Node]bool{target: true}
			case "nonterminal":
				target.AddNext(tail)
			case "loop":
				target.IsInCircle = true
			case "owned":
				root.RemoveNext(external)
				condition.AddNext(external)
			}
			manager := NewRootStatementManager(root)
			manager.DominatorMap = GenerateDominatorTree(root)
			splitSharedTerminalLeaves(manager, condition)
			accept := kind == "literal" || kind == "null" || kind == "same coverage"
			if !accept {
				if condition.Next[0] != target {
					t.Fatal("split an unproved value return")
				}
				return
			}
			private := condition.Next[0]
			copy, ok := private.Statement.(*statements.ReturnStatement)
			if private == target || !ok || copy == ret || copy.JavaValue != value ||
				copy.OriginPC != 20 || !copy.HasOriginPC || external.Next[0] != target ||
				condition.Next[1] != tail || len(target.Source) != 1 {
				t.Fatal("literal, terminal PC, predecessor or branch polarity changed")
			}
		})
	}
}
