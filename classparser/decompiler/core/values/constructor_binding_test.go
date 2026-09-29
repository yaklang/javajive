package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestRawConstructorBindingRequiresAllocationAndCompetition(t *testing.T) {
	for _, scenario := range []string{"raw", "null", "array", "parameterized", "this", "other owner", "no competing declaration"} {
		t.Run(scenario, func(t *testing.T) {
			desc := "(Ljava/lang/Object;)V"
			ctx := &class_context.ClassContext{ClassName: "example.Box", MethodDescriptors: map[string]bool{
				class_context.MethodDescKey("<init>", desc):                    true,
				class_context.MethodDescKey("<init>", "(Ljava/lang/String;)V"): true,
			}}
			allocation := NewNewExpression(types.NewJavaClass("example.Box"))
			var receiver JavaValue = NewJavaRef(utils.NewRootVariableId(), allocation, allocation.Type())
			var arg JavaValue = NewJavaLiteral("value", types.NewJavaClass("java.lang.String"))
			want := "Object"
			switch scenario {
			case "null":
				arg = NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
			case "array":
				arg = NewNewArrayExpression(types.NewJavaArrayType(types.NewJavaClass("java.lang.String")))
			case "parameterized":
				allocation.JavaType = types.NewParameterizedType("example.Box", []types.JavaType{types.NewJavaClass("java.lang.String")})
				want = ""
			case "this":
				receiver.(*JavaRef).IsThis = true
				want = ""
			case "other owner":
				allocation.JavaType = types.NewJavaClass("example.Other")
				want = ""
			case "no competing declaration":
				delete(ctx.MethodDescriptors, class_context.MethodDescKey("<init>", "(Ljava/lang/String;)V"))
				want = ""
			}
			call := &FunctionCallExpression{ClassName: "example.Box", FunctionName: "<init>", Descriptor: desc, Kind: InvokeSpecial, Object: receiver}
			if got := call.rawConstructorBindingCast(0, arg, ctx); got != want {
				t.Fatalf("binding cast=%q want=%q", got, want)
			}
		})
	}
}
