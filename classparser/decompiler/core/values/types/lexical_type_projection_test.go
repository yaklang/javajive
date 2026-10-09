package types

import (
	"reflect"
	"testing"
)

func TestLexicalTypeProjectionPreservesDeclarationIdentity(t *testing.T) {
	outer := LexicalTypeScope{Signature: "<T:Ljava/lang/Number;U:TT;>Ljava/lang/Object;"}
	rows := []struct {
		name             string
		scopes           []LexicalTypeScope
		requested, names []string
		clauses, erased  map[string]string
		valid            bool
	}{
		{"bounded method shadows class", []LexicalTypeScope{outer, {Signature: "<T::Ljava/lang/CharSequence;>(TT;)TT;", Method: true, Descriptor: "(Ljava/lang/CharSequence;)Ljava/lang/CharSequence;"}}, []string{"T"}, []string{"T"}, map[string]string{"T": "CharSequence"}, map[string]string{"T": "Ljava/lang/CharSequence;"}, true},
		{"Object method also shadows class", []LexicalTypeScope{outer, {Signature: "<T:Ljava/lang/Object;>()V", Method: true, Descriptor: "()V"}}, []string{"T"}, []string{"T"}, map[string]string{}, map[string]string{"T": "Ljava/lang/Object;"}, true},
		{"dependent first bound closes declarations", []LexicalTypeScope{{Signature: "<T:TU;U::Ljava/lang/CharSequence;>()V", Method: true, Descriptor: "()V"}}, []string{"T"}, []string{"T", "U"}, map[string]string{"T": "U", "U": "CharSequence"}, map[string]string{"T": "Ljava/lang/CharSequence;", "U": "Ljava/lang/CharSequence;"}, true},
		{"intersection and recursive argument", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;:Ljava/lang/Comparable<TT;>;:Ljava/io/Serializable;>()V", Method: true, Descriptor: "()V"}}, []string{"T"}, []string{"T"}, map[string]string{"T": "Number & Comparable<T> & Serializable"}, map[string]string{"T": "Ljava/lang/Number;"}, true},
		{"class after method retains captured binding", []LexicalTypeScope{{Signature: "<T::Ljava/lang/CharSequence;>()V", Method: true, Descriptor: "()V"}, {Signature: "Ljava/lang/Object;Ljava/util/function/Supplier<TT;>;"}}, []string{"T"}, []string{"T"}, map[string]string{"T": "CharSequence"}, map[string]string{"T": "Ljava/lang/CharSequence;"}, true},
		{"method after method shadows captured binding", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>()V", Method: true, Descriptor: "()V"}, {Signature: "Ljava/lang/Object;"}, {Signature: "<T::Ljava/lang/CharSequence;>()V", Method: true, Descriptor: "()V"}}, []string{"T"}, []string{"T"}, map[string]string{"T": "CharSequence"}, map[string]string{"T": "Ljava/lang/CharSequence;"}, true},
		{"hidden outer bound cannot borrow method name", []LexicalTypeScope{outer, {Signature: "<T::Ljava/lang/CharSequence;>()V", Method: true, Descriptor: "()V"}}, []string{"U"}, nil, nil, nil, false},
		{"physical method descriptor mismatch", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>(TT;)V", Method: true, Descriptor: "(Ljava/lang/Object;)V"}}, []string{"T"}, nil, nil, nil, false},
		{"missing requested declaration", []LexicalTypeScope{outer}, []string{"X"}, nil, nil, nil, false},
		{"class cannot fix free method bound", []LexicalTypeScope{{Signature: "<T:TU;>()V", Method: true, Descriptor: "()V"}, {Signature: "<U:Ljava/lang/Number;>Ljava/lang/Object;"}}, []string{"T"}, nil, nil, nil, false},
		{"duplicate formal", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;T:Ljava/lang/Object;>()V", Method: true}}, []string{"T"}, nil, nil, nil, false},
		{"malformed trailing grammar", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>()Vx", Method: true}}, []string{"T"}, nil, nil, nil, false},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			names, clauses, erased, ok := ProjectLexicalTypeParameters(r.scopes, r.requested, nil)
			if ok != r.valid || !reflect.DeepEqual(names, r.names) || !reflect.DeepEqual(clauses, r.clauses) || !reflect.DeepEqual(erased, r.erased) {
				t.Fatalf("names=%v bounds=%v erasures=%v valid=%v", names, clauses, erased, ok)
			}
		})
	}
}

func TestLexicalTypeErasureDoesNotRebindHiddenOuterBound(t *testing.T) {
	scopes := []LexicalTypeScope{
		{Signature: "<T:Ljava/lang/Number;U:TT;>Ljava/lang/Object;"},
		{Signature: "<T::Ljava/lang/CharSequence;>()V", Method: true, Descriptor: "()V"},
	}
	got, ok := LexicalTypeParameterErasures(scopes, []string{"U", "T"})
	if !ok || !reflect.DeepEqual(got, map[string]string{"U": "Ljava/lang/Number;", "T": "Ljava/lang/CharSequence;"}) {
		t.Fatal(got, ok)
	}
	if _, ok := LexicalTypeParameterErasures(scopes, []string{"Missing"}); ok {
		t.Fatal("invented binding erasure")
	}
	if _, _, _, ok := ProjectLexicalTypeParameters(make([]LexicalTypeScope, 130), nil, nil); ok {
		t.Fatal("scope cap ignored")
	}
	if _, _, _, ok := ProjectLexicalTypeParameters(scopes, make([]string, 513), nil); ok {
		t.Fatal("projection request cap ignored")
	}
	if _, ok := LexicalTypeParameterErasures(scopes, make([]string, 513)); ok {
		t.Fatal("erasure request cap ignored")
	}
}
