package statements

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestZeroInputFactoryReturnRequiresWideningAndDeclaredErasure(t *testing.T) {
	for _, scenario := range []string{"proved", "same erasure", "unknown hierarchy", "foreign result", "narrowing", "mismatched consumer", "raw consumer", "array consumer"} {
		t.Run(scenario, func(t *testing.T) {
			desc := "()Lproof/Order;"
			meta := map[string]callbinding.Class{
				"proof/Order":      {Name: "proof/Order", Public: true, Parents: []string{"proof/Comparator"}, ParentsComplete: true, MembersComplete: true, Methods: []callbinding.Method{{Name: "choose", Desc: desc, Static: true, Generic: true}}},
				"proof/Comparator": {Name: "proof/Comparator", IsInterface: true, Public: true, ParentsComplete: true, MembersComplete: true},
				"proof/Foreign":    {Name: "proof/Foreign", Public: true, ParentsComplete: true, MembersComplete: true},
			}
			var target types.JavaType = types.NewParameterizedType("proof.Comparator", []types.JavaType{types.NewJavaClass("T")})
			ctx := &class_context.ClassContext{FunctionType: &types.JavaFuncType{ReturnType: target}, CurrentMethodDesc: "()Lproof/Comparator;", InvocationMetadata: func(n string) (callbinding.Class, bool) { c, ok := meta[n]; return c, ok }, SiblingClassSig: func(n string) (string, map[string]string, bool) {
				return "<X:Ljava/lang/Object;>Lproof/Comparator<TX;>;", map[string]string{class_context.MethodDescKey("choose", desc): "<C:Ljava/lang/Object;>()Lproof/Order<TC;>;"}, n == "proof/Order"
			}}
			call := &values.FunctionCallExpression{ClassName: "proof.Order", FunctionName: "choose", Descriptor: desc, IsStatic: true, Kind: values.InvokeStatic, OriginPC: 5, HasOriginPC: true}
			switch scenario {
			case "same erasure":
				target = types.NewParameterizedType("proof.Order", []types.JavaType{types.NewJavaClass("T")})
				ctx.CurrentMethodDesc = desc
			case "unknown hierarchy":
				delete(meta, "proof/Comparator")
			case "foreign result":
				m := meta["proof/Order"]
				m.Parents = []string{"proof/Foreign"}
				meta["proof/Order"] = m
			case "narrowing":
				m := meta["proof/Comparator"]
				m.Parents = []string{"proof/Order"}
				meta["proof/Comparator"] = m
				m = meta["proof/Order"]
				m.Parents = nil
				meta["proof/Order"] = m
			case "mismatched consumer":
				ctx.CurrentMethodDesc = desc
			case "raw consumer":
				target = types.NewJavaClass("proof.Comparator")
			case "array consumer":
				target = types.NewJavaArrayType(target)
			}
			ctx.FunctionType = &types.JavaFuncType{ReturnType: target}
			before := call.Witness()
			cast, raw := erasedFactoryReturnCast(ctx, call)
			want := scenario == "proved" || scenario == "same erasure"
			if (cast != "" && raw != "") != want {
				t.Fatalf("target=%q raw=%q", cast, raw)
			}
			if call.Witness() != before || call.Arguments != nil {
				t.Fatal("return proof changed invoke/evaluation")
			}
		})
	}
}
