package values

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestOptionalGenericThrowsViewRequiresTheExactRawReceiverCall(t *testing.T) {
	for _, kind := range []string{"raw local", "raw chain", "parameterized", "other owner", "other name", "other descriptor", "static", "special", "wrong arity", "unknown receiver", "class literal", "disabled"} {
		t.Run(kind, func(t *testing.T) {
			ctx := &class_context.ClassContext{}
			const desc = "(Ljava/util/function/Supplier;)Ljava/lang/Object;"
			mt, _ := types.ParseMethodDescriptor(desc)
			owner := "java.util.Optional"
			var receiver JavaValue = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(owner))
			switch kind {
			case "raw chain":
				inner, _ := types.ParseMethodDescriptor("()Ljava/util/Optional;")
				receiver = NewFunctionCallExpression(nil, NewJavaClassMember("java.util.stream.Stream", "findFirst", "()Ljava/util/Optional;", inner), inner.FunctionType())
			case "parameterized":
				receiver = NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType(owner, []types.JavaType{types.NewJavaClass("java.lang.String")}))
			case "unknown receiver":
				receiver = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			case "class literal":
				receiver = NewJavaClassValue(types.NewJavaClass(owner))
			case "disabled":
				t.Setenv("JDEC_OPTIONAL_GENERIC_THROWS_OFF", "1")
			}
			call := NewFunctionCallExpression(receiver, NewJavaClassMember(owner, "orElseThrow", desc, mt), mt.FunctionType())
			call.Arguments = []JavaValue{NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("java.util.function.Supplier", []types.JavaType{types.NewJavaClass("java.io.IOException")}))}
			switch kind {
			case "other owner":
				call.ClassName = "example.Optional"
			case "other name":
				call.FunctionName = "orElseGet"
			case "other descriptor":
				call.Descriptor = "()Ljava/lang/Object;"
			case "static":
				call.IsStatic = true
			case "special":
				call.IsSpecialInvoke = true
			case "wrong arity":
				call.Arguments = nil
			}
			view := call.optionalGenericThrowsReceiver(ctx)
			want := kind == "raw local" || kind == "raw chain"
			if (view != nil) != want {
				t.Fatalf("receiver view=%v, want=%t", view, want)
			}
			if want && view.String(ctx) != "Optional<?>" {
				t.Fatalf("invented element type: %s", view.String(ctx))
			}
		})
	}
}

func TestFunctionalReturnSignaturePreservesDescriptorAndVariableScopes(t *testing.T) {
	for _, kind := range []string{"static", "instance", "instantiated receiver", "class variable", "shadowed class variable", "foreign method variable", "shadowed method variable", "foreign static class variable", "other descriptor", "signature erasure mismatch", "unknown signature", "raw signature", "wrong arity", "private bound", "unknown bound", "missing accessibility", "disabled"} {
		t.Run(kind, func(t *testing.T) {
			const desc = "(I)Ljava/util/function/Supplier;"
			ctx := &class_context.ClassContext{ClassName: "example.Caller"}
			ctx.SiblingClassAccessible = func(name string) (bool, bool) {
				return strings.HasPrefix(name, "java/"), name != "example/Unknown"
			}
			sig := "(I)Ljava/util/function/Supplier<Ljava/lang/IllegalStateException;>;"
			ownerSig := "Ljava/lang/Object;"
			var receiver types.JavaType = types.NewJavaClass("example.Factory")
			switch kind {
			case "instantiated receiver", "class variable", "shadowed class variable", "shadowed method variable", "foreign static class variable":
				ownerSig = "<R:Ljava/lang/Object;>Ljava/lang/Object;"
				sig = "(I)Ljava/util/function/Supplier<TR;>;"
				receiver = types.NewParameterizedType("example.Factory", []types.JavaType{types.NewJavaClass("java.io.IOException")})
				if kind == "class variable" || kind == "shadowed class variable" {
					ctx.TypeParams, ctx.ClassTypeParams = []string{"R"}, []string{"R"}
					receiver = types.NewParameterizedType("example.Factory", []types.JavaType{types.NewJavaClass("R")})
				}
				if kind == "shadowed class variable" {
					ctx.CurrentMethodSig = "<R:Ljava/lang/Object;>()V"
				}
				if kind == "shadowed method variable" {
					sig = "<R:Ljava/lang/Object;>" + sig
				}
				if kind == "foreign static class variable" {
					ctx.TypeParams = []string{"R"}
				}
			case "foreign method variable":
				ctx.TypeParams = []string{"R"}
				sig = "<R:Ljava/lang/Object;>(I)Ljava/util/function/Supplier<TR;>;"
			case "signature erasure mismatch":
				sig = "(I)Ljava/util/function/Function<Ljava/lang/String;Ljava/lang/String;>;"
			case "unknown signature":
				sig = ""
			case "raw signature":
				sig = desc
			case "private bound":
				sig = "(I)Ljava/util/function/Supplier<Lexample/Hidden;>;"
			case "unknown bound":
				sig = "(I)Ljava/util/function/Supplier<Lexample/Unknown;>;"
			case "missing accessibility":
				ctx.SiblingClassAccessible = nil
			case "disabled":
				t.Setenv("JDEC_FUNCTIONAL_RETURN_SIGNATURE_OFF", "1")
			}
			ctx.SiblingClassSig = func(name string) (string, map[string]string, bool) {
				return ownerSig, map[string]string{class_context.MethodDescKey("make", desc): sig}, name == "example/Factory"
			}
			mt, _ := types.ParseMethodDescriptor(desc)
			call := NewFunctionCallExpression(NewJavaRef(utils.NewRootVariableId(), nil, receiver), NewJavaClassMember("example.Factory", "make", desc, mt), mt.FunctionType())
			call.IsStatic = kind != "instance" && kind != "instantiated receiver" && kind != "class variable" && kind != "shadowed class variable" && kind != "shadowed method variable"
			call.Arguments = []JavaValue{NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))}
			if kind == "other descriptor" {
				call.Descriptor = "(I)Ljava/lang/Object;"
			}
			if kind == "wrong arity" {
				call.Arguments = nil
			}
			call.RetainFunctionalReturnSignature(ctx)
			want := kind == "static" || kind == "instance" || kind == "instantiated receiver" || kind == "class variable"
			if (call.SourceReturnType != nil) != want {
				t.Fatalf("source return=%v, want=%t", call.SourceReturnType, want)
			}
			if call.FuncType.ReturnType.String(ctx) != "Supplier" {
				t.Fatal("raw invoke descriptor return mutated")
			}
			if call.Type().String(ctx) != "Supplier" {
				t.Fatal("declaration evidence narrowed a raw result")
			}
			if kind == "instantiated receiver" && call.SourceReturnType.String(ctx) != "Supplier<IOException>" {
				t.Fatalf("receiver substitution lost: %s", call.SourceReturnType.String(ctx))
			}
			if want {
				clone := call.Clone()
				clone.SourceReturnType.ResetType(types.NewJavaClass("java.lang.Object"))
				if call.SourceReturnType.String(ctx) == "Object" {
					t.Fatal("clone shared a mutable source type")
				}
			}
		})
	}
}
