package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func sharedFinallyReturnFixture() (*statements.TryCatchStatement, *statements.ReturnStatement) {
	tr := finallyFixture()
	cleanup := *tr.TryBody[1].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
	cleanup.OriginPC = 33
	work := *tr.TryBody[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
	work.OriginPC = 21
	work.FunctionName = "handler"
	ret := *tr.TryBody[2].(*statements.ReturnStatement)
	tr.CatchBodies[0] = []statements.Statement{statements.NewExpressionStatement(&work), statements.NewExpressionStatement(&cleanup), &ret}
	tr.Handlers[1].ProtectedRanges[1][1] = 30
	tail := ret
	return tr, &tail
}
func TestFinallySharedVoidTailRequiresCompleteSamePCProof(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*statements.TryCatchStatement, *statements.ReturnStatement)
		want   bool
	}{
		{"original shared void return", func(*statements.TryCatchStatement, *statements.ReturnStatement) {}, true},
		{"tail has no origin", func(_ *statements.TryCatchStatement, r *statements.ReturnStatement) { r.HasOriginPC = false }, false},
		{"tail is another return PC", func(_ *statements.TryCatchStatement, r *statements.ReturnStatement) { r.OriginPC++ }, false},
		{"tail evaluates value", func(_ *statements.TryCatchStatement, r *statements.ReturnStatement) {
			r.JavaValue = values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
		}, false},
		{"try terminal has no origin", func(tr *statements.TryCatchStatement, _ *statements.ReturnStatement) {
			tr.TryBody[2].(*statements.ReturnStatement).HasOriginPC = false
		}, false},
		{"typed catch completes at different PC", func(tr *statements.TryCatchStatement, _ *statements.ReturnStatement) {
			tr.CatchBodies[0][2].(*statements.ReturnStatement).OriginPC++
		}, false},
		{"typed catch may continue", func(tr *statements.TryCatchStatement, _ *statements.ReturnStatement) {
			tr.CatchBodies[0] = tr.CatchBodies[0][:2]
		}, false},
		{"original raw handler missing", func(tr *statements.TryCatchStatement, _ *statements.ReturnStatement) { tr.Handlers[1].CatchAll = false }, false},
		{"cleanup is a different effect", func(tr *statements.TryCatchStatement, _ *statements.ReturnStatement) {
			tr.CatchBodies[0][1].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).FunctionName = "other"
		}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tr, tail := sharedFinallyReturnFixture()
			tt.change(tr, tail)
			if got := FinallyConsumesSharedVoidReturn(tr, tail); got != tt.want {
				t.Fatalf("proof%v want%v", got, tt.want)
			}
		})
	}
}
