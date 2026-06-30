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
