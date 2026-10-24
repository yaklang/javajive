package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestFinallyShortCircuitKeepsBooleanTypesOperandOrderAndDomains(t *testing.T) {
	for _, op := range []string{values.LOGICAL_AND, values.LOGICAL_OR} {
		t.Run(op, func(t *testing.T) {
			for _, variant := range []string{"same ordered effects", "different operator", "swapped operands", "boxed operand", "numeric operand", "numeric result", "wrong arity", "protected effect", "missing effect PC", "excluded local"} {
				t.Run(variant, func(t *testing.T) {
					boolean := types.NewJavaPrimer(types.JavaBoolean)
					ref := values.NewJavaRef(utils.NewRootVariableId(), nil, boolean)
					call := &values.FunctionCallExpression{ClassName: "p/Effects", FunctionName: "guard", Descriptor: "()Z", FuncType: types.NewJavaFuncType("()Z", nil, boolean), Kind: values.InvokeStatic, IsStatic: true, OriginPC: 30, HasOriginPC: true}
					other := *call
					other.OriginPC = 40
					x := &values.JavaExpression{Op: op, Values: []values.JavaValue{ref, call}, Typ: boolean}
					y := &values.JavaExpression{Op: op, Values: []values.JavaValue{ref, &other}, Typ: boolean}
					var excluded []*values.JavaRef
					switch variant {
					case "different operator":
						if op == values.LOGICAL_AND {
							y.Op = values.LOGICAL_OR
						} else {
							y.Op = values.LOGICAL_AND
						}
					case "swapped operands":
						y.Values[0], y.Values[1] = y.Values[1], y.Values[0]
					case "boxed operand":
						ref.ResetVarType(types.NewJavaClass("java.lang.Boolean"))
					case "numeric operand":
						ref.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
					case "numeric result":
						y.Typ = types.NewJavaPrimer(types.JavaInteger)
					case "wrong arity":
						y.Values = y.Values[:1]
					case "protected effect":
						other.OriginPC = 19
					case "missing effect PC":
						other.HasOriginPC = false
					case "excluded local":
						excluded = []*values.JavaRef{ref}
					}
					if got := sameFinallyEffectValue([]core.HandlerRange{{StartPc: 10, EndPc: 20}}, x, y, excluded, 0); got != (variant == "same ordered effects") {
						t.Fatalf("same=%v", got)
					}
				})
			}
		})
	}
}

func TestFinallyOriginalTypeTestRequiresExactTargetAndOriginalDomain(t *testing.T) {
	for _, variant := range []string{"original", "different target", "different array rank", "protected type test", "public annotations", "opaque callback", "primitive operand", "excluded operand"} {
		t.Run(variant, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			x := values.NewOriginalInstanceOf(ref, types.NewJavaClass("java.lang.String"), 30)
			y := values.NewOriginalInstanceOf(ref, types.NewJavaClass("java.lang.String"), 40)
			var excluded []*values.JavaRef
			switch variant {
			case "different target":
				y = values.NewOriginalInstanceOf(ref, types.NewJavaClass("java.lang.Integer"), 40)
			case "different array rank":
				x = values.NewOriginalInstanceOf(ref, types.NewJavaArrayType(types.NewJavaClass("java.lang.String")), 30)
				y = values.NewOriginalInstanceOf(ref, types.NewJavaArrayType(types.NewJavaArrayType(types.NewJavaClass("java.lang.String"))), 40)
			case "protected type test":
				y = values.NewOriginalInstanceOf(ref, types.NewJavaClass("java.lang.String"), 19)
			case "public annotations":
				y.Flag, y.HasOriginPC, y.CapturesKnown, y.Captures = "other", false, false, nil
			case "opaque callback":
				y = &values.CustomValue{Flag: "instanceof", CapturesKnown: true, Captures: []values.JavaValue{ref}, OriginPC: 40, HasOriginPC: true}
			case "primitive operand":
				ref.ResetVarType(types.NewJavaPrimer(types.JavaInteger))
			case "excluded operand":
				excluded = []*values.JavaRef{ref}
			}
			if got := sameFinallyEffectValue([]core.HandlerRange{{StartPc: 10, EndPc: 20}}, x, y, excluded, 0); got != (variant == "original" || variant == "public annotations") {
				t.Fatalf("same=%v", got)
			}
		})
	}
}
