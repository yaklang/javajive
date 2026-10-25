package types

import (
	"reflect"
	"strings"
	"testing"
)

func TestHiddenLexicalSignatureProjectionPreservesVisibleBinders(t *testing.T) {
	scopes := []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>Ljava/lang/Object;"}, {Signature: "<U:Ljava/lang/CharSequence;>Ljava/lang/Object;"}}
	for _, test := range []struct{ name, signature, want string }{
		{"hidden standalone", "(TT;)TT;", "(Ljava/lang/Number;)Ljava/lang/Number;"},
		{"own binder", "(TU;)TU;", "(TU;)TU;"},
		{"method shadow", "<T:Ljava/lang/CharSequence;>(TT;)TT;", "<T:Ljava/lang/CharSequence;>(TT;)TT;"},
		{"dependent method bound", "<V:TT;>(TV;)TV;", "<V:Ljava/lang/Number;>(TV;)TV;"},
		{"interface bound", "<V::Ljava/lang/Comparable<TV;>;>(TT;)TV;", "<V::Ljava/lang/Comparable<TV;>;>(Ljava/lang/Number;)TV;"},
		{"hidden arrays", "([TT;)[[TT;", "([Ljava/lang/Number;)[[Ljava/lang/Number;"},
		{"raw direct argument", "()Ljava/util/Map<TT;TU;>;", "()Ljava/util/Map;"},
		{"nested argument", "()Ljava/util/List<Ljava/util/Map<TT;TU;>;>;", "()Ljava/util/List<Ljava/util/Map;>;"},
		{"array argument", "()Ljava/util/List<[TT;>;", "()Ljava/util/List<[Ljava/lang/Number;>;"},
		{"wildcard bound", "()Ljava/util/List<+TT;>;", "()Ljava/util/List;"},
		{"preserve visible variance", "()Ljava/util/Map<-TU;*>;", "()Ljava/util/Map<-TU;*>;"},
		{"literal class role", "()Ljava/util/List<LT;>;", "()Ljava/util/List<LT;>;"},
		{"raw owner member", "()Lproof/Owner<TT;>.Leaf;", "()Lproof/Owner$Leaf;"},
		{"raw owner chain", "()Lproof/Owner<TT;>.Branch.Leaf;", "()Lproof/Owner$Branch$Leaf;"},
		{"raw owner parameterized member", "()Lproof/Owner<TT;>.Leaf<TU;>;", "()Lproof/Owner$Leaf;"},
		{"retained owner member", "()Lproof/Owner<TU;>.Leaf;", "()Lproof/Owner<TU;>.Leaf;"},
		{"raw ancestor makes member raw", "()Lproof/Owner<TT;>.Branch<TU;>.Leaf;", "()Lproof/Owner$Branch$Leaf;"},
		{"raw child cannot parameterize owner", "()Lproof/Owner<TU;>.Leaf<TT;>;", "()Lproof/Owner$Leaf;"},
		{"raw member nested argument", "()Ljava/util/List<Lproof/Owner<TT;>.Leaf;>;", "()Ljava/util/List<Lproof/Owner$Leaf;>;"},
		{"raw member array", "()[Lproof/Owner<TT;>.Leaf;", "()[Lproof/Owner$Leaf;"},
		{"raw member method shadow", "<T:Ljava/lang/Object;>()Lproof/Owner<TT;>.Leaf;", "<T:Ljava/lang/Object;>()Lproof/Owner<TT;>.Leaf;"},
		{"unknown binder", "()TMissing;", ""},
		{"trailing junk", "()TT;garbage", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, valid := ProjectHiddenLexicalMethodSignature(scopes, test.signature)
			if valid != (test.want != "") || valid && got != test.want {
				t.Fatalf("projection %q valid=%v want %q", got, valid, test.want)
			}
		})
	}
	copy := append([]LexicalTypeScope(nil), scopes...)
	if _, ok := ProjectHiddenLexicalMethodSignature(scopes, "()TT;"); !ok || !reflect.DeepEqual(copy, scopes) {
		t.Fatal("caller scopes changed")
	}
	if _, ok := ProjectHiddenLexicalMethodSignature(scopes, "()"+strings.Repeat("[", 65)+"TT;"); ok {
		t.Fatal("unbounded projection")
	}
	if _, ok := ProjectHiddenLexicalMethodSignature(make([]LexicalTypeScope, 129), "()V"); ok {
		t.Fatal("unbounded scope stack")
	}
	large := []LexicalTypeScope{{Signature: "<T:L" + strings.Repeat("LongName", 7000) + ";>Ljava/lang/Object;"}, scopes[1]}
	if _, ok := ProjectHiddenLexicalMethodSignature(large, "("+strings.Repeat("TT;", 32)+")V"); ok {
		t.Fatal("unbounded repeated bound expansion")
	}
}

func TestHiddenLexicalClassProjectionKeepsOwnArityAndBoundStructure(t *testing.T) {
	for _, row := range []struct{ name, outer, own, want string }{
		{"dependent own", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "<U:TT;>Ljava/lang/Object;", "<U:Ljava/lang/Number;>Ljava/lang/Object;"},
		{"member bound", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "<U:Lproof/Outer<TT;>.Leaf;>Ljava/lang/Object;", "<U:Lproof/Outer$Leaf;>Ljava/lang/Object;"},
		{"shadow", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "<T:Ljava/lang/CharSequence;>Ljava/lang/Object;", "<T:Ljava/lang/CharSequence;>Ljava/lang/Object;"},
		{"dependent chain", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "<U:TT;V:TU;>Ljava/lang/Object;", "<U:Ljava/lang/Number;V:TU;>Ljava/lang/Object;"},
		{"secondary recursive bound", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "<U:TT;:Ljava/lang/Comparable<TU;>;>Ljava/lang/Object;", "<U:Ljava/lang/Number;:Ljava/lang/Comparable<TU;>;>Ljava/lang/Object;"},
		{"nested supertype", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "<U:Ljava/lang/Object;>Ljava/util/ArrayList<Ljava/util/Map<TU;TT;>;>;", "<U:Ljava/lang/Object;>Ljava/util/ArrayList<Ljava/util/Map;>;"},
		{"direct supertype", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "<U:Ljava/lang/Object;>Ljava/util/ArrayList<TT;>;", "<U:Ljava/lang/Object;>Ljava/util/ArrayList;"},
		{"unknown bound", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "<U:TX;>Ljava/lang/Object;", ""},
		{"bound cycle", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "<U:TV;V:TU;>Ljava/lang/Object;", ""},
		{"duplicate own", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "<U:TT;U:TT;>Ljava/lang/Object;", ""},
		{"trailing junk", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "<U:TT;>Ljava/lang/Object;garbage", ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			scopes := []LexicalTypeScope{{Signature: row.outer}, {Signature: row.own}}
			before := append([]LexicalTypeScope(nil), scopes...)
			got, valid := ProjectHiddenLexicalClassSignature(scopes)
			if valid != (row.want != "") || valid && got != row.want {
				t.Fatalf("got=%q valid=%v want=%q", got, valid, row.want)
			}
			if !reflect.DeepEqual(before, scopes) {
				t.Fatal("changed original declarations")
			}
		})
	}
}
