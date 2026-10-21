package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestCapturedFieldSourceUsesTheSuppliedMethodContext(t *testing.T) {
	for _, variant := range []string{"current", "snapshot after sibling", "foreign method", "foreign owner", "wrong read pc", "legacy only"} {
		t.Run(variant, func(t *testing.T) {
			live := &class_context.ClassContext{ClassName: "fixture.Owner", FunctionName: "factory", CurrentMethodDesc: "()V"}
			legacyCalls, scopedCalls := 0, 0
			var observed *class_context.ClassContext
			live.SourceCapturedField = func(int, string, bool) (string, bool) {
				legacyCalls++
				return "legacy", true
			}
			if variant != "legacy only" {
				live.SourceCapturedFieldAt = func(source *class_context.ClassContext, pc int, name string, receiver bool) (string, bool) {
					scopedCalls++
					observed = source
					if source.ClassName == "fixture.Owner" && source.FunctionName == "factory" && source.CurrentMethodDesc == "()V" && pc == 2 && name == "hidden" && receiver {
						return "captured", true
					}
					return "", false
				}
			}
			view := live
			if variant == "snapshot after sibling" {
				var known bool
				view, known = live.SnapshotForSourceProof(nil)
				if !known {
					t.Fatal("original method snapshot")
				}
				live.FunctionName, live.CurrentMethodDesc = "sibling", "(I)V"
			} else if variant == "foreign method" {
				view.FunctionName = "foreign"
			} else if variant == "foreign owner" {
				view.ClassName = "fixture.Other"
			}
			typ := types.NewJavaPrimer(types.JavaLong)
			receiver := NewJavaRef(utils.NewRootVariableId(), JavaNull, types.NewJavaClass("fixture.Owner"))
			receiver.IsThis = true
			field := NewRefMember(receiver, "hidden", typ)
			field.OriginPC, field.HasOriginPC = 2, true
			field.MarkOriginalFieldRead(NewJavaClassMember("fixture.Owner", "hidden", "J", typ), 2)
			if variant == "wrong read pc" {
				field.OriginPC++
			}
			text := field.String(view)
			if variant == "legacy only" {
				if text != "legacy" || legacyCalls != 1 || scopedCalls != 0 {
					t.Fatal("legacy-only source reader changed")
				}
				return
			}
			want := variant == "current" || variant == "snapshot after sibling"
			if (text == "captured") != want || text == "legacy" || scopedCalls != 1 || legacyCalls != 0 || observed != view {
				t.Fatal("source reader borrowed another method or retried its live legacy closure")
			}
		})
	}
}
