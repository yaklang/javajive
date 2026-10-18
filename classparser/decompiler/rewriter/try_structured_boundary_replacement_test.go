package rewriter

import (
	"slices"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestProtectedTryBoundaryRetargetsOnlyExactDetachedOriginalEntry(t *testing.T) {
	for _, variant := range []string{"direct", "detached jump", "wrong original", "missing original range", "wrong entry kind", "protected body", "protected catch", "protected initializer", "overlapping handler range", "missing handler range", "unknown effect", "swallowed protected effect", "catchall parent", "missing parent metadata"} {
		t.Run(variant, func(t *testing.T) {
			original := core.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
			original.HasProtectedRange = true
			original.ProtectedStartPC, original.ProtectedEndPC = 30, 35
			bodyCall, handlerCall := layerTestCall(31), layerTestCall(41)
			inner := &statements.TryCatchStatement{
				TryBody:     []statements.Statement{statements.NewExpressionStatement(bodyCall)},
				Exception:   []*values.JavaRef{layerTestRef("java.io.IOException")},
				CatchBodies: [][]statements.Statement{{statements.NewExpressionStatement(handlerCall)}},
				Handlers:    []statements.CatchHandler{{EntryPC: 40, ProtectedRanges: [][2]int{{30, 35}}}},
			}
			replacement := core.NewNode(inner)
			region := core.NewNode(&statements.MiddleStatement{})
			region.HasProtectedRange = true
			region.ProtectedStartPC, region.ProtectedEndPC = 0, 10
			region.ProtectedEnd = original
			handler := core.NewNode(&statements.ReturnStatement{})
			handler.IsCatchStart = true
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 12, ProtectedRanges: [][2]int{{0, 10}}}
			region.AddNext(handler)
			jump := core.NewNode(&statements.GOTOStatement{})
			jump.AddNext(original)
			switch variant {
			case "detached jump":
				region.ProtectedEnd = jump
				original.RemoveAllSource()
				jump.Next = []*core.Node{original}
			case "wrong original":
				region.ProtectedEnd = core.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
			case "missing original range":
				original.HasProtectedRange = false
			case "wrong entry kind":
				original.Statement = &statements.ConditionStatement{}
			case "protected body":
				bodyCall.OriginPC = 5
			case "protected catch":
				handlerCall.OriginPC = 5
			case "protected initializer":
				inner.EntryInitializers = []*statements.AssignStatement{{LeftValue: layerTestRef("java.lang.Object"), JavaValue: layerTestCall(5), IsFirst: true}}
			case "overlapping handler range":
				inner.Handlers[0].ProtectedRanges = [][2]int{{5, 35}}
			case "missing handler range":
				inner.Handlers[0].ProtectedRanges = nil
			case "unknown effect":
				bodyCall.HasOriginPC = false
			case "swallowed protected effect":
				inner.Exception[0] = layerTestRef("java.lang.Throwable")
				inner.CatchBodies[0] = nil
				bodyCall.OriginPC = 5
			case "catchall parent":
				handler.CatchHandler.CatchAll = true
			case "missing parent metadata":
				handler.CatchHandler = nil
			}
			before := region.ProtectedEnd
			next, sources := slices.Clone(jump.Next), slices.Clone(original.Source)
			retargetProtectedTryBoundary([]*core.Node{region}, original, replacement)
			want := variant == "direct" || variant == "detached jump"
			if (region.ProtectedEnd == replacement) != want || !want && region.ProtectedEnd != before {
				t.Fatalf("retargeted=%v want=%v", region.ProtectedEnd == replacement, want)
			}
			if !slices.Equal(next, jump.Next) || !slices.Equal(sources, original.Source) {
				t.Fatal("semantic-boundary retargeting changed normal CFG edges")
			}
		})
	}
}
