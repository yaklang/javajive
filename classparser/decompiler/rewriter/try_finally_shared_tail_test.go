package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"math"
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

func TestFinallySharedValueTailKeepsLiteralBitsAndAllNormalExits(t *testing.T) {
	for _, variant := range []string{"integer", "null", "negative zero", "string units", "different value", "different type", "different zero sign", "different string units", "effectful value", "different return PC", "fallthrough handler", "typed throw exit"} {
		t.Run(variant, func(t *testing.T) {
			tr, tail := sharedFinallyReturnFixture()
			value := values.NewJavaLiteral(6, types.NewJavaPrimer(types.JavaInteger))
			if variant == "negative zero" || variant == "different zero sign" {
				value = values.NewJavaLiteral(math.Copysign(0, -1), types.NewJavaPrimer(types.JavaDouble))
			}
			if variant == "null" {
				value = values.NewOriginalNullLiteral(44)
			}
			if variant == "string units" || variant == "different string units" {
				value = values.NewJavaLiteral("", types.NewJavaClass("java.lang.String"))
				value.Units = []uint16{0xd800}
			}
			for _, r := range []*statements.ReturnStatement{tr.TryBody[2].(*statements.ReturnStatement), tr.CatchBodies[0][2].(*statements.ReturnStatement), tail} {
				copy := *value
				r.JavaValue = &copy
				if variant == "null" {
					r.JavaValue = value
				}
			}
			want := false
			switch variant {
			case "integer", "null", "negative zero", "string units":
				want = true
			case "different value":
				tail.JavaValue = values.NewJavaLiteral(7, value.Type())
			case "different type":
				tail.JavaValue = values.NewJavaLiteral(6, types.NewJavaPrimer(types.JavaLong))
			case "different zero sign":
				tail.JavaValue = values.NewJavaLiteral(float64(0), value.Type())
			case "different string units":
				tail.JavaValue.(*values.JavaLiteral).Units = []uint16{0xd801}
			case "effectful value":
				tail.JavaValue = tr.TryBody[0].(*statements.ExpressionStatement).Expression
			case "different return PC":
				tail.OriginPC++
			case "fallthrough handler":
				tr.CatchBodies[0] = tr.CatchBodies[0][:2]
			case "typed throw exit":
				throw := statements.NewThrowStatement(tr.Exception[0])
				throw.OriginPC, throw.HasOriginPC = 21, true
				tr.CatchBodies[0] = []statements.Statement{throw}
				want = true
			}
			if got := FinallyConsumesSharedReturn(tr, tail); got != want {
				t.Fatalf("shared value return=%v want=%v", got, want)
			}
		})
	}
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
			if got := FinallyConsumesSharedReturn(tr, tail); got != tt.want {
				t.Fatalf("proof%v want%v", got, tt.want)
			}
		})
	}
}

func TestSharedReturnLiteralRequiresDecodedCategoryAndPayload(t *testing.T) {
	intType := types.NewJavaPrimer(types.JavaInteger)
	stringType := types.NewJavaPrimer(types.JavaString)
	units := func() *values.JavaLiteral {
		v := values.NewJavaLiteral("display", stringType)
		v.Units = []uint16{'a', 0xd800}
		return v
	}
	for _, tc := range []struct {
		name string
		a, b values.JavaValue
		want bool
	}{
		{"decoded null", values.NewOriginalNullLiteral(4), values.NewOriginalNullLiteral(4), true},
		{"different null origin", values.NewOriginalNullLiteral(4), values.NewOriginalNullLiteral(7), false},
		{"forged null spelling", values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object")), values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object")), false},
		{"nil primitive payload", values.NewJavaLiteral(nil, intType), values.NewJavaLiteral(nil, intType), false},
		{"numeric object payload", values.NewJavaLiteral(6, types.NewJavaClass("java.lang.Object")), values.NewJavaLiteral(6, types.NewJavaClass("java.lang.Object")), false},
		{"string primer units", units(), units(), true},
		{"string null spelling", values.NewJavaLiteral("null", stringType), values.NewJavaLiteral("null", stringType), true},
		{"nonliteral string object", values.NewJavaLiteral(struct{}{}, stringType), values.NewJavaLiteral(struct{}{}, stringType), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameSharedReturnLiteral(tc.a, tc.b); got != tc.want {
				t.Fatalf("literal proof=%v want=%v", got, tc.want)
			}
		})
	}
}
