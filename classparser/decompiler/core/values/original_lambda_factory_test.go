package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestOriginalLambdaFactoryPinsOnlyEagerEvaluation(t *testing.T) {
	for _, variant := range []string{"original", "type projection", "defensive result", "second registration", "unmarked", "negative pc", "oversize pc", "wrong registration pc", "unknown captures", "nil capture", "typed nil capture", "too many captures", "changed pc", "missing pc", "changed flag", "changed captures known", "changed operand", "reordered operands", "added operand", "removed operand", "changed reference kind", "changed outer capture", "changed descriptor", "equal public fields"} {
		t.Run(variant, func(t *testing.T) {
			operand := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("p.Receiver"))
			other := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("p.Receiver"))
			v := NewStreamingCustomValue(func(*class_context.ClassContext, *workbudget.Writer) error {
				panic("analysis must not render deferred body")
			}, func() types.JavaType { panic("analysis must not call Type callback") })
			v.Flag, v.HasOriginPC, v.OriginPC, v.CapturesKnown, v.Captures = "lambda", true, 12, true, []JavaValue{operand, other}
			v.InstantiatedMtdDesc = "(I)I"
			pc := 12
			switch variant {
			case "negative pc":
				pc = -1
				v.OriginPC = pc
			case "oversize pc":
				pc = 65536
				v.OriginPC = pc
			case "wrong registration pc":
				pc++
			case "unknown captures":
				v.CapturesKnown = false
			case "nil capture":
				v.Captures[0] = nil
			case "typed nil capture":
				v.Captures[0] = (*JavaRef)(nil)
			case "too many captures":
				for len(v.Captures) < 65 {
					v.Captures = append(v.Captures, operand)
				}
			}
			if variant != "unmarked" {
				v.MarkOriginalLambdaFactory(pc)
			}
			switch variant {
			case "type projection":
				v = v.WithType(func() types.JavaType { panic("projected type must not execute") })
			case "defensive result":
				_, args, ok := v.OriginalLambdaFactoryEvaluation()
				if !ok {
					t.Fatal("missing original")
				}
				args[0] = other
			case "second registration":
				v.MarkOriginalLambdaFactory(19)
			case "changed pc":
				v.OriginPC++
			case "missing pc":
				v.HasOriginPC = false
			case "changed flag":
				v.Flag = "instanceof"
			case "changed captures known":
				v.CapturesKnown = false
			case "changed operand":
				v.Captures[0] = other
			case "reordered operands":
				v.Captures[0], v.Captures[1] = v.Captures[1], v.Captures[0]
			case "added operand":
				v.Captures = append(v.Captures, operand)
			case "removed operand":
				v.Captures = v.Captures[:1]
			case "changed reference kind":
				v.IsMethodRef = true
			case "changed outer capture":
				v.NoOuterCapture = true
			case "changed descriptor":
				v.InstantiatedMtdDesc = "()V"
			case "equal public fields":
				v = &CustomValue{Flag: v.Flag, HasOriginPC: v.HasOriginPC, OriginPC: v.OriginPC, CapturesKnown: v.CapturesKnown, Captures: v.Captures, InstantiatedMtdDesc: v.InstantiatedMtdDesc}
			}
			location, captures, known := v.OriginalLambdaFactoryEvaluation()
			want := variant == "original" || variant == "type projection" || variant == "defensive result" || variant == "second registration"
			if known != want || known && (location != 12 || len(captures) != 2 || captures[0] != operand || captures[1] != other) {
				t.Fatalf("eager witness %d/%d/%v want %v", location, len(captures), known, want)
			}
			if known && MayFold(v) {
				t.Fatal("original eager allocation cannot become pure/movable")
			}
		})
	}
}
