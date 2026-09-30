package types

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

func TestSelectedConsumerFormalIgnoresUnrelatedProducerCapture(t *testing.T) {
	desc := "(Ljava/lang/Object;)Ljava/lang/Object;"
	provider := func(name string) (string, map[string]string, bool) {
		switch name {
		case "example/Mapping":
			return "<A:Ljava/lang/Object;B:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("apply", desc): "(TA;)TB;"}, true
		case "example/Child":
			return "<X:Ljava/lang/Object;Y:Ljava/lang/Object;>Ljava/lang/Object;Lexample/Mapping<TY;TX;>;", nil, true
		case "example/Shadow":
			return "<A:Ljava/lang/Object;B:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("apply", desc): "<A:Ljava/lang/Object;>(TA;)TB;"}, true
		}
		return "", nil, false
	}
	ctx := &class_context.ClassContext{ClassTypeParams: []string{"T", "U"}, TypeParams: []string{"T", "U"}}
	lower := &JavaWildcardType{Variant: "super", Bound: NewJavaClass("U")}
	upper := &JavaWildcardType{Variant: "extends", Bound: NewJavaClass("T")}
	for _, tc := range []struct {
		name, owner string
		args        []JavaType
		want        string
	}{
		{"unrelated upper capture", "example.Mapping", []JavaType{lower, upper}, "U"},
		{"unrelated unbounded capture", "example.Mapping", []JavaType{lower, &JavaWildcardType{}}, "U"},
		{"ordinary consumer", "example.Mapping", []JavaType{NewJavaClass("U"), upper}, "U"},
		{"concrete consumer", "example.Mapping", []JavaType{NewJavaClass("java.lang.String"), upper}, "String"},
		{"inherited swapped arguments", "example.Child", []JavaType{upper, lower}, "U"},
		{"selected upper capture", "example.Mapping", []JavaType{upper, lower}, ""},
		{"selected unbounded capture", "example.Mapping", []JavaType{&JavaWildcardType{}, lower}, ""},
		{"foreign lower bound", "example.Mapping", []JavaType{&JavaWildcardType{Variant: "super", Bound: NewJavaClass("Foreign")}, upper}, ""},
		{"method variable shadow", "example.Shadow", []JavaType{lower, upper}, ""},
		{"raw receiver", "example.Mapping", nil, ""},
		{"missing signature", "example.Missing", []JavaType{lower, upper}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveInstantiatedParamType(ctx, provider, tc.owner, tc.args, "apply", desc, 1, 0)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("invented capture formal %s", got.String(ctx))
				}
			} else if got == nil || got.String(ctx) != tc.want {
				t.Fatalf("resolved=%v want=%s", got, tc.want)
			}
		})
	}
	t.Run("disabled lower bound recovery", func(t *testing.T) {
		disabled := *ctx
		disabled.Env = func(key string) string {
			if key == "JDEC_GENERIC_SUPERWILDCARD_OFF" {
				return "1"
			}
			return ""
		}
		if got := ResolveInstantiatedParamType(&disabled, provider, "example.Mapping", []JavaType{lower, upper}, "apply", desc, 1, 0); got != nil {
			t.Fatal("lower bound recovery ignored the policy switch")
		}
	})
}
