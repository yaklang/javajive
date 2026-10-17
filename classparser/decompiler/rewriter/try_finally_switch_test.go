package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestFinallyInternalSwitchRetainsEveryEffectAndLocalControlScope(t *testing.T) {
	for _, variant := range []string{"original fallthrough", "selector effect", "nested switch", "selector outside", "reference selector", "long selector", "case effect outside", "duplicate key", "duplicate default", "nil arm", "unsealed break", "labeled break", "continue", "method return", "unsealed throw", "throw outside", "nested loop", "cycle", "case bound", "work bound"} {
		t.Run(variant, func(t *testing.T) {
			selector := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			selector.IsParam = true
			failure := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
			failure.IsParam = true
			thrown := statements.NewThrowStatement(failure)
			thrown.HasOriginPC, thrown.OriginPC = true, 17
			body := statements.NewExpressionStatement(numericConversionCall(14))
			sw := &statements.SwitchStatement{Value: selector, Cases: []*statements.CaseItem{
				{IntValue: 0, Body: []statements.Statement{statements.NewExpressionStatement(numericConversionCall(12))}},
				{IntValue: 1, Body: []statements.Statement{body, statements.NewSourceTransferStatement("break", "")}},
				{IsDefault: true, Body: []statements.Statement{thrown}},
			}}
			switch variant {
			case "selector effect":
				sw.Value = numericConversionCall(11)
			case "nested switch":
				sw.Cases[0].Body = []statements.Statement{&statements.SwitchStatement{Value: selector, Cases: []*statements.CaseItem{{IntValue: 3, Body: []statements.Statement{statements.NewSourceTransferStatement("break", "")}}}}}
			case "selector outside":
				sw.Value = numericConversionCall(21)
			case "reference selector":
				selector.ResetVarType(types.NewJavaClass("java.lang.Integer"))
			case "long selector":
				selector.ResetVarType(types.NewJavaPrimer(types.JavaLong))
			case "case effect outside":
				body.Expression.(*values.FunctionCallExpression).OriginPC = 21
			case "duplicate key":
				sw.Cases[1].IntValue = 0
			case "duplicate default":
				sw.Cases[1].IsDefault = true
			case "nil arm":
				sw.Cases[1] = nil
			case "unsealed break":
				sw.Cases[1].Body[1] = &statements.CustomStatement{Name: "break", StringFunc: func(*class_context.ClassContext) string { panic("must not render") }}
			case "labeled break":
				sw.Cases[1].Body[1] = statements.NewSourceTransferStatement("break", "outer")
			case "continue":
				sw.Cases[1].Body[1] = statements.NewSourceTransferStatement("continue", "")
			case "method return":
				sw.Cases[1].Body[1] = statements.NewReturnStatement(nil)
			case "unsealed throw":
				sw.Cases[2].Body[0] = &statements.CustomStatement{ThrownValue: failure, HasOriginPC: true, OriginPC: 17}
			case "throw outside":
				thrown.OriginPC = 21
			case "nested loop":
				sw.Cases[0].Body = []statements.Statement{statements.NewDoWhileStatement(values.NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean)), nil)}
			case "cycle":
				sw.Cases[0].Body = []statements.Statement{sw}
			case "case bound":
				for len(sw.Cases) < 65 {
					sw.Cases = append(sw.Cases, &statements.CaseItem{IntValue: len(sw.Cases)})
				}
			case "work bound":
				for i := 0; i < 300; i++ {
					sw.Cases[0].Body = append(sw.Cases[0].Body, body)
				}
			}
			proof := finallyProof{remaining: 256}
			want := variant == "original fallthrough" || variant == "selector effect" || variant == "nested switch"
			if proof.internalSwitchCovered(sw, func(pc int) bool { return pc >= 10 && pc < 20 }) != want {
				t.Fatalf("switch effect/control domain accepted incorrectly: %s", variant)
			}
		})
	}
}
