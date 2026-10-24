package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestFinallyLambdaCaptureInlineNeedsOriginalPCOrdinalAndSeed(t *testing.T) {
	for _, variant := range []string{"original", "unmarked", "wrong PC", "wrong ordinal", "changed source", "changed UID", "opaque source", "protected eager source"} {
		t.Run(variant, func(t *testing.T) {
			id := values.NewLambdaSourceIdentity()
			parameter := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaLong))
			parameter.IsParam = true
			makeFactory := func(pc int) *values.CustomValue {
				var seed values.JavaValue = parameter
				if variant == "protected eager source" {
					seed = &values.FunctionCallExpression{ClassName: "p/Source", FunctionName: "read", Descriptor: "()J", Kind: values.InvokeStatic, IsStatic: true, HasOriginPC: true, OriginPC: 11, FuncType: types.NewJavaFuncType("()J", nil, parameter.Type())}
				}
				snapshot := values.NewJavaRef(utils.NewRootVariableId(), seed, parameter.Type())
				ordinal, origin := 0, pc
				if variant == "wrong ordinal" {
					ordinal = 1
				}
				if variant == "wrong PC" {
					origin++
				}
				if variant != "unmarked" {
					snapshot.MarkOriginalDynamicOperand(origin, ordinal, seed)
				}
				snapshot.StackVar = seed
				snapshot.MarkOriginalDynamicOperandInline(origin, seed)
				switch variant {
				case "changed source":
					snapshot.StackVar = values.JavaNull
				case "changed UID":
					snapshot.VarUid += "::other"
				case "opaque source":
					snapshot.StackVar = values.NewCustomValue(func(*class_context.ClassContext) string { panic("proof must not render") }, func() types.JavaType { panic("proof must not query an opaque type") })
				}
				v := values.NewCustomValue(func(*class_context.ClassContext) string { panic("deferred body must not render") }, func() types.JavaType { panic("factory proof must not query type callback") })
				v.Flag, v.OriginPC, v.HasOriginPC, v.CapturesKnown, v.Captures = "lambda", pc, true, true, []values.JavaValue{snapshot}
				v.InstantiatedMtdDesc = "(J)V"
				v.BindLambdaSourceIdentity(id)
				v.MarkOriginalLambdaFactory(pc)
				v.BindOriginalLambdaFactorySignature("accept", "(J)Ljava/util/function/LongConsumer;", "(J)V")
				return v
			}
			a, b := makeFactory(12), makeFactory(42)
			got := sameFinallyEffectValue([]core.HandlerRange{{StartPc: 10, EndPc: 12}, {StartPc: 20, EndPc: 40}}, a, b, nil, 0)
			if got != (variant == "original") {
				t.Fatalf("inline capture equivalence=%v; variant=%s", got, variant)
			}
			covered := finallyCoveredValue(a, func(pc int) bool { return pc == 12 })
			if covered != (variant == "original") {
				t.Fatalf("protected inline capture domain=%v; variant=%s", covered, variant)
			}
		})
	}
}
