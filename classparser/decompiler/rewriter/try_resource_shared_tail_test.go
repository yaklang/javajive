package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"reflect"
	"slices"
	"testing"
)

func sharedVoidResourceTailFixture() (*core.Node, *statements.TryCatchStatement) {
	n, tr := resourceTailFixture()
	n.HasProtectedRange = true
	n.ProtectedStartPC = 0
	n.ProtectedEndPC = 20
	n.SharedProtectedHandler = true
	n.SharedProtectedRanges = []core.HandlerRange{{StartPc: 0, EndPc: 20, HandlerPc: 80}, {StartPc: 30, EndPc: 60, HandlerPc: 80}}
	tr.Handlers[0] = statements.CatchHandler{EntryPC: 80, ProtectedRanges: [][2]int{{0, 20}, {30, 60}}}
	cleanup := tr.CatchBodies[0][0].(*statements.IfStatement).IfBody[0].(*statements.TryCatchStatement)
	cleanup.TryBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = 91
	cleanup.Handlers[0] = statements.CatchHandler{EntryPC: 100, ProtectedRanges: [][2]int{{90, 95}}}
	cleanup.CatchBodies[0][0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = 101
	tr.CatchBodies[0][1].(*statements.CustomStatement).OriginPC = 109
	tr.TryBody[1].(*statements.ReturnStatement).JavaValue = nil
	copyCall := *tr.TryBody[0].(*statements.IfStatement).IfBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
	copyCall.OriginPC = 61
	copyGuard := *tr.TryBody[0].(*statements.IfStatement)
	copyGuard.IfBody = []statements.Statement{statements.NewExpressionStatement(&copyCall)}
	caught := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.io.IOException"))
	thrown := statements.NewThrowStatement(caught)
	thrown.HasOriginPC = true
	thrown.OriginPC = 50
	success := []statements.Statement{statements.NewExpressionStatement(layerTestCall(35)), &copyGuard, &statements.ReturnStatement{OriginPC: 65, HasOriginPC: true}}
	branch := statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), success, []statements.Statement{thrown})
	nested := statements.NewTryCatchStatement([]statements.Statement{statements.NewExpressionStatement(layerTestCall(3))}, [][]statements.Statement{{branch}})
	nested.Exception = []*values.JavaRef{caught}
	nested.Handlers = []statements.CatchHandler{{EntryPC: 30, ProtectedRanges: [][2]int{{0, 20}}}}
	tr.TryBody = append([]statements.Statement{nested}, tr.TryBody...)
	return n, tr
}
func TestSharedResourceVoidTailRequiresCompleteRangesAndEveryExit(t *testing.T) {
	for _, variant := range []string{"proved", "direct cleanup", "mixed normal guards", "split rethrow interval", "no range", "foreign handler", "missing row", "foreign row entry", "normal close protected", "catch close protected", "wrong close receiver", "different close invoke", "unknown close origin", "missing return origin", "value exit", "protected continuation", "nested handler outside", "nested range outside", "missing nested exception", "empty successful arm", "missing cleanup", "loop transfer", "opaque effect", "budget"} {
		t.Run(variant, func(t *testing.T) {
			n, tr := sharedVoidResourceTailFixture()
			nested := tr.TryBody[0].(*statements.TryCatchStatement)
			branch := nested.CatchBodies[0][0].(*statements.IfStatement)
			call := branch.IfBody[1].(*statements.IfStatement).IfBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
			switch variant {
			case "direct cleanup", "mixed normal guards":
				tr.CatchBodies[0][0] = tr.CatchBodies[0][0].(*statements.IfStatement).IfBody[0]
				tr.TryBody[1] = tr.TryBody[1].(*statements.IfStatement).IfBody[0]
				if variant == "direct cleanup" {
					branch.IfBody[1] = branch.IfBody[1].(*statements.IfStatement).IfBody[0]
				}
			case "split rethrow interval":
				n.ProtectedEndPC = 40
				n.SharedProtectedRanges[0].EndPc = 40
				n.SharedProtectedRanges[1].StartPc = 50
				n.SharedProtectedRanges[1].EndPc = 51
				tr.Handlers[0].ProtectedRanges = [][2]int{{0, 40}, {50, 51}}
				tr.TryBody[1].(*statements.IfStatement).IfBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = 41
				tr.TryBody[2].(*statements.ReturnStatement).OriginPC = 46
			case "no range":
				n.HasProtectedRange = false
			case "foreign handler":
				n.SharedProtectedRanges[1].HandlerPc++
			case "missing row":
				n.SharedProtectedRanges = n.SharedProtectedRanges[:1]
			case "foreign row entry":
				n.SharedProtectedRanges[1].StartPc = 29
				tr.Handlers[0].ProtectedRanges[1][0] = 29
			case "normal close protected":
				tr.Handlers[0].ProtectedRanges[0][1] = 22
				n.SharedProtectedRanges[0].EndPc = 22
				n.ProtectedEndPC = 22
			case "catch close protected":
				call.OriginPC = 59
			case "wrong close receiver":
				call.Object = values.NewJavaRef(utils.NewRootVariableId(), nil, call.Object.Type())
			case "different close invoke":
				call.FunctionName = "other"
			case "unknown close origin":
				call.HasOriginPC = false
			case "missing return origin":
				branch.IfBody[2].(*statements.ReturnStatement).HasOriginPC = false
			case "value exit":
				branch.IfBody[2].(*statements.ReturnStatement).JavaValue = values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
			case "protected continuation":
				tr.TryBody = append(tr.TryBody[:1], append([]statements.Statement{statements.NewExpressionStatement(layerTestCall(4))}, tr.TryBody[1:]...)...)
			case "nested handler outside":
				nested.Handlers[0].EntryPC = 60
			case "nested range outside":
				nested.Handlers[0].ProtectedRanges[0][1] = 21
			case "missing nested exception":
				nested.Exception[0] = nil
			case "missing cleanup":
				branch.IfBody[1] = statements.NewExpressionStatement(layerTestCall(35))
			case "empty successful arm":
				branch.IfBody = nil
			case "loop transfer":
				branch.ElseBody = []statements.Statement{statements.NewSourceTransferStatement("break", "LOOP")}
			case "opaque effect":
				branch.IfBody = append([]statements.Statement{&statements.CustomStatement{}}, branch.IfBody...)
			case "budget":
				for i := 0; i < 128; i++ {
					branch.IfBody = append([]statements.Statement{statements.NewExpressionStatement(layerTestCall(35))}, branch.IfBody...)
				}
			}
			before := append([]statements.Statement{}, tr.TryBody...)
			originalBranch := slices.Clone(branch.IfBody)
			declaration, tail := factorResourceNormalTail(n, tr)
			want := variant == "proved" || variant == "direct cleanup" || variant == "empty successful arm" || variant == "split rethrow interval"
			if (len(tail) == 2) != want || declaration != nil {
				t.Fatalf("shared void factoring=%t want=%t", len(tail) == 2, want)
			}
			if !reflect.DeepEqual(originalBranch, branch.IfBody) {
				t.Fatal("nested input source mutated")
			}
			if !want && !reflect.DeepEqual(before, tr.TryBody) {
				t.Fatal("failed proof changed source")
			}
			if want {
				if len(tr.TryBody) != 1 {
					t.Fatal("normal cleanup still protected")
				}
				view := tr.TryBody[0].(*statements.TryCatchStatement).CatchBodies[0][0].(*statements.IfStatement)
				expected := 1
				if variant == "empty successful arm" {
					expected = 0
				}
				if len(view.IfBody) != expected {
					t.Fatal("early normal exit retained duplicate close")
				}
				if view.ElseBody[0] != branch.ElseBody[0] {
					t.Fatal("abrupt exit identity changed")
				}
			}
		})
	}
}

func TestResourceTerminalNestedTryRequiresAllOriginalExits(t *testing.T) {
	for _, variant := range []string{"proved", "missing normal close", "empty normal arm", "normal close protected", "caught close protected", "missing caught return", "fallthrough caught arm", "protected continuation", "foreign nested range", "missing exception", "cycle"} {
		t.Run(variant, func(t *testing.T) {
			n, tr := sharedVoidResourceTailFixture()
			nested := tr.TryBody[0].(*statements.TryCatchStatement)
			branch := nested.CatchBodies[0][0].(*statements.IfStatement)
			nested.TryBody = append(slices.Clone(nested.TryBody), tr.TryBody[1:]...)
			tr.TryBody = []statements.Statement{nested}
			switch variant {
			case "missing normal close":
				nested.TryBody = append(nested.TryBody[:1], nested.TryBody[2:]...)
			case "empty normal arm":
				nested.TryBody = nil
			case "normal close protected":
				nested.TryBody[1].(*statements.IfStatement).IfBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = 19
			case "caught close protected":
				branch.IfBody[1].(*statements.IfStatement).IfBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).OriginPC = 59
			case "missing caught return":
				branch.IfBody = branch.IfBody[:2]
			case "fallthrough caught arm":
				branch.ElseBody = nil
			case "protected continuation":
				tr.TryBody = append(tr.TryBody, statements.NewExpressionStatement(layerTestCall(4)))
			case "foreign nested range":
				nested.Handlers[0].ProtectedRanges[0][1] = 21
			case "missing exception":
				nested.Exception[0] = nil
			case "cycle":
				nested.TryBody = []statements.Statement{nested}
			}
			before := slices.Clone(tr.TryBody)
			declaration, tail := factorResourceNormalTail(n, tr)
			if got := len(tail) == 2; got != (variant == "proved") || declaration != nil {
				t.Fatalf("terminal nested factoring=%v", got)
			}
			if variant != "proved" && !reflect.DeepEqual(before, tr.TryBody) {
				t.Fatal("failed proof mutated source")
			}
			if variant == "proved" {
				view := tr.TryBody[0].(*statements.TryCatchStatement)
				if len(view.TryBody) != 1 || len(view.CatchBodies[0][0].(*statements.IfStatement).IfBody) != 1 || len(nested.TryBody) != 3 {
					t.Fatal("cleanup not factored from every successful exit immutably")
				}
			}
		})
	}
}
