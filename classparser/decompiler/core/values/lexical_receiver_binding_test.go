package values

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
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

func TestLexicalThisReceiverRequiresExactOriginalScope(t *testing.T) {
	for _, kind := range []string{"original", "no path", "unknown path", "static cut", "wrong path", "missing outer", "missing declaration", "wrong original bounds", "wrong receiver", "method shadow", "inner shadow", "cyclic scope", "excess scope", "unnamed source", "budget", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			outerSig := "<K:Ljava/lang/Object;>Ljava/lang/Object;"
			outer := &class_context.ClassContext{ClassName: "scope.Owner", ClassSig: outerSig}
			ctx := &class_context.ClassContext{ClassName: "scope.Owner$Gate", ClassSig: "Ljava/lang/Object;", LexicalClassName: "Gate", TypeParams: []string{"K"}, SourceLexicalParent: outer}
			path := []string{"scope/Owner", "scope/Owner$Gate"}
			known := true
			ctx.SiblingLexicalTypeOwners = func(string) ([]string, bool) { return path, known }
			ctx.SiblingClassSig = func(n string) (string, map[string]string, bool) {
				if n == "scope/Owner" {
					return outerSig, nil, kind != "missing declaration"
				}
				return ctx.ClassSig, nil, n == "scope/Owner$Gate"
			}
			ref := NewJavaRef(nil, nil, types.NewJavaClass(ctx.ClassName))
			ref.IsThis = true
			var receiver JavaValue = ref
			switch kind {
			case "no path":
				ctx.SiblingLexicalTypeOwners = nil
			case "unknown path":
				known = false
			case "static cut":
				path = path[1:]
			case "wrong path":
				path[0] = "scope/Other"
			case "missing outer":
				ctx.SourceLexicalParent = nil
			case "missing declaration":
			case "wrong original bounds":
				outer.ClassSig = "<K:Ljava/lang/Number;>Ljava/lang/Object;"
			case "wrong receiver":
				other := NewJavaRef(nil, nil, types.NewJavaClass("scope.Other"))
				other.IsThis = true
				receiver = other
			case "method shadow":
				ctx.CurrentMethodSig = "<K:Ljava/lang/Number;>()V"
			case "inner shadow":
				ctx.ClassSig = "<K:Ljava/lang/Number;>Ljava/lang/Object;"
			case "cyclic scope":
				outer.SourceLexicalParent = ctx
			case "excess scope":
				for i := 0; i < 65; i++ {
					outer.SourceLexicalParent = &class_context.ClassContext{SourceLexicalParent: outer.SourceLexicalParent}
				}
			case "unnamed source":
				ctx.LexicalClassName = ""
			case "budget":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "cancelled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			}
			got := (&FunctionCallExpression{Object: receiver}).lexicalParameterizedReceiver(ctx)
			if kind == "original" {
				if got == nil || len(got.OwnerSegments) != 2 || len(got.OwnerSegments[0].TypeArgs) != 1 || got.OwnerSegments[0].TypeArgs[0].String(ctx) != "K" {
					t.Fatalf("binding=%v", got)
				}
			} else if got != nil {
				t.Fatalf("unproved lexical self=%v", got)
			}
		})
	}
}
