package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"slices"
	"testing"
)

func TestCollapsedConditionBoundaryRequiresExactConsumerAndWholeDomain(t *testing.T) {
	for _, kind := range []string{"assignment", "expression", "detached jump", "structural tree", "duplicate converging edges", "unmatched structural tree", "repeated structural condition", "unknown structural value", "protected condition", "protected arm", "unknown effect", "opaque arm", "cycle", "foreign consumer", "missing callback", "multiple successors", "wrong successor", "wrong original", "ambiguous jump", "jump cycle", "missing interval", "invalid interval", "catch-all", "unknown handler", "shared interval", "handler interval", "control transfer", "budget"} {
		t.Run(kind, func(t *testing.T) {
			original := core.NewNode(&statements.ConditionStatement{Condition: layerTestCall(18), Callback: func(values.JavaValue) {}})
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaLong))
			condition := layerTestCall(18)
			arm := layerTestCall(22)
			ternary := values.NewTernaryExpression(condition, arm, values.NewJavaLiteral(int64(7), ref.Type()))
			replacement := core.NewNode(statements.NewAssignStatement(ref, ternary, false))
			replacement.SourceConditionNode = original
			original.AddNext(replacement)
			region := core.NewNode(&statements.MiddleStatement{})
			region.HasProtectedRange = true
			region.ProtectedStartPC, region.ProtectedEndPC = 0, 10
			region.ProtectedEnd = original
			handler := core.NewNode(&statements.ReturnStatement{})
			handler.IsCatchStart = true
			handler.CatchHandler = &statements.CatchHandler{ProtectedRanges: [][2]int{{0, 10}}}
			region.AddNext(handler)
			jump := core.NewNode(&statements.GOTOStatement{})
			jump.AddNext(original)
			switch kind {
			case "expression":
				replacement.Statement = statements.NewExpressionStatement(ternary)
			case "detached jump":
				region.ProtectedEnd = jump
				original.RemoveSource(jump)
				jump.Next = []*core.Node{original}
			case "structural tree", "duplicate converging edges", "unmatched structural tree", "repeated structural condition", "unknown structural value":
				replacement.SourceConditionNode = nil
				original.Statement.(*statements.ConditionStatement).TernaryChainArm = true
				original.Statement.(*statements.ConditionStatement).Condition = condition
				if kind == "duplicate converging edges" {
					original.Next = append(original.Next, replacement)
				}
				if kind == "unmatched structural tree" {
					original.Statement.(*statements.ConditionStatement).Condition = layerTestCall(18)
				}
				if kind == "repeated structural condition" {
					ternary.FalseValue = values.NewTernaryExpression(condition, values.JavaNull, values.JavaNull)
				}
				if kind == "unknown structural value" {
					ternary.FalseValue = &values.CustomValue{CapturesKnown: true, Flag: "boolean_stack_word"}
				}
			case "protected condition":
				condition.OriginPC = 9
			case "protected arm":
				arm.OriginPC = 9
			case "unknown effect":
				arm.HasOriginPC = false
			case "opaque arm":
				ternary.FalseValue = &values.CustomValue{}
			case "cycle":
				ternary.FalseValue = ternary
			case "foreign consumer":
				replacement.SourceConditionNode = core.NewNode(&statements.ConditionStatement{})
			case "missing callback":
				original.Statement.(*statements.ConditionStatement).Callback = nil
			case "multiple successors":
				original.AddNext(handler)
			case "wrong successor":
				original.RemoveNext(replacement)
				original.AddNext(handler)
			case "wrong original":
				region.ProtectedEnd = core.NewNode(&statements.ConditionStatement{})
			case "ambiguous jump":
				region.ProtectedEnd = jump
				jump.AddNext(handler)
			case "jump cycle":
				region.ProtectedEnd = jump
				jump.RemoveNext(original)
				jump.AddNext(jump)
			case "missing interval":
				region.HasProtectedRange = false
			case "invalid interval":
				region.ProtectedEndPC = 0
			case "catch-all":
				handler.CatchHandler.CatchAll = true
			case "unknown handler":
				handler.CatchHandler = nil
			case "shared interval":
				region.SharedProtectedRanges = []core.HandlerRange{{StartPc: 16, EndPc: 24}}
			case "handler interval":
				handler.CatchHandler.ProtectedRanges = append(handler.CatchHandler.ProtectedRanges, [2]int{16, 24})
			case "control transfer":
				replacement.Statement = statements.NewReturnStatement(ternary)
			case "budget":
				nested := values.JavaValue(arm)
				for i := 0; i < 513; i++ {
					nested = values.NewTernaryExpression(condition, nested, values.JavaNull)
				}
				ternary.TrueValue = nested
			}
			before := region.ProtectedEnd
			next, sources := slices.Clone(original.Next), slices.Clone(replacement.Source)
			RetargetCollapsedConditionBoundary([]*core.Node{region}, original, replacement)
			want := kind == "assignment" || kind == "expression" || kind == "detached jump" || kind == "structural tree" || kind == "duplicate converging edges"
			if (region.ProtectedEnd == replacement) != want {
				t.Fatalf("retargeted=%v", region.ProtectedEnd == replacement)
			}
			if !want && region.ProtectedEnd != before || !slices.Equal(next, original.Next) || !slices.Equal(sources, replacement.Source) {
				t.Fatal("proof mutated normal CFG or unrelated semantic references")
			}
		})
	}
}
