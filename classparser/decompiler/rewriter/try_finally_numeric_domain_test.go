package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestFinallyUnaryNumericNegationHasNoInventedEffectPC(t *testing.T) {
	for _, name := range []string{types.JavaByte, types.JavaShort, types.JavaChar, types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble} {
		t.Run(name, func(t *testing.T) {
			operand := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(name))
			result := name
			if name == types.JavaByte || name == types.JavaShort || name == types.JavaChar {
				result = types.JavaInteger
			}
			expr := &values.JavaExpression{Op: values.SUB, Values: []values.JavaValue{operand}, Typ: types.NewJavaPrimer(result)}
			if !finallyCoveredValue(expr, func(int) bool { panic("numeric negation has no throwing PC") }) {
				t.Fatal("pure unary numeric negation refused")
			}
		})
	}
}

func TestFinallyUnaryNegationRefusesSourceUnboxingAndWrongDomains(t *testing.T) {
	for _, variant := range []string{"covered operand call", "uncovered operand call", "missing call pc", "boxed operand", "boolean operand", "string operand", "boxed source view", "wrong result", "missing result", "wrong arity", "opaque operand"} {
		t.Run(variant, func(t *testing.T) {
			operand := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			expr := &values.JavaExpression{Op: values.SUB, Values: []values.JavaValue{operand}, Typ: types.NewJavaPrimer(types.JavaInteger)}
			switch variant {
			case "covered operand call", "uncovered operand call", "missing call pc":
				call := numericConversionCall(15)
				expr.Values[0] = call
				if variant == "missing call pc" {
					call.HasOriginPC = false
				}
			case "boxed operand":
				operand.ResetVarType(types.NewJavaClass("java.lang.Integer"))
			case "boolean operand":
				operand.ResetVarType(types.NewJavaPrimer(types.JavaBoolean))
			case "string operand":
				operand.ResetVarType(types.NewJavaPrimer(types.JavaString))
			case "boxed source view":
				operand.WebDeclType = types.NewJavaClass("java.lang.Integer")
			case "wrong result":
				expr.Typ = types.NewJavaPrimer(types.JavaLong)
			case "missing result":
				expr.Typ = nil
			case "wrong arity":
				expr.Values = nil
			case "opaque operand":
				expr.Values[0] = &values.CustomValue{}
			}
			want := variant == "covered operand call"
			got := finallyCoveredValue(expr, func(pc int) bool { return pc == 15 && variant != "uncovered operand call" })
			if got != want {
				t.Fatalf("coverage=%v want=%v", got, want)
			}
		})
	}
}
