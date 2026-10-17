package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"slices"
	"testing"
)

func TestTypedProtectedReturnKeepsOriginalBoundaryBeforeIfCollection(t *testing.T) {
	for _, variant := range []string{"outside return", "pure return", "protected operand", "mixed operand", "unknown operand origin", "opaque operand", "operand cycle", "missing return origin", "missing node origin", "changed return PC", "protected return PC", "encoded transfer", "live return successor", "stale boundary", "foreign condition", "unknown condition origin", "outside condition", "unprotected condition effect", "catchall", "nonterminal handler", "missing dominators"} {
		t.Run(variant, func(t *testing.T) {
			region := core.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
			region.HasProtectedRange = true
			region.ProtectedStartPC, region.ProtectedEndPC = 0, 10
			condition := core.NewNode(&statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))})
			condition.OriginPC, condition.HasOriginPC = 4, true
			jump := core.NewNode(&statements.GOTOStatement{})
			call := layerTestCall(24)
			ret := &statements.ReturnStatement{JavaValue: call, OriginPC: 27, HasOriginPC: true}
			tail := core.NewNode(ret)
			tail.OriginPC, tail.HasOriginPC = 27, true
			handler := core.NewNode(&statements.ReturnStatement{})
			handler.IsCatchStart = true
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 12, ProtectedRanges: [][2]int{{0, 10}}}
			region.AddNext(condition)
			region.AddNext(handler)
			condition.AddNext(jump)
			jump.AddNext(tail)
			region.ProtectedEnd = jump
			switch variant {
			case "pure return":
				ret.JavaValue = values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger))
			case "protected operand":
				call.OriginPC = 7
			case "mixed operand":
				call.Arguments = []values.JavaValue{layerTestCall(7)}
			case "unknown operand origin":
				call.HasOriginPC = false
			case "opaque operand":
				ret.JavaValue = &values.CustomValue{}
			case "operand cycle":
				call.Arguments = []values.JavaValue{call}
			case "missing return origin":
				ret.HasOriginPC = false
			case "missing node origin":
				tail.HasOriginPC = false
			case "changed return PC":
				ret.OriginPC++
			case "protected return PC":
				ret.OriginPC = 7
				tail.OriginPC = 7
			case "encoded transfer":
				tail.EncodedJumps = map[*core.Node]bool{handler: true}
			case "live return successor":
				tail.AddNext(core.NewNode(statements.NewExpressionStatement(layerTestCall(28))))
			case "stale boundary":
				region.ProtectedEnd = core.NewNode(&statements.ReturnStatement{JavaValue: call, OriginPC: 27, HasOriginPC: true})
			case "unknown condition origin":
				condition.HasOriginPC = false
			case "outside condition":
				condition.OriginPC = 24
			case "unprotected condition effect":
				condition.Statement.(*statements.ConditionStatement).Condition = layerTestCall(24)
			case "catchall":
				handler.CatchHandler.CatchAll = true
			case "nonterminal handler":
				handler.Statement = &statements.MiddleStatement{}
			}
			manager := NewRootStatementManager(region)
			manager.TryNodes = []*core.Node{region}
			manager.DominatorMap = GenerateDominatorTree(region)
			if variant == "foreign condition" {
				copy := core.NewNode(condition.Statement)
				copy.OriginPC, copy.HasOriginPC = 4, true
				condition = copy
			}
			if variant == "missing dominators" {
				manager.DominatorMap = nil
			}
			beforeNext, beforeSource := slices.Clone(tail.Next), slices.Clone(tail.Source)
			got := typedProtectedIfCollectionStops(manager, condition)
			want := variant == "outside return" || variant == "pure return"
			if got[tail] != want {
				t.Fatalf("accepted=%v want=%v", got[tail], want)
			}
			if !slices.Equal(beforeNext, tail.Next) || !slices.Equal(beforeSource, tail.Source) || region.ProtectedEnd == nil {
				t.Fatal("proof mutated original boundary")
			}
		})
	}
}
