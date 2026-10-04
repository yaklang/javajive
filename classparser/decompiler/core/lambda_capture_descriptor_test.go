package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestLambdaCaptureDescriptorViewsKeepSnapshotsAndGenericTypes(t *testing.T) {
	for _, scenario := range []string{"null-only", "matching generic", "Object formal", "instance receiver", "bad descriptor", "unsupported handle", "missing capture parameter"} {
		t.Run(scenario, func(t *testing.T) {
			impl := &values.JavaClassMember{Name: "p.Owner", Member: "lambda$body", Description: "(Ljava/lang/ClassLoader;)Ljava/lang/Object;", RefKind: RefInvokeStatic}
			ref := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaClass("java.lang.Object"))
			cast := scenario == "null-only" || scenario == "instance receiver"
			switch scenario {
			case "matching generic":
				impl.Description = "(Ljava/util/List;)Ljava/lang/Object;"
				ref.ResetVarType(types.NewParameterizedType("java.util.List", []types.JavaType{types.NewJavaClass("java.lang.String")}))
			case "Object formal":
				impl.Description = "(Ljava/lang/Object;)Ljava/lang/Object;"
			case "instance receiver":
				impl.RefKind = RefInvokeVirtual
				impl.Description = "()Ljava/lang/Object;"
			case "bad descriptor":
				impl.Description = "bad"
			case "unsupported handle":
				impl.RefKind = RefNewInvokeSpecial
			case "missing capture parameter":
				impl.Description = "()Ljava/lang/Object;"
			}
			before := ref.Type().String(&class_context.ClassContext{})
			input := []values.JavaValue{ref}
			out := lambdaCaptureDescriptorViews(impl, input)
			if input[0] != ref || ref.Type().String(&class_context.ClassContext{}) != before {
				t.Fatal("changed shared capture identity/type")
			}
			if !cast {
				if out[0] != ref {
					t.Fatal("unproved capture changed")
				}
				return
			}
			view, ok := out[0].(*values.CastExpression)
			if !ok || !view.Binding || view.Value != ref {
				t.Fatal("snapshot identity lost")
			}
			want := "java.lang.ClassLoader"
			if scenario == "instance receiver" {
				want = "p.Owner"
			}
			got, _ := types.RawClassFQN(view.TargetType)
			if got != want {
				t.Fatalf("target=%s want %s", got, want)
			}
		})
	}
}
