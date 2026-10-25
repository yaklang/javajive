package rewriter

import (
	"slices"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestTypedTryConditionalContinuationRequiresClosedOutsideEffects(t *testing.T) {
	for _, variant := range []string{"outside branch", "detached jump", "nested outside branch", "protected condition", "protected arm", "protected later continuation", "unknown effect", "opaque arm", "stale boundary", "nonterminating handler", "handler enters continuation", "catchall", "encoded transfer", "outside cycle", "protected cycle", "oversized tree"} {
		t.Run(variant, func(t *testing.T) {
			region := core.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
			region.HasProtectedRange = true
			region.ProtectedStartPC, region.ProtectedEndPC = 0, 10
			start := core.NewNode(statements.NewExpressionStatement(layerTestCall(3)))
			branchCall := layerTestCall(20)
			condition := values.NewBinaryExpression(layerTestRef("java.lang.Object"), values.JavaNull, values.NEQ, nil)
			branch := statements.NewIfStatement(condition, []statements.Statement{statements.NewExpressionStatement(branchCall)}, nil)
			tail := core.NewNode(branch)
			laterCall := layerTestCall(35)
			later := core.NewNode(statements.NewExpressionStatement(laterCall))
			handler := core.NewNode(&statements.ReturnStatement{})
			handler.IsCatchStart = true
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 12, ProtectedRanges: [][2]int{{0, 10}}}
			region.AddNext(start)
			region.AddNext(handler)
			start.AddNext(tail)
			tail.AddNext(later)
			region.ProtectedEnd = tail
			switch variant {
			case "detached jump":
				jump := core.NewNode(&statements.GOTOStatement{})
				jump.Next = []*core.Node{tail}
				region.ProtectedEnd = jump
			case "nested outside branch":
				branch.IfBody = []statements.Statement{statements.NewIfStatement(condition, branch.IfBody, nil)}
			case "protected condition":
				branch.Condition = layerTestCall(9)
			case "protected arm":
				branchCall.OriginPC = 9
			case "protected later continuation":
				laterCall.OriginPC = 9
			case "unknown effect":
				branchCall.HasOriginPC = false
			case "opaque arm":
				branch.IfBody = append(branch.IfBody, &statements.CustomStatement{})
			case "stale boundary":
				region.ProtectedEnd = core.NewNode(branch)
			case "nonterminating handler":
				handler.Statement = &statements.MiddleStatement{}
			case "handler enters continuation":
				handler.Statement = statements.NewExpressionStatement(layerTestCall(14))
				handler.AddNext(tail)
			case "catchall":
				handler.CatchHandler.CatchAll = true
			case "encoded transfer":
				tail.EncodedJumps = map[*core.Node]bool{later: true}
			case "outside cycle":
				later.AddNext(tail)
			case "protected cycle":
				later.AddNext(start)
			case "oversized tree":
				for i := 0; i < 512; i++ {
					branch.IfBody = append(branch.IfBody, statements.NewExpressionStatement(branchCall))
				}
			}
			before, sources := slices.Clone(tail.Next), slices.Clone(tail.Source)
			got := typedTryNormalBoundary(region, start, region.Next, GenerateDominatorTree(region))
			want := variant == "outside branch" || variant == "detached jump" || variant == "nested outside branch" || variant == "outside cycle"
			if (got == tail) != want {
				t.Fatalf("outside conditional accepted=%v want=%v", got == tail, want)
			}
			if !slices.Equal(before, tail.Next) || !slices.Equal(sources, tail.Source) {
				t.Fatal("proof mutated normal edges")
			}
		})
	}
}
