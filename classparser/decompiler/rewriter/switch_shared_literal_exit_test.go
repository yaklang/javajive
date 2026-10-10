package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/omap"
)

func TestSharedSwitchLiteralDefaultRequiresTerminalOriginAndCoverage(t *testing.T) {
	for _, kind := range []string{"literal", "null", "same coverage", "different coverage", "missing source pc", "missing return pc", "wrong return pc", "reference", "invocation", "hidden", "encoded", "nonterminal", "loop", "owned", "catch"} {
		t.Run(kind, func(t *testing.T) {
			root, outside := jumpTestNode("root"), jumpTestNode("outside")
			owner := core.NewNode(&statements.MiddleStatement{Flag: "switch"})
			owner.OriginPC, owner.HasOriginPC = 10, true
			first, second := jumpTestNode("first"), jumpTestNode("second")
			first.OriginPC, first.HasOriginPC = 12, true
			second.OriginPC, second.HasOriginPC = 15, true
			value := values.JavaValue(values.NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean)))
			if kind == "null" {
				value = values.JavaNull
			} else if kind == "reference" {
				value = values.NewJavaRef(nil, nil, types.NewJavaClass("java.lang.Object"))
			} else if kind == "invocation" {
				value = &values.FunctionCallExpression{}
			}
			ret := &statements.ReturnStatement{JavaValue: value, OriginPC: 30, HasOriginPC: true}
			target := core.NewNode(ret)
			target.OriginPC, target.HasOriginPC = 30, true
			root.AddNext(owner)
			root.AddNext(outside)
			outside.AddNext(target)
			owner.AddNext(first)
			owner.AddNext(second)
			owner.AddNext(target)
			owner.SwitchDefault = target
			owner.SwitchCases = omap.NewEmptyOrderedMap[int, *core.Node]()
			owner.SwitchCases.Set(1, first)
			owner.SwitchCases.Set(2, second)
			// Real fall-through must not be replaced by a terminal leaf.
			first.AddNext(second)
			second.AddNext(target)
			switch kind {
			case "same coverage", "different coverage":
				root.HasProtectedRange = true
				root.ProtectedStartPC, root.ProtectedEndPC = 10, 40
				if kind == "different coverage" {
					root.ProtectedEndPC = 30
				}
			case "missing source pc":
				owner.HasOriginPC, second.HasOriginPC = false, false
			case "missing return pc":
				ret.HasOriginPC = false
			case "wrong return pc":
				ret.OriginPC++
			case "hidden":
				target.HideNext = outside
			case "encoded":
				owner.EncodedJumps = map[*core.Node]bool{target: true}
				second.EncodedJumps = map[*core.Node]bool{target: true}
			case "nonterminal":
				target.AddNext(jumpTestNode("cleanup"))
			case "loop":
				target.IsInCircle = true
			case "owned":
				root.RemoveNext(outside)
			case "catch":
				target.IsCatchStart = true
			}
			manager := NewRootStatementManager(root)
			manager.DominatorMap = GenerateDominatorTree(root)
			splitExternalSharedSwitchReturns(manager, owner)
			accept := kind == "literal" || kind == "null" || kind == "same coverage"
			if !accept {
				if owner.SwitchDefault != target || second.Next[0] != target {
					t.Fatal("copied an unproved shared value return")
				}
				return
			}
			if outside.Next[0] != target || first.Next[0] != second || owner.Next[0] != first || owner.Next[1] != second {
				t.Fatal("changed outside entrance, fall-through or case order")
			}
			for _, leaf := range []*core.Node{owner.SwitchDefault, second.Next[0]} {
				copy, ok := leaf.Statement.(*statements.ReturnStatement)
				if leaf == target || !ok || copy == ret || copy.JavaValue != value || !copy.HasOriginPC || copy.OriginPC != 30 || !leaf.HasOriginPC || leaf.OriginPC != 30 {
					t.Fatal("lost the terminal value or exact return-PC witness")
				}
			}
			if owner.Next[2] != owner.SwitchDefault || owner.SwitchDefault == second.Next[0] {
				t.Fatal("default must keep its successor position and independent ownership")
			}
		})
	}
}
