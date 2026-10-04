package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestStreamFunctionBridgeRequiresIdenticalErasure(t *testing.T) {
	for _, scenario := range []string{"raw input", "typed input", "different input", "wildcard input", "wildcard function", "unknown stream", "wrong descriptor", "different owner", "poly expression"} {
		t.Run(scenario, func(t *testing.T) {
			stringType := types.NewJavaClass("java.lang.String")
			input := types.NewParameterizedType("java.util.Optional", []types.JavaType{stringType})
			element := types.NewJavaClass("java.util.Optional")
			switch scenario {
			case "typed input":
				element = types.NewParameterizedType("java.util.Optional", []types.JavaType{types.NewJavaClass("java.lang.Integer")})
			case "different input":
				element = types.NewJavaClass("java.lang.String")
			case "wildcard input":
				element = &types.JavaWildcardType{}
			case "wildcard function":
				input = &types.JavaWildcardType{Bound: input, Variant: "super"}
			}
			receiver := types.NewParameterizedType("java.util.stream.Stream", []types.JavaType{element})
			if scenario == "unknown stream" {
				receiver = types.NewJavaClass("java.util.stream.Stream")
			}
			fnType := types.NewParameterizedType("java.util.function.Function", []types.JavaType{input, stringType})
			var fn JavaValue = NewJavaRef(utils.NewRootVariableId(), nil, fnType)
			call := &FunctionCallExpression{
				ClassName: "java.util.stream.Stream", FunctionName: "map", Descriptor: streamMapDescriptor,
				Object: NewJavaRef(utils.NewRootVariableId(), nil, receiver),
			}
			switch scenario {
			case "wrong descriptor":
				call.Descriptor = "(Ljava/lang/Object;)Ljava/util/stream/Stream;"
			case "different owner":
				call.ClassName = "example.Stream"
			case "poly expression":
				fn = NewCustomValue(func(*class_context.ClassContext) string { return "Optional::get" }, func() types.JavaType { return fnType })
			}
			call.Arguments = []JavaValue{fn}
			got := call.streamFunctionInputBridge(0)
			if scenario != "raw input" {
				if got != nil {
					t.Fatal("bridged without proof of matching erased input")
				}
				return
			}
			if got == nil || got.String(&class_context.ClassContext{}) != "Function<Optional, String>" {
				t.Fatalf("bridge=%v", got)
			}
		})
	}
}
