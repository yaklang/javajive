package types

import "testing"

func TestClassSignatureInnerSuffixKeepsBinaryIdentity(t *testing.T) {
	for _, tc := range []struct{ signature, binary string }{
		{"Lp/Owner.Inner<TT;>;", "p.Owner$Inner"},
		{"Lp/Owner<TT;>.Inner<TU;>;", "p.Owner$Inner"},
		{"Lp/Owner.Middle.Inner<TT;>;", "p.Owner$Middle$Inner"},
		{"Lp/Owner$Literal.Inner<TT;>;", "p.Owner$Literal$Inner"},
		{"Lp/Owner$Literal<TT;>;", "p.Owner$Literal"},
	} {
		t.Run(tc.signature, func(t *testing.T) {
			super, interfaces := ParseClassSignatureSupers(tc.signature)
			p, ok := AsParameterizedType(super)
			if !ok || p.RawClassName != tc.binary || len(p.TypeArgs) != 1 || len(interfaces) != 0 {
				t.Fatalf("original signature must identify %q with one formal: %+v", tc.binary, super)
			}
		})
	}
}
