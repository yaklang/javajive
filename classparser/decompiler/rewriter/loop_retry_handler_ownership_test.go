package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"slices"
	"testing"
)

func retryFrontierFixture() (region, handler, condition, left, right *core.Node, call *values.FunctionCallExpression) {
	region = core.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
	region.HasProtectedRange, region.ProtectedStartPC, region.ProtectedEndPC = true, 10, 20
	region.SharedProtectedHandler = true
	region.SharedProtectedRanges = []core.HandlerRange{{StartPc: 10, EndPc: 20, HandlerPc: 40, CatchType: 1}, {StartPc: 23, EndPc: 30, HandlerPc: 40, CatchType: 1}}
	handler = core.NewNode(&statements.ExpressionStatement{})
	handler.IsCatchStart, handler.HasOriginPC, handler.OriginPC = true, true, 40
	handler.CatchHandler = &statements.CatchHandler{EntryPC: 40, ProtectedRanges: [][2]int{{10, 20}, {23, 30}}}
	call = &values.FunctionCallExpression{ClassName: "proof/Operation", FunctionName: "test", Kind: values.InvokeStatic, IsStatic: true, Descriptor: "()Z", FuncType: types.NewJavaFuncType("()Z", nil, types.NewJavaPrimer(types.JavaBoolean)), HasOriginPC: true, OriginPC: 12}
	condition = core.NewNode(&statements.ConditionStatement{Condition: call})
	condition.HasOriginPC, condition.OriginPC = true, 15
	leftReturn := statements.NewReturnStatement(values.NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean)))
	rightReturn := statements.NewReturnStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)))
	leftReturn.HasOriginPC, leftReturn.OriginPC = true, 30
	rightReturn.HasOriginPC, rightReturn.OriginPC = true, 35
	left, right = core.NewNode(leftReturn), core.NewNode(rightReturn)
	left.HasOriginPC, left.OriginPC = true, 30
	right.HasOriginPC, right.OriginPC = true, 35
	region.AddNext(condition)
	region.AddNext(handler)
	condition.AddNext(left)
	condition.AddNext(right)
	return
}

func TestSplitRetryOwnershipRequiresExactPrivateOriginalRows(t *testing.T) {
	for _, change := range []string{"original", "no region", "no handler", "not split", "missing range", "missing rows", "unknown handler", "catch all", "unknown entry", "mismatched entry", "foreign handler entrance", "no reciprocal edge", "wrong region kind", "wrong interval", "different handler", "different catch type", "zero catch type", "overlap", "empty row", "unknown original rows", "extra original row", "row budget"} {
		t.Run(change, func(t *testing.T) {
			r, h, _, _, _, _ := retryFrontierFixture()
			switch change {
			case "no region":
				r = nil
			case "no handler":
				h = nil
			case "not split":
				r.SharedProtectedHandler = false
			case "missing range":
				r.HasProtectedRange = false
			case "missing rows":
				r.SharedProtectedRanges = nil
			case "unknown handler":
				h.CatchHandler = nil
			case "catch all":
				h.CatchHandler.CatchAll = true
			case "unknown entry":
				h.HasOriginPC = false
			case "mismatched entry":
				h.OriginPC++
			case "foreign handler entrance":
				core.NewNode(&statements.ExpressionStatement{}).AddNext(h)
			case "no reciprocal edge":
				r.Next = r.Next[:1]
			case "wrong region kind":
				r.Statement = &statements.ExpressionStatement{}
			case "wrong interval":
				r.ProtectedEndPC++
			case "different handler":
				r.SharedProtectedRanges[1].HandlerPc++
			case "different catch type":
				r.SharedProtectedRanges[1].CatchType++
			case "zero catch type":
				r.SharedProtectedRanges[0].CatchType = 0
			case "overlap":
				r.SharedProtectedRanges[1].StartPc = 19
			case "empty row":
				r.SharedProtectedRanges[1].EndPc = 23
			case "unknown original rows":
				h.CatchHandler.ProtectedRanges = nil
			case "extra original row":
				h.CatchHandler.ProtectedRanges = append(h.CatchHandler.ProtectedRanges, [2]int{50, 60})
			case "row budget":
				for len(r.SharedProtectedRanges) < 33 {
					r.SharedProtectedRanges = append(r.SharedProtectedRanges, r.SharedProtectedRanges[1])
				}
			}
			if got := splitRetryHandlerOwnershipClosed(r, h); got != (change == "original") {
				t.Fatalf("ownership=%v", got)
			}
		})
	}
}

func TestSplitRetryFrontiersPreserveDomainsAndAllNormalExits(t *testing.T) {
	for _, change := range []string{"two frontiers", "shared frontier", "gap transfer", "foreign path entrance", "unknown node PC", "unknown operand PC", "operand in gap", "operand outside", "condition in gap", "unknown frontier PC", "catch frontier", "nested region", "encoded transfer", "back edge", "missing successor", "opaque statement", "second normal entry", "nil successor", "node budget", "mixed frontier operand", "unknown frontier operand"} {
		t.Run(change, func(t *testing.T) {
			r, _, condition, left, right, call := retryFrontierFixture()
			switch change {
			case "shared frontier":
				condition.RemoveNext(right)
				// Keep two CFG arms even if both ultimately reach the same frontier.
				bridge := core.NewNode(&statements.GOTOStatement{})
				bridge.HasOriginPC, bridge.OriginPC = true, 20
				condition.AddNext(bridge)
				bridge.AddNext(left)
			case "gap transfer":
				bridge := core.NewNode(&statements.GOTOStatement{})
				bridge.HasOriginPC, bridge.OriginPC = true, 20
				condition.RemoveNext(left)
				condition.AddNext(bridge)
				bridge.AddNext(left)
			case "foreign path entrance":
				core.NewNode(&statements.ExpressionStatement{}).AddNext(condition)
			case "unknown node PC":
				condition.HasOriginPC = false
			case "unknown operand PC":
				call.HasOriginPC = false
			case "operand in gap":
				call.OriginPC = 21
			case "operand outside":
				call.OriginPC = 35
			case "condition in gap":
				condition.OriginPC = 21
			case "unknown frontier PC":
				left.HasOriginPC = false
			case "catch frontier":
				left.IsCatchStart = true
			case "nested region":
				condition.HasProtectedRange = true
			case "encoded transfer":
				condition.EncodedJumps = map[*core.Node]bool{left: true}
			case "back edge":
				condition.ReplaceNext(left, condition)
			case "missing successor":
				condition.RemoveNext(left)
			case "opaque statement":
				condition.Statement = &statements.MiddleStatement{}
			case "second normal entry":
				r.AddNext(left)
			case "nil successor":
				condition.Next[0] = nil
			case "mixed frontier operand":
				left.Statement.(*statements.ReturnStatement).JavaValue = call
			case "unknown frontier operand":
				left.Statement.(*statements.ReturnStatement).JavaValue = values.NewCustomValue(nil, nil)
			case "node budget":
				condition.RemoveNext(left)
				last := condition
				for i := 0; i < 513; i++ {
					next := core.NewNode(&statements.GOTOStatement{})
					next.HasOriginPC, next.OriginPC = true, 20
					last.AddNext(next)
					last = next
				}
				last.AddNext(left)
			}
			path, frontiers := splitRetryProtectedFrontiers(r, nil)
			want := change == "two frontiers" || change == "shared frontier" || change == "gap transfer"
			if (len(path) > 0) != want {
				t.Fatalf("path=%v frontiers=%v", path, frontiers)
			}
			if want && (slices.Contains(path, left) || slices.Contains(path, right) || !slices.Contains(path, condition) || !slices.Contains(frontiers, left)) {
				t.Fatal("outside continuation absorbed or protected condition lost")
			}
			if change == "two frontiers" && len(frontiers) != 2 {
				t.Fatal("normal frontiers collapsed")
			}
			if change == "shared frontier" && len(frontiers) != 1 {
				t.Fatal("shared frontier duplicated")
			}
		})
	}
}
