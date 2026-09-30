package values

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestFunctionalErasureUsesSymbolicShapeWithoutTargetingForeignVariables(t *testing.T) {
	for _, kind := range []string{"stream", "optional", "external generic", "shadowed generic", "other owner", "other descriptor", "other name", "wrong staticness", "poly lambda", "poly method reference", "concrete mismatch", "descriptor mismatch", "complete nested arguments", "unknown signature", "foreign target", "disabled"} {
		t.Run(kind, func(t *testing.T) {
			owner, name := "java.util.stream.Stream", "flatMap"
			desc := "(Ljava/util/function/Function;)Ljava/util/stream/Stream;"
			input, output := types.NewJavaClass("java.util.List"), types.NewJavaClass("java.util.stream.Stream")
			ctx := &class_context.ClassContext{ClassName: "example.Caller"}
			if kind == "shadowed generic" || kind == "foreign target" {
				ctx.TypeParams, ctx.ClassTypeParams = []string{"R"}, []string{"R"}
			}
			sig := "<R:Ljava/lang/Object;>(Ljava/util/function/Function<Ljava/util/List<Ljava/lang/String;>;Ljava/util/stream/Stream<TR;>;>;)Ljava/util/List<TR;>;"
			switch kind {
			case "optional":
				owner, desc, output = "java.util.Optional", "(Ljava/util/function/Function;)Ljava/util/Optional;", types.NewJavaClass("java.util.Optional")
			case "external generic", "shadowed generic", "concrete mismatch", "unknown signature", "foreign target":
				owner, name, desc = "example.Pipeline", "expand", "(Ljava/util/function/Function;)Ljava/util/List;"
			case "other owner":
				owner = "example.Stream"
			case "other descriptor":
				desc = "(Ljava/util/function/Function;I)Ljava/util/stream/Stream;"
			case "other name":
				name = "map"
			case "descriptor mismatch":
				desc = "(Ljava/util/function/Predicate;)Ljava/util/stream/Stream;"
			case "complete nested arguments":
				output = types.NewParameterizedType("java.util.stream.Stream", []types.JavaType{types.NewJavaClass("java.lang.String")})
			case "disabled":
				t.Setenv("JDEC_FUNCTIONAL_ERASURE_ARG_CAST_OFF", "1")
			}
			if kind == "concrete mismatch" {
				input = types.NewParameterizedType("java.util.List", []types.JavaType{types.NewJavaClass("java.lang.String")})
				output = types.NewParameterizedType("java.util.stream.Stream", []types.JavaType{types.NewJavaClass("java.lang.Integer")})
				sig = "(Ljava/util/function/Function<Ljava/util/List<Ljava/lang/String;>;Ljava/util/stream/Stream<Ljava/lang/String;>;>;)Ljava/util/List<Ljava/lang/String;>;"
			}
			if kind == "unknown signature" {
				sig = ""
			}
			ownerSig := "Ljava/lang/Object;"
			var receiverType types.JavaType = types.NewJavaClass(owner)
			if kind == "shadowed generic" {
				ownerSig = "<R:Ljava/lang/Object;>Ljava/lang/Object;"
				receiverType = types.NewParameterizedType(owner, []types.JavaType{types.NewJavaClass("java.lang.Integer")})
			}
			ctx.SiblingClassSig = func(internal string) (string, map[string]string, bool) {
				if internal != "example/Pipeline" {
					return "", nil, false
				}
				return ownerSig, map[string]string{class_context.MethodDescKey("expand", desc): sig}, true
			}
			typ, err := types.ParseMethodDescriptor(desc)
			if err != nil {
				t.Fatal(err)
			}
			member := NewJavaClassMember(owner, name, desc, typ)
			call := NewFunctionCallExpression(NewJavaRef(utils.NewRootVariableId(), nil, receiverType), member, typ.FunctionType())
			call.Descriptor, call.Kind = desc, InvokeInterface
			fnType := types.NewParameterizedType("java.util.function.Function", []types.JavaType{input, output})
			var argument JavaValue = NewJavaRef(utils.NewRootVariableId(), nil, fnType)
			if kind == "poly lambda" || kind == "poly method reference" {
				poly := NewCustomValue(func(*class_context.ClassContext) string { return "List::stream" }, func() types.JavaType { return fnType })
				poly.Flag, poly.IsMethodRef = "lambda", kind == "poly method reference"
				argument = poly
			}
			call.Arguments = []JavaValue{argument}
			if kind == "wrong staticness" {
				call.IsStatic = true
			}
			got := call.nestedGenericErasureArgCast(0, argument, ctx)
			want := kind == "stream" || kind == "optional" || kind == "external generic" || kind == "shadowed generic" || kind == "foreign target"
			if (got != "") != want {
				t.Fatalf("erasure bridge=%q, want=%t", got, want)
			}
			if kind == "foreign target" || kind == "shadowed generic" {
				if formal := call.functionalFormalForErasure(0, ctx); formal == nil || !javaTypeMentionsNames(formal, []string{"R"}) {
					t.Fatal("method R was replaced by the caller or receiver R")
				}
				if target := call.FunctionalTargetParamType(0, ctx); target != nil {
					t.Fatalf("callee R leaked into caller R: %s", target.String(ctx))
				}
				if text := call.renderArgAt(0, ctx); strings.Contains(text, "<R>") {
					t.Fatalf("symbolic method variable printed: %s", text)
				}
			}
		})
	}
}

func TestFunctionalErasureCollectorResultNeedsAnExactProducerWitness(t *testing.T) {
	for _, kind := range []string{"list factory", "set factory", "typed collector", "raw local", "stale local definition", "other factory owner", "other factory name", "other factory descriptor", "instance factory", "factory argument", "other consumer owner", "other consumer name", "other consumer descriptor", "instance consumer", "raw collector result", "concrete mismatch"} {
		t.Run(kind, func(t *testing.T) {
			ctx := &class_context.ClassContext{}
			makeCall := func(owner, name, desc string, static bool) *FunctionCallExpression {
				mt, err := types.ParseMethodDescriptor(desc)
				if err != nil {
					t.Fatal(err)
				}
				call := NewFunctionCallExpression(nil, NewJavaClassMember(owner, name, desc, mt), mt.FunctionType())
				call.Descriptor, call.IsStatic = desc, static
				return call
			}
			factory := makeCall("java.util.stream.Collectors", "toList", "()Ljava/util/stream/Collector;", true)
			var producer JavaValue = factory
			resultRaw := "java.util.List"
			switch kind {
			case "set factory":
				factory.FunctionName, resultRaw = "toSet", "java.util.Set"
			case "typed collector", "raw collector result", "concrete mismatch":
				var result types.JavaType = types.NewParameterizedType(resultRaw, []types.JavaType{types.NewJavaClass("java.lang.String")})
				if kind == "raw collector result" {
					result = types.NewJavaClass(resultRaw)
				}
				collector := types.NewParameterizedType("java.util.stream.Collector", []types.JavaType{types.NewJavaClass("java.lang.String"), &types.JavaWildcardType{}, result})
				producer = NewJavaRef(utils.NewRootVariableId(), nil, collector)
			case "raw local":
				producer = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.util.stream.Collector"))
			case "stale local definition":
				producer = NewJavaRef(utils.NewRootVariableId(), factory, types.NewJavaClass("java.util.stream.Collector"))
			case "other factory owner":
				factory.ClassName = "example.Collectors"
			case "other factory name":
				factory.FunctionName = "toCollection"
			case "other factory descriptor":
				factory.Descriptor = "()Ljava/lang/Object;"
			case "instance factory":
				factory.IsStatic = false
			case "factory argument":
				factory.Arguments = []JavaValue{NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))}
			}
			call := makeCall("java.util.stream.Collectors", "collectingAndThen", "(Ljava/util/stream/Collector;Ljava/util/function/Function;)Ljava/util/stream/Collector;", true)
			switch kind {
			case "other consumer owner":
				call.ClassName = "example.Collectors"
			case "other consumer name":
				call.FunctionName = "mapping"
			case "other consumer descriptor":
				call.Descriptor = "(Ljava/util/stream/Collector;Ljava/util/function/UnaryOperator;)Ljava/util/stream/Collector;"
			case "instance consumer":
				call.IsStatic = false
			}
			var input types.JavaType = types.NewJavaClass(resultRaw)
			if kind == "concrete mismatch" {
				input = types.NewParameterizedType(resultRaw, []types.JavaType{types.NewJavaClass("java.lang.Integer")})
			}
			finisher := NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("java.util.function.Function", []types.JavaType{input, types.NewJavaClass(resultRaw)}))
			call.Arguments = []JavaValue{producer, finisher}
			got := call.nestedGenericErasureArgCast(1, finisher, ctx)
			want := kind == "list factory" || kind == "set factory" || kind == "typed collector"
			if (got != "") != want {
				t.Fatalf("erasure bridge=%q, want=%t", got, want)
			}
		})
	}
}
