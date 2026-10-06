package types

import (
	"reflect"
	"testing"
)

func TestLexicalMethodErasureUsesOriginalClassAndMethodBinders(t *testing.T) {
	for _, tc := range []struct {
		name, class, method, want string
		throws                    []string
	}{
		{name: "static formal", method: "<T::Ljava/lang/CharSequence;>(TT;J)TT;", want: "(Ljava/lang/CharSequence;J)Ljava/lang/CharSequence;"},
		{name: "shadowing formal", class: "<T:Ljava/lang/Number;>Ljava/lang/Object;", method: "<T::Ljava/lang/CharSequence;>(TT;J)TT;", want: "(Ljava/lang/CharSequence;J)Ljava/lang/CharSequence;"},
		{name: "unbounded formal", class: "<T:Ljava/lang/Number;>Ljava/lang/Object;", method: "<T:Ljava/lang/Object;>(TT;)TT;", want: "(Ljava/lang/Object;)Ljava/lang/Object;"},
		{name: "class formal", class: "<T:Ljava/lang/Number;>Ljava/lang/Object;", method: "([TT;)TT;", want: "([Ljava/lang/Number;)Ljava/lang/Number;"},
		{name: "plain class scope", class: "Ljava/lang/Object;", method: "<U:Ljava/lang/Number;>(TU;)TU;", want: "(Ljava/lang/Number;)Ljava/lang/Number;"},
		{name: "literal class T", method: "(LT;)LT;", want: "(LT;)LT;"},
		{name: "generic concrete array", method: "(J[Ljava/util/List<Ljava/lang/String;>;)Ljava/util/List<Ljava/lang/String;>;", want: "(J[Ljava/util/List;)Ljava/util/List;"},
		{name: "throws scopes", class: "<E:Ljava/lang/Exception;>Ljava/lang/Object;", method: "<E:Ljava/io/IOException;>()V^TE;", want: "()V", throws: []string{"Ljava/io/IOException;"}},
		{name: "static cannot borrow class formal", method: "(TT;)TT;"},
		{name: "method cannot repair invalid class scope", class: "<T:Ljava/lang/Object;>Ljava/util/List<TX;>;", method: "<X:Ljava/lang/Object;>()V"},
		{name: "dependent method first bound", class: "<T:Ljava/lang/Number;>Ljava/lang/Object;", method: "<U:TT;>(TU;)TU;"},
		{name: "unknown method ref", method: "<U:Ljava/lang/Object;>(TX;)TU;"},
		{name: "primitive generic argument", method: "(Ljava/util/List<I>;)V"},
		{name: "void array", method: "([V)V"},
		{name: "class signature is not a method", class: "()V", method: "()V"},
		{name: "class signature is not a field", class: "TT;", method: "()V"},
		{name: "duplicate method binder", method: "<T:Ljava/lang/Object;T:Ljava/lang/Object;>()V"},
		{name: "missing signature", method: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, throws, ok := EraseLexicalMethodSignatureWithThrows(tc.class, tc.method)
			if ok != (tc.want != "") || got != tc.want || len(throws) != len(tc.throws) || len(throws) > 0 && !reflect.DeepEqual(throws, tc.throws) {
				t.Fatalf("erasure %q, throws %v, known %v; want %q/%v", got, throws, ok, tc.want, tc.throws)
			}
		})
	}
}
