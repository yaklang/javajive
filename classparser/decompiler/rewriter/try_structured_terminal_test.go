package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"slices"
	"testing"
)

// Return/throw arms are already structured when try collection runs. An
// original normal effect outside the table must not enter the typed catch's
// domain merely because terminal branch nodes no longer exist in the CFG.
func TestTypedTryBoundaryPreservesStructuredHandlerTerminationAndOriginalCatchDomain(t *testing.T) {
	for _, variant := range []string{"return throw", "nested", "effects before exit", "empty true", "empty false", "fallthrough effect", "wrong return origin", "wrong throw origin", "opaque throw", "hidden break", "opaque prior effect", "unknown predicate origin", "live normal successor", "external handler entry", "cycle", "depth", "budget"} {
		t.Run(variant, func(t *testing.T) {
			root := core.NewNode(&statements.MiddleStatement{})
			region := core.NewNode(&statements.MiddleStatement{Flag: "try"})
			region.HasProtectedRange = true
			region.ProtectedStartPC = 0
			region.ProtectedEndPC = 10
			start := core.NewNode(statements.NewExpressionStatement(layerTestCall(3)))
			tail := core.NewNode(statements.NewExpressionStatement(layerTestCall(18)))
			handler := core.NewNode(&statements.MiddleStatement{})
			handler.IsCatchStart = true
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 12, ProtectedRanges: [][2]int{{0, 10}}}
			ret := &statements.ReturnStatement{OriginPC: 14, HasOriginPC: true}
			thrown := statements.NewThrowStatement(layerTestRef("java.lang.IllegalStateException"))
			thrown.OriginPC = 16
			thrown.HasOriginPC = true
			condition := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
			branch := statements.NewIfStatement(condition, []statements.Statement{ret}, []statements.Statement{thrown})
			terminal := core.NewNode(branch)
			root.AddNext(region)
			region.AddNext(start)
			region.AddNext(handler)
			start.AddNext(tail)
			handler.AddNext(terminal)
			region.ProtectedEnd = tail
			switch variant {
			case "nested":
				branch.IfBody = []statements.Statement{statements.NewIfStatement(condition, []statements.Statement{ret}, []statements.Statement{thrown})}
			case "effects before exit":
				branch.IfBody = append([]statements.Statement{statements.NewExpressionStatement(layerTestCall(13))}, branch.IfBody...)
			case "empty true":
				branch.IfBody = nil
			case "empty false":
				branch.ElseBody = nil
			case "fallthrough effect":
				branch.ElseBody = []statements.Statement{statements.NewExpressionStatement(layerTestCall(16))}
			case "wrong return origin":
				ret.HasOriginPC = false
			case "wrong throw origin":
				thrown.HasOriginPC = false
			case "opaque throw":
				x := &statements.CustomStatement{ThrownValue: thrown.ThrownValue, OriginPC: 16, HasOriginPC: true}
				branch.ElseBody = []statements.Statement{x}
			case "hidden break":
				branch.IfBody = append([]statements.Statement{statements.NewSourceTransferStatement("break", "LOOP")}, branch.IfBody...)
			case "opaque prior effect":
				branch.IfBody = append([]statements.Statement{&statements.CustomStatement{}}, branch.IfBody...)
			case "unknown predicate origin":
				c := layerTestCall(13)
				c.HasOriginPC = false
				branch.Condition = c
			case "live normal successor":
				terminal.AddNext(tail)
			case "external handler entry":
				root.AddNext(terminal)
			case "cycle":
				branch.IfBody = []statements.Statement{branch}
			case "depth":
				for i := 0; i < 25; i++ {
					branch.IfBody = []statements.Statement{statements.NewIfStatement(condition, branch.IfBody, []statements.Statement{thrown})}
				}
			case "budget":
				for i := 0; i < 512; i++ {
					branch.IfBody = append([]statements.Statement{statements.NewExpressionStatement(layerTestCall(13))}, branch.IfBody...)
				}
			}
			beforeNext, beforeSource := slices.Clone(handler.Next), slices.Clone(handler.Source)
			result := typedTryNormalBoundary(region, start, []*core.Node{start, handler}, GenerateDominatorTree(root))
			want := variant == "return throw" || variant == "nested" || variant == "effects before exit"
			if (result == tail) != want {
				t.Fatalf("normal boundary accepted=%t want=%t", result == tail, want)
			}
			if !slices.Equal(beforeNext, handler.Next) || !slices.Equal(beforeSource, handler.Source) || region.ProtectedEnd != tail {
				t.Fatal("proof mutated graph ownership")
			}
		})
	}
}
