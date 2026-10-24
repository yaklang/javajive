package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestOriginalLambdaResultNeedsProducerAndSourceDeclaration(t *testing.T) {
	for _, variant := range []string{"original", "source clone", "shallow copy", "matching public fields", "wrong producer PC", "changed UID", "changed ID", "changed value", "changed capture", "changed factory PC", "source alias changed UID", "ordinary local", "parameter", "receiver", "stack view"} {
		t.Run(variant, func(t *testing.T) {
			v := NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, func() types.JavaType { panic("must not query factory type") })
			v.Flag, v.CapturesKnown, v.HasOriginPC, v.OriginPC = "lambda", true, true, 12
			v.MarkOriginalLambdaFactory(12)
			typ := types.NewJavaClass("java.util.function.LongConsumer")
			ref := NewJavaRef(utils.NewRootVariableId(), v, typ)
			if variant != "matching public fields" {
				pc := 12
				if variant == "wrong producer PC" {
					pc = 13
				}
				ref.MarkOriginalLambdaResult(pc, v)
			}
			switch variant {
			case "source clone", "source alias changed UID":
				ref = ref.CloneForSourceDeclaration()
				if variant == "source alias changed UID" {
					ref.VarUid += "::foreign"
				}
			case "shallow copy":
				copy := *ref
				ref = &copy
			case "changed UID":
				ref.VarUid += "::foreign"
			case "changed ID":
				ref = ref.CloneForSourceDeclaration()
				ref.Id = utils.NewRootVariableId()
			case "changed value":
				ref.Val = JavaNull
			case "changed capture":
				v.Captures = []JavaValue{JavaNull}
			case "changed factory PC":
				v.OriginPC++
			case "ordinary local":
				ref.MarkOriginalLocalDeclaration(12, 2, v)
			case "parameter":
				ref.IsParam = true
			case "receiver":
				ref.IsThis = true
			case "stack view":
				ref.StackVar = v
			}
			pc, known := ref.OriginalLambdaResult(v)
			want := variant == "original" || variant == "source clone"
			if known != want || known && pc != 12 {
				t.Fatalf("original result=%v pc=%d; want %v", known, pc, want)
			}
			if _, _, stackRole := ref.OriginalStackMaterializationWitness(v); stackRole {
				t.Fatal("lambda-result evidence must not grant DUP/type-sharing authority")
			}
		})
	}
}
