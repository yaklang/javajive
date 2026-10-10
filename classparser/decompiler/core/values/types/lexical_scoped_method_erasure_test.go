package types

import (
	"reflect"
	"testing"
)

func TestLexicalScopedMethodErasureRetainsDeclarationEnvironments(t *testing.T) {
	for _, test := range []struct {
		name         string
		scopes       []LexicalTypeScope
		method, want string
		valid        bool
	}{
		{"class return", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>Ljava/lang/Object;"}}, "()TT;", "()Ljava/lang/Number;", true},
		{"method shadows class", []LexicalTypeScope{{Signature: "<T:Ljava/lang/CharSequence;>Ljava/lang/Object;"}, {Signature: "<T:Ljava/lang/Object;>(TT;)V", Method: true, Descriptor: "(Ljava/lang/Object;)V"}}, "()TT;", "()Ljava/lang/Object;", true},
		{"dependent method bound keeps old binder", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>Ljava/lang/Object;"}, {Signature: "<U:TT;>()V", Method: true, Descriptor: "()V"}, {Signature: "<T:Ljava/lang/CharSequence;>Ljava/lang/Object;"}}, "()TU;", "()Ljava/lang/Number;", true},
		{"method formals keep their own inference scope", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>Ljava/lang/Object;"}}, "<T:Ljava/lang/CharSequence;>()TT;", "()Ljava/lang/CharSequence;", true},
		{"array return", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>Ljava/lang/Object;"}}, "()[TT;", "()[Ljava/lang/Number;", true},
		{"parameterized return", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>Ljava/lang/Object;"}}, "()Ljava/util/List<TT;>;", "()Ljava/util/List;", true},
		{"unknown binder", nil, "()TT;", "", false},
		{"wrong physical enclosing method", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>(TT;)V", Method: true, Descriptor: "(Ljava/lang/Object;)V"}}, "()TT;", "", false},
		{"malformed enclosing scope", []LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>Ljava/lang/Object;x"}}, "()TT;", "", false},
		{"cyclic bound", []LexicalTypeScope{{Signature: "<T:TT;>Ljava/lang/Object;"}}, "()TT;", "", false},
		{"malformed target", nil, "()Ljava/lang/Object;x", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, _, valid := EraseLexicalScopedMethodSignatureWithThrows(test.scopes, test.method)
			if valid != test.valid || valid && got != test.want {
				t.Fatalf("erasure=%q valid=%v want=%q valid=%v", got, valid, test.want, test.valid)
			}
		})
	}
}

func TestLexicalScopedMethodErasureIsBoundedAndDoesNotMutateInput(t *testing.T) {
	storage := []LexicalTypeScope{{Signature: "<T:Ljava/lang/Exception;>Ljava/lang/Object;"}, {Signature: "sentinel"}, {Signature: "other sentinel"}}
	before := append([]LexicalTypeScope(nil), storage...)
	desc, throws, valid := EraseLexicalScopedMethodSignatureWithThrows(storage[:1], "()V^TT;")
	if !valid || desc != "()V" || !reflect.DeepEqual(throws, []string{"Ljava/lang/Exception;"}) || !reflect.DeepEqual(storage, before) {
		t.Fatal("original exception erasure or caller-owned declaration stack changed", desc, throws, valid, storage)
	}
	if _, _, valid := EraseLexicalScopedMethodSignatureWithThrows(make([]LexicalTypeScope, 129), "()V"); valid {
		t.Fatal("unbounded lexical declaration stack admitted")
	}
}
