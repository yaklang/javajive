package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestLambdaSourceIdentityRequiresBodyFactoryAndSAM(t *testing.T) {
	for _, variant := range []string{"same", "different pc", "unsealed", "different body", "copied body token", "missing body", "different SAM name", "different factory descriptor", "different erased SAM", "different instantiated SAM", "changed capture", "method reference", "unknown capture", "changed PC", "signature immutable"} {
		t.Run(variant, func(t *testing.T) {
			id := NewLambdaSourceIdentity()
			capture := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("p.Owner"))
			makeValue := func(pc int) *CustomValue {
				v := NewCustomValue(func(*class_context.ClassContext) string { panic("proof must not render") }, func() types.JavaType { panic("proof must not query callback type") })
				v.Flag, v.CapturesKnown, v.Captures, v.HasOriginPC, v.OriginPC = "lambda", true, []JavaValue{capture}, true, pc
				v.InstantiatedMtdDesc = "(J)J"
				v.BindLambdaSourceIdentity(id)
				v.MarkOriginalLambdaFactory(pc)
				v.BindOriginalLambdaFactorySignature("applyAsLong", "(Lp/Owner;)Ljava/util/function/LongUnaryOperator;", "(J)J")
				return v
			}
			a, b := makeValue(12), makeValue(12)
			switch variant {
			case "different pc":
				b = makeValue(42)
			case "unsealed":
				b.originalLambdaFactory = nil
			case "different body":
				b.lambdaSourceIdentity = NewLambdaSourceIdentity()
			case "copied body token":
				copy := *id
				a.lambdaSourceIdentity, b.lambdaSourceIdentity = &copy, &copy
			case "missing body":
				b.lambdaSourceIdentity = nil
			case "different SAM name":
				b.originalLambdaFactory.signature[0] = "other"
			case "different factory descriptor":
				b.originalLambdaFactory.signature[1] = "()Ljava/util/function/LongUnaryOperator;"
			case "different erased SAM":
				b.originalLambdaFactory.signature[2] = "(I)I"
			case "different instantiated SAM":
				b.InstantiatedMtdDesc = "(I)I"
			case "changed capture":
				b.Captures[0] = JavaNull
			case "method reference":
				b.IsMethodRef = true
			case "unknown capture":
				b.CapturesKnown = false
			case "changed PC":
				b.OriginPC++
			case "signature immutable":
				b.BindOriginalLambdaFactorySignature("other", "()V", "()V")
			}
			want := variant == "same" || variant == "different pc" || variant == "signature immutable"
			if a.SameOriginalLambdaSource(b) != want || b.SameOriginalLambdaSource(a) != want {
				t.Fatalf("deferred body identity=%v; want %v", a.SameOriginalLambdaSource(b), want)
			}
		})
	}
}
