package rewriter

import (
	"slices"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestProtectedIfBoundaryRetainsExactReferenceAndWholeEffectDomain(t *testing.T) {
	for _, scenario := range []string{"direct", "detached jump", "jump chain", "wrong original", "missing range", "invalid range", "ambiguous jump", "jump cycle", "protected condition", "protected branch", "unknown call PC", "opaque branch", "shared interval", "handler interval", "budget", "catch-all cleanup ownership", "missing handler metadata"} {
		t.Run(scenario, func(t *testing.T) {
			original := core.NewNode(&statements.ConditionStatement{})
			branch := layerTestCall(18)
			condition := values.NewBinaryExpression(layerTestRef("java.lang.Object"), values.JavaNull, values.NEQ, nil)
			view := statements.NewIfStatement(condition, []statements.Statement{statements.NewExpressionStatement(branch)}, nil)
			replacement := core.NewNode(view)
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
			switch scenario {
			case "detached jump":
				region.ProtectedEnd = jump
				original.RemoveAllSource() // the old jump retains its detached semantic reference below
				jump.Next = []*core.Node{original}
			case "jump chain":
				outer := core.NewNode(&statements.GOTOStatement{})
				outer.AddNext(jump)
				region.ProtectedEnd = outer
			case "wrong original":
				region.ProtectedEnd = core.NewNode(&statements.ConditionStatement{})
			case "missing range":
				region.HasProtectedRange = false
			case "invalid range":
				region.ProtectedEndPC = region.ProtectedStartPC
			case "ambiguous jump":
				region.ProtectedEnd = jump
				jump.AddNext(replacement)
			case "jump cycle":
				region.ProtectedEnd = jump
				jump.RemoveAllNext()
				jump.AddNext(jump)
			case "protected condition":
				view.Condition = layerTestCall(9)
			case "protected branch":
				branch.OriginPC = 9
			case "unknown call PC":
				branch.HasOriginPC = false
			case "opaque branch":
				view.IfBody = []statements.Statement{&statements.CustomStatement{}}
			case "shared interval":
				region.SharedProtectedRanges = []core.HandlerRange{{StartPc: 16, EndPc: 20}}
			case "handler interval":
				handler.CatchHandler.ProtectedRanges = append(handler.CatchHandler.ProtectedRanges, [2]int{16, 20})
			case "catch-all cleanup ownership":
				handler.CatchHandler.CatchAll = true
			case "missing handler metadata":
				handler.CatchHandler = nil
			case "budget":
				for i := 0; i < 512; i++ {
					view.IfBody = append(view.IfBody, statements.NewExpressionStatement(branch))
				}
			}
			before := region.ProtectedEnd
			next, sources := slices.Clone(jump.Next), slices.Clone(original.Source)
			retargetProtectedIfBoundary([]*core.Node{region}, original, replacement)
			want := scenario == "direct" || scenario == "detached jump" || scenario == "jump chain"
			if (region.ProtectedEnd == replacement) != want {
				t.Fatalf("retargeted=%v want=%v", region.ProtectedEnd == replacement, want)
			}
			if !want && region.ProtectedEnd != before || !slices.Equal(next, jump.Next) || !slices.Equal(sources, original.Source) {
				t.Fatal("proof changed an unrelated semantic reference or normal CFG edge")
			}
		})
	}
}
