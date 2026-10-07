package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestLexicalReceiverFieldDoesNotBorrowShadowingMethodBinder(t *testing.T) {
	for _, variant := range []string{"field", "typed field", "method shadow", "typed field method shadow", "parameter", "method parameter", "raw field", "field disabled", "foreign field", "nil receiver"} {
		t.Run(variant, func(t *testing.T) {
			ctx := &class_context.ClassContext{ClassName: "scope.Reader", ClassSig: "<K:Ljava/lang/Object;>Ljava/lang/Object;", TypeParams: []string{"K"}, FieldSignatures: map[string]string{"gate": "Lscope/Owner<TK;>.Gate;"}}
			raw := types.NewJavaClass("scope.Owner$Gate")
			parameterized := types.ParseSignature("Lscope/Owner<TK;>.Gate;")
			receiver := JavaValue(NewRefMember(&JavaRef{IsThis: true}, "gate", raw))
			switch variant {
			case "typed field":
				receiver = NewRefMember(&JavaRef{IsThis: true}, "gate", parameterized)
			case "method shadow":
				ctx.CurrentMethodSig = "<K:Ljava/lang/Number;>(TK;)V"
			case "typed field method shadow":
				receiver = NewRefMember(&JavaRef{IsThis: true}, "gate", parameterized)
				ctx.CurrentMethodSig = "<K:Ljava/lang/Number;>(TK;)V"
			case "parameter":
				receiver = NewJavaRef(nil, nil, parameterized)
			case "method parameter":
				receiver = NewJavaRef(nil, nil, parameterized)
				ctx.CurrentMethodSig = "<K:Ljava/lang/Number;>(TK;)V"
			case "raw field":
				ctx.FieldSignatures = nil
			case "field disabled":
				ctx.Env = func(k string) string {
					if k == "JDEC_GENERIC_PARAM_FIELD_OFF" {
						return "1"
					}
					return ""
				}
			case "foreign field":
				receiver = NewRefMember(NewJavaRef(nil, nil, types.NewJavaClass("scope.Other")), "gate", raw)
			case "nil receiver":
				receiver = nil
			}
			call := &FunctionCallExpression{Object: receiver}
			got := call.lexicalParameterizedReceiver(ctx)
			want := variant == "field" || variant == "typed field" || variant == "parameter" || variant == "method parameter"
			if (got != nil) != want {
				t.Fatalf("receiver binding known=%v", got != nil)
			}
		})
	}
}
