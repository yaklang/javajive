package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestFinallyFactoryCoverageRequiresOriginalEagerOperands(t *testing.T) {
	for _, variant := range []string{"original", "unmarked", "factory outside", "capture outside", "changed capture", "unknown captures", "opaque capture", "cycle"} {
		t.Run(variant, func(t *testing.T) {
			receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("p.Owner"))
			v := values.NewCustomValue(func(*class_context.ClassContext) string { panic("deferred body must not render") }, func() types.JavaType { panic("opaque type callback must not execute") })
			v.Flag, v.CapturesKnown, v.Captures, v.HasOriginPC, v.OriginPC = "lambda", true, []values.JavaValue{receiver}, true, 12
			if variant == "capture outside" {
				v.Captures[0] = numericConversionCall(19)
			}
			if variant == "opaque capture" {
				v.Captures[0] = &values.CustomValue{}
			}
			if variant == "cycle" {
				v.Captures[0] = v
			}
			if variant != "unmarked" {
				v.MarkOriginalLambdaFactory(12)
			}
			if variant == "changed capture" {
				v.Captures[0] = values.NewJavaRef(utils.NewRootVariableId(), nil, receiver.Type())
			}
			if variant == "unknown captures" {
				v.CapturesKnown = false
			}
			want := variant == "original"
			if got := finallyCoveredValue(v, func(pc int) bool { return pc == 12 && variant != "factory outside" }); got != want {
				t.Fatalf("coverage=%v want=%v", got, want)
			}
		})
	}
}
