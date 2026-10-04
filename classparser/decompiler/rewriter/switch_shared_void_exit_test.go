package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/omap"
)

func TestSharedSwitchVoidReturnSplitKeepsExitOwnership(t *testing.T) {
	for _, kind := range []string{"void", "value", "nonterminal", "owned", "unshared", "fallthrough", "direct grouped label"} {
		t.Run(kind, func(t *testing.T) {
			root, external := jumpTestNode("root"), jumpTestNode("external")
			owner := core.NewNode(&statements.MiddleStatement{Flag: "switch"})
			first, second := jumpTestNode("first()"), jumpTestNode("second()")
			ret := &statements.ReturnStatement{}
			target := core.NewNode(ret)
			target.OriginPC, target.HasOriginPC = 30, true
			root.AddNext(owner)
			owner.AddNext(first)
			owner.AddNext(second)
			first.AddNext(target)
			second.AddNext(target)
			root.AddNext(external)
			external.AddNext(target)
			switch kind {
			case "value":
				ret.JavaValue = &values.FunctionCallExpression{FunctionName: "effect", FuncType: &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaInteger)}}
			case "nonterminal":
				target.AddNext(jumpTestNode("cleanup()"))
			case "owned":
				root.RemoveNext(external)
				owner.AddNext(external)
			case "unshared":
				external.RemoveNext(target)
				second.RemoveNext(target)
			case "fallthrough":
				first.RemoveNext(target)
				first.AddNext(second)
			case "direct grouped label":
				owner.AddNext(target)
				owner.SwitchCases = omap.NewEmptyOrderedMap[int, *core.Node]()
				owner.SwitchCases.Set(0, target)
				owner.SwitchCases.Set(1, target)
				owner.SwitchDefault = second
			}
			manager := NewRootStatementManager(root)
			manager.DominatorMap = GenerateDominatorTree(root)
			splitExternalSharedSwitchReturns(manager, owner)
			accepted := kind == "void" || kind == "fallthrough" || kind == "direct grouped label"
			if !accepted {
				if first.Next[0] != target {
					t.Fatal("split without proof of an externally shared terminal void return")
				}
				return
			}
			if external.Next[0] != target || owner.Next[0] != first || owner.Next[1] != second || second.Next[0] == target {
				t.Fatal("changed another entrance, case order, or failed to isolate the return")
			}
			leaf := second.Next[0]
			if copied, ok := leaf.Statement.(*statements.ReturnStatement); !ok || copied.JavaValue != nil || leaf.OriginPC != 30 || !leaf.HasOriginPC {
				t.Fatal("copied a value or lost the return's bytecode witness")
			}
			if kind == "fallthrough" && first.Next[0] != second {
				t.Fatal("changed a real case-to-case fall-through")
			}
			if kind == "direct grouped label" {
				a, b := owner.SwitchCases.GetMust(0), owner.SwitchCases.GetMust(1)
				if a == target || a != b || owner.Next[2] != a || owner.SwitchDefault != second {
					t.Fatal("lost grouped label identity or successor position")
				}
			}
		})
	}
}

func TestExternalSwitchContinuationRequiresUniqueForwardNonLabelJoin(t *testing.T) {
	for _, scenario := range []string{"shared", "fallthrough", "grouped only", "owned", "label", "backward", "missing PC", "multiple boundaries"} {
		t.Run(scenario, func(t *testing.T) {
			root, external := jumpTestNode("root"), jumpTestNode("external")
			owner := core.NewNode(&statements.MiddleStatement{Flag: "switch"})
			owner.OriginPC = 10
			owner.HasOriginPC = true
			first, second, third := jumpTestNode("first"), jumpTestNode("second"), jumpTestNode("third")
			target := jumpTestNode("tail")
			target.OriginPC = 40
			target.HasOriginPC = true
			root.AddNext(owner)
			root.AddNext(external)
			external.AddNext(target)
			owner.AddNext(first)
			owner.AddNext(second)
			owner.AddNext(third)
			first.AddNext(target)
			second.AddNext(target)
			third.AddNext(target)
			starts := []*core.Node{first, second, third}
			switch scenario {
			case "fallthrough":
				first.RemoveNext(target)
				first.AddNext(second)
			case "grouped only":
				starts = []*core.Node{first, first}
			case "owned":
				root.RemoveNext(external)
			case "label":
				starts = append(starts, target)
				owner.AddNext(target)
			case "backward":
				target.OriginPC = 5
			case "missing PC":
				target.HasOriginPC = false
			case "multiple boundaries":
				other := jumpTestNode("other")
				other.OriginPC = 50
				other.HasOriginPC = true
				external.AddNext(other)
				first.AddNext(other)
				second.AddNext(other)
			}
			manager := NewRootStatementManager(root)
			manager.DominatorMap = GenerateDominatorTree(root)
			got := externalSharedSwitchContinuation(manager, owner, starts)
			want := scenario == "shared" || scenario == "fallthrough"
			if (got == target) != want || (!want && got != nil) {
				t.Fatalf("continuation=%p want=%v", got, want)
			}
			if external.Next[0] != target || (scenario == "fallthrough" && first.Next[0] != second) {
				t.Fatal("changed external path or real fallthrough")
			}
		})
	}
}
