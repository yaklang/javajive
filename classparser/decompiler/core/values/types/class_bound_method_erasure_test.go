package types

import "testing"

func TestClassBoundMethodErasureKeepsDeclarationAndGrammarIdentity(t *testing.T) {
	for _, row := range []struct{ name, decl, sig, want string }{
		{"Object bound", "<T:Ljava/lang/Object;>Ljava/lang/Object;", "(TT;J)TT;", "(Ljava/lang/Object;J)Ljava/lang/Object;"},
		{"Number bound", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "(TT;J)TT;", "(Ljava/lang/Number;J)Ljava/lang/Number;"},
		{"interface first", "<T::Ljava/lang/Runnable;>Ljava/lang/Object;", "(TT;)V", "(Ljava/lang/Runnable;)V"},
		{"arrays and wide", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "([[TT;DJ)[TT;", "([[Ljava/lang/Number;DJ)[Ljava/lang/Number;"},
		{"class versus formal", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "(LT;TT;)LT;", "(LT;Ljava/lang/Number;)LT;"},
		{"nested binary identity", "<T:Ljava/lang/Object;>Ljava/lang/Object;", "(Lprobe/Outer<TT;>.Inner<TT;>;)V", "(Lprobe/Outer$Inner;)V"},
		{"generic bounds", "<T:Ljava/lang/Comparable<TT;>;>Ljava/lang/Object;", "(TT;)TT;", "(Ljava/lang/Comparable;)Ljava/lang/Comparable;"},
		{"checked class", "<T:Ljava/lang/Object;>Ljava/lang/Object;", "(TT;)TT;^Ljava/io/IOException;", "(Ljava/lang/Object;)Ljava/lang/Object;"},
		{"unknown variable", "<T:Ljava/lang/Object;>Ljava/lang/Object;", "(TU;)V", ""},
		{"method shadows class", "<T:Ljava/lang/Object;>Ljava/lang/Object;", "<T:Ljava/lang/Number;>(TT;)TT;", ""},
		{"dependent first bound", "<T:TU;U:Ljava/lang/Number;>Ljava/lang/Object;", "(TT;)V", ""},
		{"cannot skip dependent bound", "<T:TU;:Ljava/lang/Runnable;U:Ljava/lang/Number;>Ljava/lang/Object;", "(TT;)V", ""},
		{"no class formal", "Ljava/lang/Object;", "(Ljava/lang/Object;)V", ""},
		{"duplicate formal", "<T:Ljava/lang/Object;T:Ljava/lang/Number;>Ljava/lang/Object;", "(TT;)V", ""},
		{"invalid class shape", "<T:Ljava/lang/Object;>()V", "(TT;)V", ""},
		{"void parameter", "<T:Ljava/lang/Object;>Ljava/lang/Object;", "(V)V", ""},
		{"void array", "<T:Ljava/lang/Object;>Ljava/lang/Object;", "([V)V", ""},
		{"primitive throws", "<T:Ljava/lang/Object;>Ljava/lang/Object;", "()V^I", ""},
		{"trailing malformed", "<T:Ljava/lang/Object;>Ljava/lang/Object;", "(TT;)Vx", ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			got, known := EraseClassBoundMethodSignature(row.decl, row.sig)
			if got != row.want || known != (row.want != "") {
				t.Fatalf("erasure=%q,%v want=%q", got, known, row.want)
			}
		})
	}
}

func TestClassBoundMethodErasureRetainsCheckedThrows(t *testing.T) {
	desc, throws, known := EraseClassBoundMethodSignatureWithThrows("<E:Ljava/lang/Exception;>Ljava/lang/Object;", "(Ljava/lang/Object;)V^TE;^Ljava/io/IOException;")
	if !known || desc != "(Ljava/lang/Object;)V" || len(throws) != 2 || throws[0] != "Ljava/lang/Exception;" || throws[1] != "Ljava/io/IOException;" {
		t.Fatalf("erasure=%q throws=%v known=%v", desc, throws, known)
	}
}
