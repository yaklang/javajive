package values

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestInstantiatedArrayResultOverloadKeepsPermissionAtOriginalUse(t *testing.T) {
	for _, variant := range []string{"array", "interface", "rank two", "array type argument", "primitive array type argument", "named result", "method formal result", "wrong rank", "wrong first erasure", "primitive type argument", "free formal"} {
		t.Run(variant, func(t *testing.T) {
			rank := 1
			if variant == "rank two" {
				rank = 2
			}
			prefix := strings.Repeat("[", rank)
			result := prefix + "Ljava/lang/Object;"
			box := callbinding.Class{Name: "proof/ArrayBox", Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}, Signature: "<T:Ljava/lang/Object;>Ljava/lang/Object;", Methods: []callbinding.Method{{Name: "get", Desc: "()" + result, Signature: "()" + prefix + "TT;", Public: true}}}
			var argument types.JavaType = types.NewJavaClass("java.lang.String")
			rival := prefix + "Ljava/lang/String;"
			want := "Object" + strings.Repeat("[]", rank)
			kind := InvokeVirtual
			switch variant {
			case "interface":
				box.IsInterface, kind = true, InvokeInterface
			case "array type argument":
				argument = types.NewJavaArrayType(argument)
				rival = "[" + rival
			case "primitive array type argument":
				argument = types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))
				rival = "[Ljava/lang/Cloneable;"
			case "named result":
				box.Methods[0].Signature, want = "()[Lproof/T;", ""
			case "method formal result":
				box.Methods[0].Signature, want = "<U:Ljava/lang/Object;>()[TU;", ""
				box.Methods[0].Generic = true
			case "wrong rank":
				box.Methods[0].Signature, want = "()[[TT;", ""
			case "wrong first erasure":
				box.Signature, want = "<T:Ljava/lang/Number;>Ljava/lang/Object;", ""
			case "primitive type argument":
				argument, want = types.NewJavaPrimer(types.JavaInteger), ""
			case "free formal":
				box.Methods[0].Signature, want = "()[TU;", ""
			}
			consumer := callbinding.Class{Name: "proof/Use", Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}, Methods: []callbinding.Method{{Name: "choose", Desc: "(" + result + ")Ljava/lang/Object;", Static: true, Public: true}, {Name: "choose", Desc: "(" + rival + ")Ljava/lang/Object;", Static: true, Public: true}}}
			metadata := map[string]callbinding.Class{"proof/ArrayBox": box, "proof/Use": consumer, "java/lang/Object": {Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true}, "java/lang/String": {Name: "java/lang/String", Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}}, "java/lang/Cloneable": {Name: "java/lang/Cloneable", IsInterface: true, Public: true, MembersComplete: true, ParentsComplete: true}}
			ctx := &class_context.ClassContext{InvocationMetadata: func(name string) (callbinding.Class, bool) { v, known := metadata[name]; return v, known }}
			receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("proof.ArrayBox", []types.JavaType{argument}))
			parsed, err := types.ParseMethodDescriptor("()" + result)
			if err != nil {
				t.Fatal(err)
			}
			inner := &FunctionCallExpression{ClassName: "proof.ArrayBox", FunctionName: "get", Descriptor: "()" + result, Object: receiver, Kind: kind, HasOriginPC: true, OriginPC: 4, FuncType: parsed.FunctionType()}
			outer := &FunctionCallExpression{ClassName: "proof.Use", FunctionName: "choose", Descriptor: consumer.Methods[0].Desc, IsStatic: true, Kind: InvokeStatic, HasOriginPC: true, OriginPC: 9, Arguments: []JavaValue{inner}}
			if ErasedFixedInstanceResult(ctx, inner, result) {
				t.Fatal("array permission escaped to unrelated erased-result consumers")
			}
			before := inner.Descriptor
			if got := outer.instantiatedMethodResultOverloadCast(0, ctx); got != want {
				t.Fatalf("cast=%q want=%q", got, want)
			}
			if inner.Descriptor != before || bindingType(inner.FuncType.ReturnType) != result || inner.Object != receiver || outer.Arguments[0] != inner || inner.OriginPC != 4 {
				t.Fatal("array use proof changed physical producer/evaluation/origin")
			}
		})
	}
}
