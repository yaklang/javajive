package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestPureEarlyReturnKeepsOnlyClosedNonthrowingOperands(t *testing.T) {
	for _, variant := range []string{"constant", "local", "null", "void", "arithmetic", "division", "call", "allocation", "field", "opaque", "missing node origin", "missing return origin", "changed PC", "live successor", "encoded transfer"} {
		t.Run(variant, func(t *testing.T) {
			integer := types.NewJavaPrimer(types.JavaInteger)
			value := values.NewJavaLiteral(7, integer)
			ret := &statements.ReturnStatement{JavaValue: value, OriginPC: 10, HasOriginPC: true}
			node := core.NewNode(ret)
			node.OriginPC, node.HasOriginPC = 10, true
			switch variant {
			case "local":
				ret.JavaValue = layerTestRef("java.lang.Object")
			case "null":
				ret.JavaValue = values.JavaNull
			case "void":
				ret.JavaValue = nil
			case "arithmetic":
				ret.JavaValue = values.NewBinaryExpression(value, value, values.ADD, integer)
			case "division":
				ret.JavaValue = values.NewBinaryExpression(value, value, values.DIV, integer)
			case "call":
				ret.JavaValue = layerTestCall(24)
			case "allocation":
				ret.JavaValue = &values.NewExpression{}
			case "field":
				ret.JavaValue = values.NewRefMember(layerTestRef("Owner"), "word", integer)
			case "opaque":
				ret.JavaValue = &values.CustomValue{}
			case "missing node origin":
				node.HasOriginPC = false
			case "missing return origin":
				ret.HasOriginPC = false
			case "changed PC":
				ret.OriginPC++
			case "live successor":
				node.AddNext(core.NewNode(statements.NewExpressionStatement(layerTestCall(32))))
			case "encoded transfer":
				node.EncodedJumps = map[*core.Node]bool{node: true}
			}
			want := variant == "constant" || variant == "local" || variant == "null" || variant == "void" || variant == "arithmetic"
			if got := protectedArmCanOwnReturn(node); got != want {
				t.Fatalf("accepted=%v want=%v", got, want)
			}
		})
	}
}

func TestEarlyReturnFrontierClosesAllNormalPathsAndRejectsProtectedReentry(t *testing.T) {
	for _, variant := range []string{"original", "structured branch", "structured terminal live successor", "condition missing edge", "pure local early return", "effectful early return", "unknown end origin", "wrong physical endpoint", "invalid range", "catchall", "nonterminal handler", "second handler", "opaque prefix", "outside prefix", "outside prefix reentry", "two frontiers", "mixed frontier effects", "encoded path", "live return reentry", "missing dominators"} {
		t.Run(variant, func(t *testing.T) {
			region := core.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
			region.HasProtectedRange = true
			region.ProtectedStartPC, region.ProtectedEndPC = 0, 10
			prefix := core.NewNode(statements.NewExpressionStatement(layerTestCall(3)))
			condition := core.NewNode(&statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))})
			condition.OriginPC, condition.HasOriginPC = 7, true
			pure := &statements.ReturnStatement{JavaValue: values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger)), OriginPC: 10, HasOriginPC: true}
			early := core.NewNode(pure)
			early.OriginPC, early.HasOriginPC = 10, true
			region.ProtectedEnd = early
			jump := core.NewNode(&statements.GOTOStatement{})
			jump.OriginPC, jump.HasOriginPC = 11, true
			call := layerTestCall(24)
			tail := core.NewNode(&statements.ReturnStatement{JavaValue: call, OriginPC: 27, HasOriginPC: true})
			tail.OriginPC, tail.HasOriginPC = 27, true
			handler := core.NewNode(&statements.ReturnStatement{})
			handler.IsCatchStart = true
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 40, ProtectedRanges: [][2]int{{0, 10}}}
			region.AddNext(prefix)
			region.AddNext(handler)
			prefix.AddNext(condition)
			condition.AddNext(early)
			condition.AddNext(jump)
			jump.AddNext(tail)
			want := tail
			switch variant {
			case "structured branch":
				condition.Statement = statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{pure}, nil)
				condition.HasOriginPC = false
				condition.RemoveNext(early)
			case "structured terminal live successor":
				condition.Statement = statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{pure}, []statements.Statement{pure})
				condition.RemoveNext(early)
				want = nil
			case "condition missing edge":
				condition.RemoveNext(early)
				want = nil
			case "pure local early return":
				pure.JavaValue = layerTestRef("java.lang.Object")
			case "effectful early return":
				pure.JavaValue = layerTestCall(13)
				want = nil
			case "unknown end origin":
				early.HasOriginPC = false
				want = nil
			case "wrong physical endpoint":
				region.ProtectedEndPC++
				want = nil
			case "invalid range":
				region.ProtectedStartPC = 10
				want = nil
			case "catchall":
				handler.CatchHandler.CatchAll = true
				want = nil
			case "nonterminal handler":
				handler.Statement = &statements.MiddleStatement{}
				want = nil
			case "second handler":
				other := core.NewNode(&statements.ReturnStatement{})
				other.IsCatchStart = true
				region.AddNext(other)
				want = nil
			case "opaque prefix":
				prefix.Statement = &statements.CustomStatement{}
				want = nil
			case "outside prefix":
				prefix.Statement = statements.NewExpressionStatement(layerTestCall(14))
				want = prefix
			case "outside prefix reentry":
				prefix.Statement = statements.NewExpressionStatement(layerTestCall(14))
				condition.Statement.(*statements.ConditionStatement).Condition = layerTestCall(5)
				want = nil
			case "two frontiers":
				condition.RemoveNext(jump)
				split := core.NewNode(&statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))})
				split.OriginPC, split.HasOriginPC = 8, true
				condition.AddNext(split)
				split.AddNext(tail)
				split.AddNext(core.NewNode(&statements.ReturnStatement{JavaValue: layerTestCall(32)}))
				want = nil
			case "mixed frontier effects":
				call.Arguments = []values.JavaValue{layerTestCall(3)}
				want = nil
			case "encoded path":
				condition.EncodedJumps = map[*core.Node]bool{jump: true}
				want = nil
			case "live return reentry":
				tail.AddNext(prefix)
				want = nil
			}
			dom := GenerateDominatorTree(region)
			if variant == "missing dominators" {
				dom = nil
				want = nil
			}
			if got := typedEarlyReturnNormalFrontier(region, prefix, region.Next, dom); got != want {
				t.Fatalf("got frontier=%p want=%p", got, want)
			}
		})
	}
}
