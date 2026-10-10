package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	valueutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"slices"
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
			handler := core.NewNode(&statements.ReturnStatement{})
			handler.IsCatchStart = true
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 12, ProtectedRanges: [][2]int{{0, 10}}}
			region.AddNext(start)
			region.AddNext(handler)
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
			got := typedTryNormalBoundary(region, start, next, GenerateDominatorTree(region))
			if (got == tail) != (variant == "outside effect") {
				t.Fatalf("boundary=%p tail=%p", got, tail)
			}
		})
	}
}

func TestTypedTryBoundaryPreservesHandlerAndNormalControlOwnership(t *testing.T) {
	for _, scenario := range []string{"private return", "private throw", "empty catch", "shared fallback", "conditional fallback", "retry backedge", "catch cycle", "unknown sink", "unproved throw", "terminal with live edge", "normal return", "normal break goto", "missing dominators", "external handler entry"} {
		t.Run(scenario, func(t *testing.T) {
			root := core.NewNode(&statements.MiddleStatement{})
			region := core.NewNode(&statements.MiddleStatement{Flag: "try"})
			region.HasProtectedRange = true
			region.ProtectedStartPC, region.ProtectedEndPC = 0, 10
			start := core.NewNode(&statements.ExpressionStatement{Expression: layerTestCall(3)})
			tail := core.NewNode(&statements.ExpressionStatement{Expression: layerTestCall(18)})
			handler := core.NewNode(&statements.MiddleStatement{})
			handler.IsCatchStart = true
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 12, ProtectedRanges: [][2]int{{0, 10}}}
			terminal := core.NewNode(&statements.ReturnStatement{})
			root.AddNext(region)
			region.AddNext(start)
			region.AddNext(handler)
			start.AddNext(tail)
			handler.AddNext(terminal)
			region.ProtectedEnd = tail
			thrown := statements.NewCustomStatement(func(*class_context.ClassContext) string { return "throw caught" }, func(*valueutils.VariableId, *valueutils.VariableId) {})
			thrown.ThrownValue = layerTestRef("java.lang.IllegalArgumentException")
			thrown.HasOriginPC, thrown.OriginPC = true, 14
			switch scenario {
			case "private throw":
				terminal.Statement = thrown
			case "empty catch":
				handler.RemoveAllNext()
			case "shared fallback":
				root.AddNext(terminal)
			case "conditional fallback":
				condition := core.NewNode(&statements.ConditionStatement{})
				handler.ReplaceNext(terminal, condition)
				condition.AddNext(terminal)
				condition.AddNext(tail)
			case "retry backedge":
				handler.ReplaceNext(terminal, region)
			case "catch cycle":
				handler.ReplaceNext(terminal, handler)
			case "unknown sink":
				terminal.Statement = &statements.MiddleStatement{}
			case "unproved throw":
				terminal.Statement = thrown
				thrown.ThrownValue = nil
			case "terminal with live edge":
				terminal.AddNext(tail)
			case "normal return":
				tail.Statement = &statements.ReturnStatement{}
			case "normal break goto":
				jump := core.NewNode(&statements.GOTOStatement{})
				start.ReplaceNext(tail, jump)
				jump.AddNext(tail)
				region.ProtectedEnd = jump
				tail.Statement = &statements.ReturnStatement{}
			case "external handler entry":
				root.AddNext(terminal)
			}
			dom := GenerateDominatorTree(root)
			if scenario == "missing dominators" {
				dom = nil
			}
			originalNext, originalSource := slices.Clone(handler.Next), slices.Clone(handler.Source)
			got := typedTryNormalBoundary(region, start, []*core.Node{start, handler}, dom)
			want := scenario == "private return" || scenario == "private throw"
			if (got == tail) != want {
				t.Fatalf("boundary accepted=%v want=%v", got == tail, want)
			}
			if !slices.Equal(originalNext, handler.Next) || !slices.Equal(originalSource, handler.Source) || region.ProtectedEnd == nil {
				t.Fatal("proof mutated original graph")
			}
		})
	}
}
