package types

import "testing"

func TestLexicalFormalSignaturePreservesBoundsAndBinderIdentity(t *testing.T) {
	for _, row := range []struct {
		name      string
		scopes    []LexicalTypeScope
		requested []string
		want      string
	}{
		{"class bound", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>Ljava/lang/Object;"}}, []string{"T"}, "<T:Ljava/lang/Number;>"},
		{"interface bound", []LexicalTypeScope{{Signature: "<T::Ljava/lang/CharSequence;>Ljava/lang/Object;"}}, []string{"T"}, "<T::Ljava/lang/CharSequence;>"},
		{"recursive intersection", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;:Ljava/lang/Comparable<TT;>;>Ljava/lang/Object;"}}, []string{"T"}, "<T:Ljava/lang/Number;:Ljava/lang/Comparable<TT;>;>"},
		{"dependencies", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;U:TT;>Ljava/lang/Object;"}}, []string{"U", "T"}, "<U:TT;T:Ljava/lang/Number;>"},
		{"shadow", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>Ljava/lang/Object;"}, {Signature: "<T:Ljava/lang/Object;>Ljava/lang/Object;"}}, []string{"T"}, "<T:Ljava/lang/Object;>"},
		{"hidden dependency", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;U:TT;>Ljava/lang/Object;"}, {Signature: "<T:Ljava/lang/CharSequence;>Ljava/lang/Object;"}}, []string{"T", "U"}, ""},
		{"missing dependency", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;U:TT;>Ljava/lang/Object;"}}, []string{"U"}, ""},
		{"unknown", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>Ljava/lang/Object;"}}, []string{"X"}, ""},
		{"duplicate", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>Ljava/lang/Object;"}}, []string{"T", "T"}, ""},
		{"malformed", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>Ljava/lang/Object;trailing"}}, []string{"T"}, ""},
		{"empty", nil, nil, ""},
		{"method bound", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>(TT;)TT;", Method: true, Descriptor: "(Ljava/lang/Number;)Ljava/lang/Number;"}}, []string{"T"}, "<T:Ljava/lang/Number;>"},
		{"method descriptor mismatch", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>(TT;)TT;", Method: true, Descriptor: "(Ljava/lang/Object;)Ljava/lang/Object;"}}, []string{"T"}, ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			got, ok := ProjectLexicalFormalSignature(row.scopes, row.requested)
			if ok != (row.want != "") || ok && got != row.want {
				t.Fatalf("got %q/%v, want %q", got, ok, row.want)
			}
		})
	}
}
