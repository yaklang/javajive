package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	valueutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	utils2 "github.com/yaklang/javajive/internal/utils"
)

func TestProtectedRetryTerminalRegionRequiresCompleteOriginalExceptionDomain(t *testing.T) {
	for _, change := range []string{
		"original split domain", "one interval", "covered nested argument", "missing range", "wrong region interval",
		"no original rows", "different original rows", "missing split rows", "foreign handler row", "different catch type",
		"catch all", "unknown handler", "unknown handler entry", "foreign handler entry", "missing cyclic handler",
		"missing node origin", "missing return origin", "different return origin", "return beyond boundary", "return successor",
		"unprotected allocation", "allocation in gap", "unknown allocation", "unprotected constructor", "constructor in gap",
		"unknown constructor", "missing constructor", "wrong invoke kind", "mixed nested argument", "opaque operand", "cyclic operand",
		"pure return", "unprotected whole return", "foreign return entrance", "foreign prefix entrance", "nested region", "unknown condition",
		"unprotected condition", "unknown condition effect", "normal continuation", "no normal retry", "no return", "encoded transfer",
	} {
		t.Run(change, func(t *testing.T) {
			loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
			region := core.NewNode(&statements.MiddleStatement{Flag: statements.MiddleTryStart})
			region.HasProtectedRange, region.ProtectedStartPC, region.ProtectedEndPC = true, 10, 14
			region.SharedProtectedHandler = true
			region.SharedProtectedRanges = []core.HandlerRange{{StartPc: 10, EndPc: 14, HandlerPc: 25, CatchType: 1}, {StartPc: 17, EndPc: 24, HandlerPc: 25, CatchType: 1}}
			handler := core.NewNode(&statements.ExpressionStatement{})
			handler.IsCatchStart, handler.HasOriginPC, handler.OriginPC = true, true, 25
			handler.CatchHandler = &statements.CatchHandler{EntryPC: 25, ProtectedRanges: [][2]int{{10, 14}, {17, 24}}}
			condition := core.NewNode(&statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))})
			condition.HasOriginPC, condition.OriginPC = true, 12
			allocation := values.NewNewExpression(types.NewJavaClass("proof.Output"))
			allocation.HasOriginPC, allocation.OriginPC = true, 17
			constructor := &values.FunctionCallExpression{Object: allocation, ClassName: "proof/Output", FunctionName: "<init>", Kind: values.InvokeSpecial, Descriptor: "()V", FuncType: types.NewJavaFuncType("()V", nil, types.NewJavaPrimer(types.JavaVoid)), HasOriginPC: true, OriginPC: 21}
			allocation.ConstructorCall = constructor
			ret := statements.NewReturnStatement(allocation)
			ret.HasOriginPC, ret.OriginPC = true, 24
			terminal := core.NewNode(ret)
			terminal.HasOriginPC, terminal.OriginPC = true, 24
			end := core.NewNode(&statements.MiddleStatement{Flag: "end"})
			loop.AddNext(region)
			region.AddNext(condition)
			region.AddNext(handler)
			condition.AddNext(loop)
			condition.AddNext(terminal)
			handler.AddNext(loop)
			terminal.AddNext(end)
			cyclic := utils2.NewSet[*core.Node]()
			cyclic.AddList([]*core.Node{loop, region, handler})
			argument := func(pc int) values.JavaValue {
				return &values.FunctionCallExpression{ClassName: "proof/Arguments", FunctionName: "next", Kind: values.InvokeStatic, IsStatic: true, Descriptor: "()I", FuncType: types.NewJavaFuncType("()I", nil, types.NewJavaPrimer(types.JavaInteger)), HasOriginPC: true, OriginPC: pc}
			}
			switch change {
			case "one interval":
				region.SharedProtectedHandler, region.ProtectedEndPC = false, 24
				region.SharedProtectedRanges = nil
				handler.CatchHandler.ProtectedRanges = [][2]int{{10, 24}}
			case "covered nested argument":
				constructor.Arguments = []values.JavaValue{argument(20)}
			case "missing range":
				region.HasProtectedRange = false
			case "wrong region interval":
				region.ProtectedEndPC = 15
			case "no original rows":
				handler.CatchHandler.ProtectedRanges = nil
			case "different original rows":
				handler.CatchHandler.ProtectedRanges[1][0] = 18
			case "missing split rows":
				region.SharedProtectedRanges = nil
			case "foreign handler row":
				region.SharedProtectedRanges[1].HandlerPc++
			case "different catch type":
				region.SharedProtectedRanges[1].CatchType++
			case "catch all":
				handler.CatchHandler.CatchAll = true
			case "unknown handler":
				handler.CatchHandler = nil
			case "unknown handler entry":
				handler.HasOriginPC = false
			case "foreign handler entry":
				core.NewNode(&statements.ExpressionStatement{}).AddNext(handler)
			case "missing cyclic handler":
				cyclic.Remove(handler)
			case "missing node origin":
				terminal.HasOriginPC = false
			case "missing return origin":
				ret.HasOriginPC = false
			case "different return origin":
				ret.OriginPC++
			case "return beyond boundary":
				terminal.OriginPC, ret.OriginPC = 25, 25
			case "return successor":
				terminal.AddNext(loop)
			case "unprotected allocation":
				allocation.OriginPC = 9
			case "allocation in gap":
				allocation.OriginPC = 15
			case "unknown allocation":
				allocation.HasOriginPC = false
			case "unprotected constructor":
				constructor.OriginPC = 24
			case "constructor in gap":
				constructor.OriginPC = 16
			case "unknown constructor":
				constructor.HasOriginPC = false
			case "missing constructor":
				allocation.ConstructorCall = nil
			case "wrong invoke kind":
				constructor.Kind = values.InvokeVirtual
			case "mixed nested argument":
				constructor.Arguments = []values.JavaValue{argument(15)}
			case "opaque operand":
				ret.JavaValue = values.NewCustomValue(nil, nil)
			case "cyclic operand":
				constructor.Arguments = []values.JavaValue{allocation}
			case "pure return":
				ret.JavaValue = values.NewJavaRef(valueutils.NewRootVariableId(), nil, types.NewJavaClass("proof.Output"))
			case "unprotected whole return":
				allocation.OriginPC, constructor.OriginPC = 26, 30
				terminal.OriginPC, ret.OriginPC = 33, 33
			case "foreign return entrance":
				core.NewNode(&statements.ExpressionStatement{}).AddNext(terminal)
			case "foreign prefix entrance":
				core.NewNode(&statements.ExpressionStatement{}).AddNext(condition)
			case "nested region":
				condition.HasProtectedRange = true
			case "unknown condition":
				condition.HasOriginPC = false
			case "unprotected condition":
				condition.OriginPC = 16
			case "unknown condition effect":
				condition.Statement.(*statements.ConditionStatement).Condition = argument(16)
			case "normal continuation":
				outside := core.NewNode(&statements.ExpressionStatement{Expression: argument(26)})
				outside.HasOriginPC, outside.OriginPC = true, 26
				condition.ReplaceNext(terminal, outside)
				outside.AddNext(terminal)
			case "no normal retry":
				condition.RemoveNext(loop)
				condition.Statement = statements.NewExpressionStatement(argument(12))
			case "no return":
				condition.ReplaceNext(terminal, loop)
			case "encoded transfer":
				terminal.EncodedJumps = map[*core.Node]bool{loop: true}
			}
			closed := protectedRetryClosedRegion(region, loop, cyclic)
			want := change == "original split domain" || change == "one interval" || change == "covered nested argument" || change == "no normal retry"
			outside := change == "pure return" || change == "unprotected whole return"
			if (closed != nil) != (want || outside) {
				t.Fatalf("closed terminal region=%v want=%v", closed, want || outside)
			}
			if closed != nil && (outside && (closed.continuation != terminal || len(closed.path) != 1) || want && (closed.continuation != nil || len(closed.path) != 2)) {
				t.Fatalf("protected path / normal continuation mixed: %+v", closed)
			}
			if want {
				dom := GenerateDominatorTree(loop)
				body := circleElementSet(loop, region, dom, true)
				if !body.Has(terminal) || searchCircleEndNode(loop, region, dom, true) == terminal {
					t.Fatal("protected throwing return escaped the original retry loop")
				}
			}
		})
	}
}
