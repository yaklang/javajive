package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestSerializableReferenceSourceTokenRequiresUnchangedOriginalFactory(t *testing.T) {
	for _, change := range []string{"original", "copied source type", "missing original", "missing token", "changed pc", "changed invocation kind", "changed instantiated type", "changed captures", "lost capture proof", "changed outer capture"} {
		t.Run(change, func(t *testing.T) {
			calls := 0
			value := NewStreamingCustomValue(func(*class_context.ClassContext, *workbudget.Writer) error { calls++; return nil }, func() types.JavaType { calls++; return types.NewJavaClass("java.util.function.Function") }, nil)
			value.Flag = "lambda"
			value.IsMethodRef = true
			value.NoOuterCapture = true
			value.CapturesKnown = true
			value.OriginPC = 17
			value.HasOriginPC = true
			value.InstantiatedMtdDesc = "(Ljava/lang/String;)Ljava/lang/String;"
			if change != "missing original" {
				value.MarkOriginalLambdaFactory(17)
			}
			token := &struct{ identity int }{1}
			var tag any = token
			if change == "missing token" {
				tag = nil
			}
			got := NewSerializableReferenceSource(value, tag)
			if change == "missing original" || change == "missing token" {
				if got != nil {
					t.Fatal("annotations granted a source token")
				}
				if calls != 0 {
					t.Fatal("source capability ran callbacks")
				}
				return
			}
			if got == nil {
				t.Fatal("original direct reference rejected")
			}
			switch change {
			case "copied source type":
				got = got.WithType(func() types.JavaType { return types.NewJavaClass("java.lang.Object") })
			case "changed pc":
				got.OriginPC++
			case "changed invocation kind":
				got.IsMethodRef = false
			case "changed instantiated type":
				got.InstantiatedMtdDesc = "()V"
			case "changed captures":
				got.Captures = []JavaValue{value}
			case "lost capture proof":
				got.CapturesKnown = false
			case "changed outer capture":
				got.NoOuterCapture = false
			}
			actual, known := got.SerializableReferenceSource()
			want := change == "original" || change == "copied source type"
			if known != want || known && actual != token {
				t.Fatal("changed factory borrowed a source packet", known, want)
			}
			if calls != 0 {
				t.Fatal("source capability ran callbacks")
			}
		})
	}
}
