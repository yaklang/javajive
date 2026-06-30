package types

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

func nameOf(t JavaType) string {
	if t == nil {
		return "<nil>"
	}
	return t.String(&class_context.ClassContext{})
}

// TestCommonSuperTypeLUB pins the two Phase 2 load-bearing LUB shapes: the Number family
// (EnumSchema Integer|Long|BigInteger) and the collection family (DaitchMokotoffSoundex
// ArrayList|List). It exercises commonSuperType / MergeTypes directly so a regression in the
// hierarchy table is caught without a full jar recompile.
func TestCommonSuperTypeLUB(t *testing.T) {
	cases := []struct {
		name string
		arms []string
		want string
	}{
		{"int_long", []string{"java.lang.Integer", "java.lang.Long"}, "Number"},
		{"int_long_bigint", []string{"java.lang.Integer", "java.lang.Long", "java.math.BigInteger"}, "Number"},
		{"arraylist_list", []string{"java.util.ArrayList", "java.util.List"}, "List"},
		{"arraylist_linkedlist", []string{"java.util.ArrayList", "java.util.LinkedList"}, "List"},
		{"hashmap_treemap", []string{"java.util.HashMap", "java.util.TreeMap"}, "AbstractMap"}, // nearest common superclass
		{"string_sb", []string{"java.lang.String", "java.lang.StringBuilder"}, "CharSequence"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			arms := make([]JavaType, 0, len(c.arms))
			for _, n := range c.arms {
				arms = append(arms, NewJavaClass(n))
			}
			got := nameOf(MergeTypes(arms...))
			if got != c.want {
				t.Errorf("MergeTypes(%v) = %s, want %s", c.arms, got, c.want)
			}
		})
	}
}

// TestCommonSuperTypeBailouts verifies the conservative guards: an unknown type or an Object-only LUB
// must fall back to the legacy first-arm behavior (return nil from commonSuperType), never widen.
func TestCommonSuperTypeBailouts(t *testing.T) {
	// Unknown app type -> bail (keep first arm).
	if got := commonSuperType([]JavaType{NewJavaClass("com.acme.Foo"), NewJavaClass("java.lang.Integer")}); got != nil {
		t.Errorf("unknown-type LUB should bail, got %s", nameOf(got))
	}
	// Number vs List share only Object -> bail (no widening to Object).
	if got := commonSuperType([]JavaType{NewJavaClass("java.lang.Integer"), NewJavaClass("java.util.List")}); got != nil {
		t.Errorf("Object-only LUB should bail, got %s", nameOf(got))
	}
}
