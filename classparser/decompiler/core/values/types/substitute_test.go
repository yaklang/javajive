package types

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// TestSubstituteTypeVars pins the generic-substitution primitive that instantiates a callee signature
// (written in the declaring class's type variables) at a call site with the receiver's actual type
// arguments. It is the root-cause core shared by the unified cross-class resolver, so a regression here
// would silently break every recovered argument cast.
func TestSubstituteTypeVars(t *testing.T) {
	ctx := &class_context.ClassContext{}
	str := NewJavaClass("java.lang.String")
	integer := NewJavaClass("java.lang.Integer")

	// identity: {K:String}, K -> String; an unmapped var V stays V; a concrete class stays.
	sigma := map[string]JavaType{"K": str, "V": integer}

	cases := []struct {
		name  string
		in    JavaType
		sigma map[string]JavaType
		want  string
	}{
		{"typevar_hit", NewJavaClass("K"), sigma, "String"},
		{"typevar_other_hit", NewJavaClass("V"), sigma, "Integer"},
		{"typevar_miss_unchanged", NewJavaClass("T"), sigma, "T"},
		{"concrete_unchanged", NewJavaClass("java.lang.String"), sigma, "String"},
		{"empty_sigma_unchanged", NewJavaClass("K"), map[string]JavaType{}, "K"},
		{
			"parameterized_args_substituted",
			NewParameterizedType("java.util.Map", []JavaType{NewJavaClass("K"), NewJavaClass("V")}),
			sigma,
			"Map<String, Integer>",
		},
		{
			"nested_parameterized",
			NewParameterizedType("java.util.List", []JavaType{
				NewParameterizedType("java.util.Map", []JavaType{NewJavaClass("K"), NewJavaClass("V")}),
			}),
			sigma,
			"List<Map<String, Integer>>",
		},
		{
			"array_of_typevar",
			NewJavaArrayType(NewJavaClass("K")),
			sigma,
			"String[]",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := SubstituteTypeVars(c.in, c.sigma)
			if got == nil {
				t.Fatalf("%s: got nil", c.name)
			}
			if s := got.String(ctx); s != c.want {
				t.Errorf("%s: got %q want %q", c.name, s, c.want)
			}
		})
	}

	// nil-safe.
	if SubstituteTypeVars(nil, sigma) != nil {
		t.Errorf("nil input should return nil")
	}
}

// A jar-internal generic map implementation can inherit a remapping method from
// ConcurrentMap.  The sibling provider deliberately stops at the JDK boundary,
// but the receiver arguments composed along the jar hierarchy are still enough
// to instantiate the stable JDK functional formal.  The return remains unknown:
// callers that only need a parameter must not invent return metadata.
func TestResolveInstantiatedSignatureRecoversJDKLeafFormal(t *testing.T) {
	ctx := &class_context.ClassContext{}
	provider := func(internal string) (string, map[string]string, bool) {
		if internal != "example/LocalMap" {
			return "", nil, false
		}
		return "<K:Ljava/lang/Object;V:Ljava/lang/Object;>Ljava/lang/Object;Ljava/util/concurrent/ConcurrentMap<TK;TV;>;", nil, true
	}
	recvArgs := []JavaType{
		NewJavaClass("K"),
		NewParameterizedType("java.util.concurrent.CompletableFuture", []JavaType{NewJavaClass("V")}),
	}
	params, ret := ResolveInstantiatedSignature(ctx, provider, "example.LocalMap", recvArgs, "computeIfPresent", 2)
	if ret != nil {
		t.Fatalf("JDK leaf parameter recovery must not invent a return type: %s", nameOf(ret))
	}
	if len(params) != 2 || params[1] == nil {
		t.Fatalf("missing JDK leaf functional formal: %#v", params)
	}
	want := "BiFunction<? super K, ? super CompletableFuture<V>, ? extends CompletableFuture<V>>"
	if got := params[1].String(ctx); got != want {
		t.Fatalf("JDK leaf formal = %q, want %q", got, want)
	}

	t.Setenv("JDEC_GENERIC_PARAM_INFER_OFF", "1")
	params, _ = ResolveInstantiatedSignature(ctx, provider, "example.LocalMap", recvArgs, "computeIfPresent", 2)
	if len(params) != 0 {
		t.Fatalf("generic-inference kill switch retained JDK leaf formals: %#v", params)
	}
}
