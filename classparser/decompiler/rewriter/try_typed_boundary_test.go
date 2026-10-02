package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"testing"
)

func TestTypedTryBoundaryRequiresReachableOriginalWholeEffectDomain(t *testing.T) {
	for _, variant := range []string{"outside effect", "missing region", "wrong interval", "catchall", "multiple handlers", "stale boundary", "protected folded input", "unknown effect origin", "ambiguous jump", "jump cycle", "opaque statement"} {
		t.Run(variant, func(t *testing.T) {
			region := core.NewNode(&statements.MiddleStatement{Flag: "try"})
			region.HasProtectedRange = true
			region.ProtectedStartPC, region.ProtectedEndPC = 0, 10
			start := core.NewNode(&statements.ExpressionStatement{Expression: layerTestCall(3)})
			jump := core.NewNode(&statements.GOTOStatement{})
			effect := layerTestCall(18)
			tail := core.NewNode(&statements.ExpressionStatement{Expression: effect})
			start.AddNext(jump)
			jump.AddNext(tail)
			region.ProtectedEnd = jump
			handler := core.NewNode(&statements.MiddleStatement{})
			handler.IsCatchStart = true
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 12, ProtectedRanges: [][2]int{{0, 10}}}
			next := []*core.Node{start, handler}
			switch variant {
			case "missing region":
				region.HasProtectedRange = false
			case "wrong interval":
				handler.CatchHandler.ProtectedRanges[0][1] = 11
			case "catchall":
				handler.CatchHandler.CatchAll = true
			case "multiple handlers":
				next = append(next, handler)
			case "stale boundary":
				region.ProtectedEnd = core.NewNode(&statements.ExpressionStatement{Expression: layerTestCall(18)})
			case "protected folded input":
				effect.Arguments = []values.JavaValue{layerTestCall(9)}
			case "unknown effect origin":
				effect.HasOriginPC = false
			case "ambiguous jump":
				jump.AddNext(handler)
			case "jump cycle":
				jump.RemoveAllNext()
				jump.AddNext(jump)
			case "opaque statement":
				tail.Statement = &statements.CustomStatement{}
			}
			got := typedTryNormalBoundary(region, start, next)
			if (got == tail) != (variant == "outside effect") {
				t.Fatalf("boundary=%p tail=%p", got, tail)
			}
		})
	}
}
