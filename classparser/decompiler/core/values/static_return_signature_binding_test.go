package values

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestStaticReturnSignatureDoesNotInventNullCheckcast(t *testing.T) {
	for _, signature := range []string{"(Lproof/Token;)Ljava/util/List<*>;", "(Lproof/Token;)Ljava/util/List<Ljava/lang/String;>;", "(Lproof/Token;)Ljava/util/List<+Ljava/lang/Number;>;"} {
		for _, variant := range []string{"unique", "competing", "missing signature", "generic argument", "method variable", "wrong erasure", "trailing garbage", "incomplete", "original cast"} {
			t.Run(signature+"/"+variant, func(t *testing.T) {
				desc := "(Lproof/Token;)Ljava/util/List;"
				m := callbinding.Method{Name: "produce", Desc: desc, Signature: signature, Public: true, Static: true, Generic: true}
				switch variant {
				case "missing signature":
					m.Signature = ""
				case "generic argument":
					m.Signature = "(Lproof/Token<*>;)Ljava/util/List<*>;"
				case "method variable":
					m.Signature = "<T:Ljava/lang/Object;>(Lproof/Token;)Ljava/util/List<TT;>;"
				case "wrong erasure":
					m.Signature = "(Lproof/Other;)Ljava/util/List<*>;"
				case "trailing garbage":
					m.Signature += "!"
				}
				meta := callbinding.Class{Name: "proof/Factory", Public: true, MembersComplete: variant != "incomplete", ParentsComplete: true, Methods: []callbinding.Method{m}}
				if variant == "competing" {
					meta.Methods = append(meta.Methods, callbinding.Method{Name: m.Name, Desc: "(Ljava/lang/String;)Ljava/util/List;", Public: true, Static: true})
				}
				ctx := &class_context.ClassContext{InvocationMetadata: func(n string) (callbinding.Class, bool) { return meta, n == meta.Name }}
				mt, _ := types.ParseMethodDescriptor(desc)
				var arg JavaValue = NewOriginalNullLiteral(4)
				if variant == "original cast" {
					arg = &CastExpression{Value: arg, TargetType: types.NewJavaClass("proof.Token"), OriginPC: 5}
				}
				call := &FunctionCallExpression{ClassName: "proof.Factory", Object: NewJavaClassValue(types.NewJavaClass("proof.Factory")), FunctionName: m.Name, Descriptor: desc, IsStatic: true, Kind: InvokeStatic, FuncType: mt.FunctionType(), Arguments: []JavaValue{arg}}
				wantProof := variant == "unique" || variant == "original cast"
				if got := call.staticCallHasUniqueErasedBinding(ctx); got != wantProof {
					t.Fatalf("selection proof %v want %v", got, wantProof)
				}
				if variant == "unique" {
					if got := call.String(ctx); got != "Factory.produce(null)" {
						t.Fatalf("unneeded resolution-bearing cast: %s", got)
					}
				}
				if variant == "original cast" {
					if got := call.String(ctx); !strings.Contains(got, "Token") {
						t.Fatalf("original cast lost: %s", got)
					}
				}
			})
		}
	}
}
