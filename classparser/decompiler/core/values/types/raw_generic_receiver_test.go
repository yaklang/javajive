package types

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

func TestRawReceiverErasesItsGenericAncestor(t *testing.T) {
	signatures := map[string]string{
		"example/Parent":       "<X:Ljava/lang/Object;>Ljava/lang/Object;",
		"example/Child":        "<X:Ljava/lang/Object;>Lexample/Parent<TX;>;",
		"example/GenericFixed": "<X:Ljava/lang/Object;>Lexample/Parent<Ljava/lang/String;>;",
		"example/Fixed":        "Lexample/Parent<Ljava/lang/String;>;",
	}
	provider := func(name string) (string, map[string]string, bool) {
		sig, ok := signatures[name]
		methods := map[string]string{}
		if name == "example/Parent" {
			methods[class_context.MethodDescKey("accept", "(Ljava/lang/Object;)V")] = "(TX;)V"
		}
		return sig, methods, ok
	}
	// The caller intentionally has a same-spelled X. A foreign declaration's
	// unbound X must not become evidence of the receiver's actual argument.
	ctx := &class_context.ClassContext{ClassName: "example.Child", TypeParams: []string{"X"}}
	for _, tc := range []struct {
		name, receiver string
		args           []JavaType
		want           string
	}{
		{"raw direct", "example.Parent", nil, ""},
		{"raw child", "example.Child", nil, ""},
		{"raw fixed ancestor", "example.GenericFixed", nil, ""},
		{"missing argument", "example.Child", []JavaType{nil}, ""},
		{"parameterized child", "example.Child", []JavaType{NewJavaClass("java.lang.String")}, "String"},
		{"parameterized fixed ancestor", "example.GenericFixed", []JavaType{NewJavaClass("java.lang.Integer")}, "String"},
		{"non-generic child", "example.Fixed", nil, "String"},
		{"explicit caller variable", "example.Child", []JavaType{NewJavaClass("X")}, "X"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveInstantiatedParamType(ctx, provider, tc.receiver, tc.args, "accept", "(Ljava/lang/Object;)V", 1, 0)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("raw receiver invented formal %s", got.String(ctx))
				}
			} else if got == nil || got.String(ctx) != tc.want {
				t.Fatalf("resolved formal=%v want=%s", got, tc.want)
			}
		})
	}
}
