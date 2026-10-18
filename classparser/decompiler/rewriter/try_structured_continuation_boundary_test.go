package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestTypedTryBoundaryKeepsAdjacentStructuredRegionOutsideOriginalHandlers(t *testing.T) {
	for _, variant := range []string{"adjacent region", "two inner handlers", "protected body", "protected handler", "protected initializer", "folded protected input", "missing ranges", "overlapping ranges", "spanning ranges", "missing entry", "protected entry", "opaque body", "cycle", "swallowed protected effect", "stale boundary"} {
		t.Run(variant, func(t *testing.T) {
			region := core.NewNode(&statements.MiddleStatement{Flag: "try"})
			region.HasProtectedRange = true
			region.ProtectedStartPC, region.ProtectedEndPC = 0, 10
			start := core.NewNode(&statements.ExpressionStatement{Expression: layerTestCall(3)})
			jump := core.NewNode(&statements.GOTOStatement{})
			bodyCall, handlerCall := layerTestCall(31), layerTestCall(41)
			inner := &statements.TryCatchStatement{
				TryBody:     []statements.Statement{statements.NewExpressionStatement(bodyCall)},
				Exception:   []*values.JavaRef{layerTestRef("java.io.IOException")},
				CatchBodies: [][]statements.Statement{{statements.NewExpressionStatement(handlerCall)}},
				Handlers:    []statements.CatchHandler{{EntryPC: 40, ProtectedRanges: [][2]int{{30, 35}}}},
			}
			tail := core.NewNode(inner)
			start.AddNext(jump)
			jump.AddNext(tail)
			region.ProtectedEnd = jump
			handler := core.NewNode(&statements.ReturnStatement{})
			handler.IsCatchStart = true
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 12, ProtectedRanges: [][2]int{{0, 10}}}
			region.AddNext(start)
			region.AddNext(handler)
			switch variant {
			case "two inner handlers":
				inner.Exception = append(inner.Exception, layerTestRef("java.lang.ReflectiveOperationException"))
				inner.CatchBodies = append(inner.CatchBodies, []statements.Statement{statements.NewExpressionStatement(layerTestCall(45))})
				inner.Handlers = append(inner.Handlers, statements.CatchHandler{EntryPC: 44, ProtectedRanges: [][2]int{{30, 35}}})
			case "protected body":
				bodyCall.OriginPC = 5
			case "protected handler":
				handlerCall.OriginPC = 5
			case "protected initializer":
				inner.EntryInitializers = []*statements.AssignStatement{{LeftValue: layerTestRef("java.lang.Object"), JavaValue: layerTestCall(5), IsFirst: true}}
			case "folded protected input":
				bodyCall.Arguments = []values.JavaValue{layerTestCall(5)}
			case "missing ranges":
				inner.Handlers[0].ProtectedRanges = nil
			case "overlapping ranges":
				inner.Handlers[0].ProtectedRanges = [][2]int{{5, 15}}
			case "spanning ranges":
				inner.Handlers[0].ProtectedRanges = [][2]int{{0, 35}}
			case "missing entry":
				inner.Handlers[0].EntryPC = -1
			case "protected entry":
				inner.Handlers[0].EntryPC = 5
			case "opaque body":
				inner.TryBody = []statements.Statement{&statements.CustomStatement{}}
			case "cycle":
				inner.TryBody = []statements.Statement{inner}
			case "swallowed protected effect":
				inner.Exception[0] = layerTestRef("java.lang.Throwable")
				inner.CatchBodies[0] = nil
				bodyCall.OriginPC = 5
			case "stale boundary":
				region.ProtectedEnd = core.NewNode(inner)
			}
			got := typedTryNormalBoundary(region, start, []*core.Node{start, handler}, GenerateDominatorTree(region))
			want := variant == "adjacent region" || variant == "two inner handlers"
			if (got == tail) != want {
				t.Fatalf("accepted=%v want=%v", got == tail, want)
			}
		})
	}
}
