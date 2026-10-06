package types

import (
	"strings"
	"testing"
)

func TestLexicalOwnerMethodErasureRetainsDeclarationBoundScope(t *testing.T) {
	rows := []struct {
		name         string
		classes      []string
		method, want string
	}{
		{"distinct inherited", []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;", "<U::Ljava/lang/CharSequence;>Ljava/lang/Object;", ""}, "(TT;TU;)Ljava/lang/Object;", "(Ljava/lang/Number;Ljava/lang/CharSequence;)Ljava/lang/Object;"},
		{"outer bound not rebound by inner shadow", []string{"<T:Ljava/lang/Number;U:TT;>Ljava/lang/Object;", "<T::Ljava/lang/CharSequence;>Ljava/lang/Object;"}, "(TU;TT;)TU;", "(Ljava/lang/Number;Ljava/lang/CharSequence;)Ljava/lang/Number;"},
		{"method shadows last class", []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}, "<T::Ljava/lang/CharSequence;>(TT;)TT;", "(Ljava/lang/CharSequence;)Ljava/lang/CharSequence;"},
		{"method bound resolves its own shadow", []string{"<T:Ljava/lang/Number;U:TT;>Ljava/lang/Object;"}, "<T::Ljava/lang/CharSequence;V:TT;>(TU;TV;)TV;", "(Ljava/lang/Number;Ljava/lang/CharSequence;)Ljava/lang/CharSequence;"},
		{"forward first-bound dependency", []string{"<U:TT;T:Ljava/lang/Number;>Ljava/lang/Object;"}, "(TU;)TT;", "(Ljava/lang/Number;)Ljava/lang/Number;"},
		{"inherited first-bound dependency", []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;", "<U:TT;>Ljava/lang/Object;", "<T::Ljava/lang/CharSequence;>Ljava/lang/Object;"}, "(TU;TT;)TU;", "(Ljava/lang/Number;Ljava/lang/CharSequence;)Ljava/lang/Number;"},
		{"parameterized recursive bound is not erasure cycle", []string{"<T:Ljava/lang/Comparable<TT;>;>Ljava/lang/Object;"}, "(TT;)TT;", "(Ljava/lang/Comparable;)Ljava/lang/Comparable;"},
		{"recursive method bound", nil, "<T:Ljava/lang/Comparable<TT;>;>(TT;)TT;", "(Ljava/lang/Comparable;)Ljava/lang/Comparable;"},
		{"literal class named formal", []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}, "(LT;TT;)[TT;", "(LT;Ljava/lang/Number;)[Ljava/lang/Number;"},
		{"owner arguments and arrays", []string{"<T:Ljava/lang/Number;>Ljava/lang/Object;"}, "([TT;Lpkg/Outer<TT;>.Inner<Ljava/lang/String;>;)Ljava/util/List<TT;>;", "([Ljava/lang/Number;Lpkg/Outer$Inner;)Ljava/util/List;"},
		{"free method variable", nil, "(TX;)V", ""},
		{"free class bound cannot be fixed by inner", []string{"<U:TT;>Ljava/lang/Object;", "<T:Ljava/lang/Number;>Ljava/lang/Object;"}, "(TU;)V", ""},
		{"free outer super arguments cannot borrow inner", []string{"Ljava/util/List<TT;>;", "<T:Ljava/lang/Number;>Ljava/lang/Object;"}, "()V", ""},
		{"dependent cycle", []string{"<T:TU;U:TT;>Ljava/lang/Object;"}, "(TT;)V", ""},
		{"self first-bound cycle", []string{"<T:TT;>Ljava/lang/Object;"}, "(TT;)V", ""},
		{"interface must not hide cyclic first bound", []string{"<T:TT;:Ljava/io/Serializable;>Ljava/lang/Object;"}, "(TT;)V", ""},
		{"duplicate formal", []string{"<T:Ljava/lang/Number;T:Ljava/lang/String;>Ljava/lang/Object;"}, "(TT;)V", ""},
		{"primitive argument", nil, "(Ljava/util/List<I>;)V", ""},
		{"void array argument", nil, "(Ljava/util/List<[V>;)V", ""},
		{"class grammar not field", []string{"TT;"}, "()V", ""},
		{"class grammar not method", []string{"()V"}, "()V", ""},
		{"method grammar not field", nil, "Ljava/lang/Object;", ""},
		{"empty first bound", []string{"<T:>Ljava/lang/Object;"}, "(TT;)V", ""},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			got, _, ok := EraseLexicalOwnerMethodSignatureWithThrows(r.classes, r.method)
			if ok != (r.want != "") || got != r.want {
				t.Fatalf("got %q valid=%v want=%q", got, ok, r.want)
			}
		})
	}
	t.Run("scope depth cap", func(t *testing.T) {
		if _, _, ok := EraseLexicalOwnerMethodSignatureWithThrows(make([]string, 65), "()V"); ok {
			t.Fatal("depth cap ignored")
		}
	})
	t.Run("signature length cap", func(t *testing.T) {
		if _, _, ok := EraseLexicalOwnerMethodSignatureWithThrows(nil, strings.Repeat("X", 65536)); ok {
			t.Fatal("signature cap ignored")
		}
	})
	t.Run("bound throws identity", func(t *testing.T) {
		got, throws, ok := EraseLexicalOwnerMethodSignatureWithThrows([]string{"<T:Ljava/lang/Exception;>Ljava/lang/Object;"}, "<T:Ljava/lang/RuntimeException;>()V^TT;")
		if !ok || got != "()V" || len(throws) != 1 || throws[0] != "Ljava/lang/RuntimeException;" {
			t.Fatal(got, throws, ok)
		}
	})
}
