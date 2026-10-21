package rewriter

import (
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestFinallyFlagConditionMatchesPrimitiveBooleanTruthTable(t *testing.T) {
	for _, op := range []string{values.EQ, values.NEQ} {
		for _, word := range []int{0, 1} {
			for _, reverse := range []bool{false, true} {
				for depth := 0; depth < 4; depth++ {
					t.Run(fmt.Sprintf("%s/%d/reverse_%t/not_%d", op, word, reverse, depth), func(t *testing.T) {
						ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
						operands := []values.JavaValue{ref, values.NewJavaLiteral(word, types.NewJavaPrimer(types.JavaInteger))}
						if reverse {
							operands[0], operands[1] = operands[1], operands[0]
						}
						var condition values.JavaValue = &values.JavaExpression{Op: op, Values: operands, Typ: types.NewJavaPrimer(types.JavaBoolean)}
						for i := 0; i < depth; i++ {
							condition = &values.JavaExpression{Op: values.Not, Values: []values.JavaValue{condition}, Typ: types.NewJavaPrimer(types.JavaBoolean)}
						}
						local, negative, known := finallyFlagCondition(condition, 0)
						if !known || local != ref {
							t.Fatal("closed primitive boolean condition was not recognized")
						}
						for _, state := range []bool{false, true} {
							input := 0
							if state {
								input = 1
							}
							original := input == word
							if op == values.NEQ {
								original = !original
							}
							for i := 0; i < depth; i++ {
								original = !original
							}
							if (state != negative) != original {
								t.Fatal("normal form changes the original word comparison truth table")
							}
						}
					})
				}
			}
		}
	}
}

func TestFinallyFlagConditionRejectsEffectsNumericAndUnknownDomains(t *testing.T) {
	for _, variant := range []string{"numeric", "boxed", "unknown expression type", "opaque operand", "opaque root", "nonboolean word", "ordered comparison", "two locals", "depth limit", "cycle"} {
		t.Run(variant, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
			zero := values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))
			expr := &values.JavaExpression{Op: values.NEQ, Values: []values.JavaValue{ref, zero}, Typ: types.NewJavaPrimer(types.JavaBoolean)}
			var condition values.JavaValue = expr
			opaque := values.NewCustomValue(func(*class_context.ClassContext) string { panic("condition proof must not render") }, func() types.JavaType { panic("condition proof must not query opaque type") })
			switch variant {
			case "numeric":
				ref.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
			case "boxed":
				ref.ResetVarType(types.NewJavaClass("java.lang.Boolean"))
			case "unknown expression type":
				expr.Typ = nil
			case "opaque operand":
				expr.Values[0] = opaque
			case "opaque root":
				condition = opaque
			case "nonboolean word":
				zero.Data = 2
			case "ordered comparison":
				expr.Op = values.GT
			case "two locals":
				expr.Values[1] = values.NewJavaRef(utils.NewRootVariableId(), nil, ref.Type())
			case "depth limit":
				for i := 0; i < 9; i++ {
					condition = &values.JavaExpression{Op: values.Not, Values: []values.JavaValue{condition}, Typ: types.NewJavaPrimer(types.JavaBoolean)}
				}
			case "cycle":
				expr.Op, expr.Values = values.Not, []values.JavaValue{expr}
			}
			if _, _, known := finallyFlagCondition(condition, 0); known {
				t.Fatal("unknown/effectful condition granted a flag source binding")
			}
		})
	}
}
