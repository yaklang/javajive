package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func numericConversionCall(pc int) *values.FunctionCallExpression {
	return &values.FunctionCallExpression{ClassName: "example.Input", FunctionName: "read", Descriptor: "()I", IsStatic: true, Kind: values.InvokeStatic,
		HasOriginPC: true, OriginPC: pc, FuncType: types.NewJavaFuncType("()I", nil, types.NewJavaPrimer(types.JavaInteger))}
}

func TestPrimitiveConversionCoverageRetainsOriginalOperandEffects(t *testing.T) {
	for _, variant := range []string{"original", "total opcode outside", "operand outside", "hidden original operand", "opaque public flag", "reference operand", "nested conversion", "boolean word"} {
		t.Run(variant, func(t *testing.T) {
			call := numericConversionCall(15)
			cast := values.NewOriginalPrimitiveConversion(call, types.NewJavaPrimer(types.JavaLong), 17)
			var input values.JavaValue = cast
			switch variant {
			case "total opcode outside":
				input = values.NewOriginalPrimitiveConversion(call, types.NewJavaPrimer(types.JavaLong), 25)
			case "operand outside":
				call.OriginPC = 25
			case "hidden original operand":
				call.OriginPC = 25
				cast.Captures = []values.JavaValue{values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))}
				cast.StringFunc = func(*class_context.ClassContext) string { panic("must not render") }
			case "opaque public flag":
				x := values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, func() types.JavaType { return types.NewJavaPrimer(types.JavaLong) })
				x.Flag, x.CapturesKnown, x.Captures, x.HasOriginPC, x.OriginPC = "primitive_cast", true, []values.JavaValue{call}, true, 17
				input = x
			case "reference operand":
				call.FuncType.ReturnType = types.NewJavaClass("java.lang.Integer")
				call.Descriptor = "()Ljava/lang/Integer;"
			case "nested conversion":
				input = values.NewOriginalPrimitiveConversion(cast, types.NewJavaPrimer(types.JavaDouble), 18)
			case "boolean word":
				input = values.NewOriginalPrimitiveConversion(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), types.NewJavaPrimer(types.JavaLong), 18)
			}
			want := variant == "original" || variant == "total opcode outside" || variant == "nested conversion" || variant == "boolean word"
			if got := finallyCoveredValue(input, func(pc int) bool { return pc >= 10 && pc < 20 }); got != want {
				t.Fatalf("conversion domain=%v want=%v", got, want)
			}
		})
	}
}

func TestPrimitiveConversionCleanupMatchesOperationAndEffectIdentity(t *testing.T) {
	rows := []core.HandlerRange{{StartPc: 10, EndPc: 20}}
	for _, variant := range []string{"original", "other conversion", "protected operand", "other method", "reference operand", "opaque substitute"} {
		t.Run(variant, func(t *testing.T) {
			a, b := numericConversionCall(25), numericConversionCall(35)
			x := values.NewOriginalPrimitiveConversion(a, types.NewJavaPrimer(types.JavaLong), 28)
			var y values.JavaValue = values.NewOriginalPrimitiveConversion(b, types.NewJavaPrimer(types.JavaLong), 38)
			switch variant {
			case "other conversion":
				y = values.NewOriginalPrimitiveConversion(b, types.NewJavaPrimer(types.JavaFloat), 38)
			case "protected operand":
				b.OriginPC = 19
			case "other method":
				b.FunctionName = "other"
			case "reference operand":
				b.FuncType.ReturnType = types.NewJavaClass("java.lang.Integer")
			case "opaque substitute":
				v := values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, func() types.JavaType { return types.NewJavaPrimer(types.JavaLong) })
				v.Flag, v.CapturesKnown, v.Captures = "primitive_cast", true, []values.JavaValue{b}
				y = v
			}
			if sameFinallyEffectValue(rows, x, y, nil, 0) != (variant == "original") {
				t.Fatal("conversion or operand effects changed identity/domain")
			}
		})
	}
}
