package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestFinallyNegatedBooleanGuardNeedsPureSameBinding(t *testing.T) {
	for _, scenario := range []string{"same boolean", "other local", "numeric operand", "extra operand", "wrong operator", "excluded local", "effect outside", "effect protected", "effect missing PC"} {
		t.Run(scenario, func(t *testing.T) {
			boolType := types.NewJavaPrimer(types.JavaBoolean)
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
			var a, b values.JavaValue = ref, ref
			rows := []core.HandlerRange{{StartPc: 10, EndPc: 20}}
			excluded := []*values.JavaRef{}
			if scenario == "other local" {
				b = values.NewJavaRef(utils.NewRootVariableId(), nil, boolType)
			}

			if scenario == "excluded local" {
				excluded = append(excluded, ref)
			}
			if scenario == "effect outside" || scenario == "effect protected" || scenario == "effect missing PC" {
				ca := &values.FunctionCallExpression{ClassName: "p.Actions", FunctionName: "guard", Descriptor: "()Z", IsStatic: true, Kind: values.InvokeStatic, OriginPC: 30, HasOriginPC: true, FuncType: types.NewJavaFuncType("()Z", nil, boolType)}
				cb := *ca
				cb.OriginPC = 40
				if scenario == "effect protected" {
					cb.OriginPC = 19
				}
				if scenario == "effect missing PC" {
					cb.HasOriginPC = false
				}
				a, b = ca, &cb
			}
			x, y := values.NewUnaryExpression(a, values.Not, boolType), values.NewUnaryExpression(b, values.Not, boolType)
			if scenario == "numeric operand" {
				ref.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
			}
			if scenario == "extra operand" {
				y.Values = append(y.Values, ref)
			}
			if scenario == "wrong operator" {
				y.Op = values.ADD
			}
			makeGuard := func(condition values.JavaValue, pc int) *statements.IfStatement {
				call := &values.FunctionCallExpression{ClassName: "p.Actions", FunctionName: "clean", Descriptor: "()V", IsStatic: true, Kind: values.InvokeStatic, OriginPC: pc, HasOriginPC: true}
				return statements.NewIfStatement(condition, []statements.Statement{statements.NewExpressionStatement(call)}, nil)
			}
			got := sameFinallyCleanup(rows, makeGuard(x, 31), makeGuard(y, 41), excluded, 0)
			want := scenario == "same boolean" || scenario == "effect outside"
			if got != want {
				t.Fatalf("proved=%v want=%v", got, want)
			}
		})
	}
}
