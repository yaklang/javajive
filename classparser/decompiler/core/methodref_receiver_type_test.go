package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestMethodRefReceiverRequiresDeclarationEvidence(t *testing.T) {
	for _, scenario := range []string{"declared", "captured", "static", "no signature", "other overload", "method formal", "concrete return", "bounded return", "different receiver", "different SAM receiver", "SAM arity", "already parameterized"} {
		t.Run(scenario, func(t *testing.T) {
			stringType := types.NewJavaClass("java.lang.String")
			fi := types.NewParameterizedType("java.util.function.Function", []types.JavaType{types.NewJavaClass("example.Box"), stringType})
			impl := &values.JavaClassMember{Name: "example/Box", Member: "read", Description: "()Ljava/lang/Object;", RefKind: RefInvokeVirtual}
			sig := "()TX;"
			key := class_context.MethodDescKey("read", impl.Description)
			captures := 0
			desc := "(Lexample/Box;)Ljava/lang/String;"
			switch scenario {
			case "captured":
				captures = 1
			case "static":
				impl.RefKind = RefInvokeStatic
			case "no signature":
				sig = ""
			case "other overload":
				key = class_context.MethodDescKey("read", "(I)Ljava/lang/Object;")
			case "method formal":
				sig = "<X:Ljava/lang/Object;>()TX;"
			case "concrete return":
				sig = "()Ljava/lang/Object;"
			case "bounded return":
				impl.Description = "()Ljava/lang/Number;"
			case "different receiver":
				impl.Name = "example/Other"
			case "different SAM receiver":
				desc = "(Lexample/Other;)Ljava/lang/String;"
			case "SAM arity":
				desc = "(Lexample/Box;I)Ljava/lang/String;"
			case "already parameterized":
				fi = types.NewParameterizedType("java.util.function.Function", []types.JavaType{types.NewParameterizedType("example.Box", []types.JavaType{stringType, stringType}), stringType})
			}
			ctx := &class_context.ClassContext{SiblingClassSig: func(owner string) (string, map[string]string, bool) {
				return "<X:Ljava/lang/Object;Y:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{key: sig}, owner == "example/Box"
			}}
			instantiated := values.NewCustomValue(func(*class_context.ClassContext) string { return desc }, func() types.JavaType { return types.NewJavaClass("java.lang.invoke.MethodType") })
			got := methodRefReceiverType(ctx, fi, impl, instantiated, captures)
			if scenario != "declared" {
				if got != fi {
					t.Fatal("changed receiver without sufficient declaration evidence")
				}
				return
			}
			fp, _ := types.AsParameterizedType(got)
			receiver, ok := types.AsParameterizedType(fp.TypeArgs[0])
			if !ok || receiver.RawClassName != "example.Box" || len(receiver.TypeArgs) != 2 {
				t.Fatalf("missing generic receiver: %v", got)
			}
			if name, _ := types.ClassFQNOf(receiver.TypeArgs[0]); name != "java.lang.String" {
				t.Fatal("returned class variable did not bind to SAM result")
			}
			if _, ok := receiver.TypeArgs[1].(*types.JavaWildcardType); !ok {
				t.Fatal("invented an invariant type for the unrelated class variable")
			}
		})
	}
}
