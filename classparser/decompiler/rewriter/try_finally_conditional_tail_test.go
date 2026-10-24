package rewriter

import (
	"slices"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestFinallyConditionalTailRequiresExclusiveNormalPathAndClosedScope(t *testing.T) {
	for _, variant := range []string{"original", "other arm falls through", "other arm loop transfer", "protected tail", "unknown tail", "changed cleanup", "escaped declaration", "escaped wrapped declaration", "escaped catch binding", "typed handler falls through", "swapped arms"} {
		t.Run(variant, func(t *testing.T) {
			tr := finallyFixture()
			call := tr.TryBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
			copyCall := *call
			copyCall.OriginPC = 11
			cleanup := *tr.TryBody[1].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
			cleanup.OriginPC = 13
			tailCall := layerTestCall(100)
			tail := statements.NewExpressionStatement(tailCall)
			ret := &statements.ReturnStatement{OriginPC: 101, HasOriginPC: true}
			branch := statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), tr.TryBody,
				[]statements.Statement{statements.NewExpressionStatement(&copyCall), statements.NewExpressionStatement(&cleanup), tail, ret})
			tr.TryBody = []statements.Statement{branch}
			switch variant {
			case "other arm falls through":
				branch.IfBody = branch.IfBody[:2]
			case "other arm loop transfer":
				branch.IfBody[2] = statements.NewSourceTransferStatement("break", "")
			case "protected tail":
				tailCall.OriginPC = 10
			case "unknown tail":
				tailCall.HasOriginPC = false
			case "changed cleanup":
				cleanup.FunctionName = "other"
			case "escaped declaration", "escaped wrapped declaration":
				local := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
				decl := statements.NewAssignStatement(local, values.NewJavaLiteral(7, local.Type()), true)
				decl.HasOriginPC, decl.OriginPC = true, 11
				if variant == "escaped wrapped declaration" {
					decl.LeftValue = values.NewSlotValue(local, local.Type())
				}
				branch.ElseBody = append([]statements.Statement{decl}, branch.ElseBody...)
				tailCall.Arguments, tailCall.Descriptor = []values.JavaValue{local}, "(I)V"
				tailCall.FuncType = types.NewJavaFuncType("(I)V", []types.JavaType{local.Type()}, types.NewJavaPrimer(types.JavaVoid))
			case "typed handler falls through":
				x := *tr.CatchBodies[1][0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
				x.OriginPC = 44
				tr.CatchBodies[0] = []statements.Statement{statements.NewExpressionStatement(layerTestCall(21)), statements.NewExpressionStatement(&x)}
			case "escaped catch binding":
				local := tr.Exception[0]
				tailCall.Arguments, tailCall.Descriptor = []values.JavaValue{local}, "(Ljava/lang/Exception;)V"
				tailCall.FuncType = types.NewJavaFuncType("(Ljava/lang/Exception;)V", []types.JavaType{local.Type()}, types.NewJavaPrimer(types.JavaVoid))
			case "swapped arms":
				branch.IfBody, branch.ElseBody = branch.ElseBody, branch.IfBody
			}
			beforeIf, beforeElse := slices.Clone(branch.IfBody), slices.Clone(branch.ElseBody)
			want := variant == "original" || variant == "swapped arms"
			late, lateKnown := RecoverCatchAllFinallyForSource(tr)
			if lateKnown != want || tr.TryBody[0] != branch || !slices.Equal(beforeIf, branch.IfBody) || !slices.Equal(beforeElse, branch.ElseBody) {
				t.Fatal("late source proof changed the original tree or widened its domain")
			}
			if lateKnown && (len(late.SourceEpilogue) != 2 || late.SourceEpilogue[0] != tail || late.SourceEpilogue[1] != ret) {
				t.Fatal("late source view lost the original continuation identity")
			}
			suffix := factorFinallyFallthroughTail(tr)
			if (len(suffix) != 0) != want {
				t.Fatalf("factored=%v", len(suffix) != 0)
			}
			if !slices.Equal(beforeIf, branch.IfBody) || !slices.Equal(beforeElse, branch.ElseBody) {
				t.Fatal("original branch was mutated during the proof")
			}
			if want {
				if len(suffix) != 2 || suffix[0] != tail || suffix[1] != ret || tr.TryBody[0] == branch {
					t.Fatal("normal continuation identity or original branch changed")
				}
				if _, known := RecoverCatchAllFinallyForSource(tr); !known {
					t.Fatal("factored prefix lost its complete cleanup proof")
				}
			}
		})
	}
}
