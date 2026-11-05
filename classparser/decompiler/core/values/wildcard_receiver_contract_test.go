package values

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Neither a type's spelling nor the position of its wildcard says which
// formal a method consumes. Missing declaration evidence cannot grant a raw
// receiver cast (which could also alter overloads, results, or checked throws).
func TestWildcardReceiverSpellingCannotAuthorizeErasure(t *testing.T) {
	for _, owner := range []string{"Transformer", "example.Transformer", "example.Callback", "org.apache.commons.collections4.Transformer", "com.google.common.base.Predicate", "java.util.function.Predicate", "java.util.function.Consumer"} {
		for _, method := range []string{"transform", "apply", "test", "accept", "deliver"} {
			t.Run(owner+"/"+method, func(t *testing.T) {
				ctx := &class_context.ClassContext{ClassName: "example.Caller"}
				receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType(owner, []types.JavaType{&types.JavaWildcardType{Variant: "extends", Bound: types.NewJavaClass("java.io.IOException")}}))
				receiver.Id.SetName("receiver")
				arg := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
				arg.Id.SetName("value")
				const desc = "(Ljava/lang/Object;)Ljava/lang/Object;"
				ft, _ := types.ParseMethodDescriptor(desc)
				call := &FunctionCallExpression{Object: receiver, ClassName: owner, FunctionName: method, Descriptor: desc, FuncType: ft.FunctionType(), Kind: InvokeInterface, OriginPC: 7, HasOriginPC: true, Arguments: []JavaValue{arg}}
				if text := call.String(ctx); !strings.HasPrefix(text, "receiver."+method+"(") {
					t.Fatalf("unproved receiver erasure: %s", text)
				}
			})
		}
	}
}
