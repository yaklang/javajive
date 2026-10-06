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
		{"primitive generic bound", "<T:Ljava/lang/Number<I>;>Ljava/lang/Object;", "(TT;)TT;", ""},
		{"primitive generic parameter", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "(Ljava/lang/Object<I>;J)TT;", ""},
		{"void generic array bound", "<T:Ljava/lang/Number<[V>;>Ljava/lang/Object;", "(TT;)TT;", ""},
		{"primitive generic super", "<T:Ljava/lang/Number;>Ljava/util/ArrayList<I>;", "(TT;)TT;", ""},
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

func TestRawClassInstanceMethodErasureKeepsSeparateLexicalDeclarations(t *testing.T) {
	for _, row := range []struct{ name, decl, sig, want string }{
		{"class only", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "(TT;J)TT;", "(Ljava/lang/Number;J)Ljava/lang/Number;"},
		{"method shadows class", "<T:Ljava/lang/CharSequence;>Ljava/lang/Object;", "<T:Ljava/lang/Number;>(TT;J)TT;", "(Ljava/lang/Number;J)Ljava/lang/Number;"},
		{"method and class", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "<T::Ljava/lang/Comparable<TT;>;>(TT;J)TK;", "(Ljava/lang/Comparable;J)Ljava/lang/Number;"},
		{"generic bound has class argument", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "<T::Ljava/lang/Comparable<TK;>;>(TT;)TK;", "(Ljava/lang/Comparable;)Ljava/lang/Number;"},
		{"wide arrays", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "<T:Ljava/lang/String;>([[TT;DJ)[TK;", "([[Ljava/lang/String;DJ)[Ljava/lang/Number;"},
		{"multiple method bounds", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "<T:Ljava/lang/Object;:Ljava/io/Serializable;>(TT;)TK;", "(Ljava/lang/Object;)Ljava/lang/Number;"},
		{"class named T", "<T:Ljava/lang/Number;>Ljava/lang/Object;", "<T:Ljava/lang/String;>(LT;TT;)LT;", "(LT;Ljava/lang/String;)LT;"},
		{"free outer remains free", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "<T:Ljava/lang/String;>(TU;)TK;", ""},
		{"method cannot repair class scope", "<K:Ljava/lang/Comparable<TT;>;>Ljava/lang/Object;", "<T:Ljava/lang/String;>()TK;", ""},
		{"dependent method bound", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "<T:TK;>(TT;)TK;", ""},
		{"duplicate method formals", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "<T:Ljava/lang/String;T:Ljava/lang/Number;>(TT;)TK;", ""},
		{"primitive method argument", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "<T:Ljava/lang/String;>(Ljava/util/List<I>;)TK;", ""},
		{"primitive method bound", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "<T:Ljava/util/List<I>;>(TT;)TK;", ""},
		{"void method array", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "<T:Ljava/lang/String;>([V)TK;", ""},
		{"array additional bound", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "<T:Ljava/lang/Number;:[I>(TT;)TK;", ""},
		{"type variable additional bound", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "<T:Ljava/lang/Number;:TK;>(TT;)TK;", ""},
		{"repeated empty bound", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "<T:::Ljava/lang/Runnable;>(TT;)TK;", ""},
		{"nongeneric class remains generic method", "Ljava/lang/Object;", "<T:Ljava/lang/String;>(TT;)TT;", ""},
		{"malformed method suffix", "<K:Ljava/lang/Number;>Ljava/lang/Object;", "<T:Ljava/lang/String;>(TT;)TK;x", ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			desc, _, known := EraseRawClassInstanceMethodSignatureWithThrows(row.decl, row.sig)
			if desc != row.want || known != (row.want != "") {
				t.Fatalf("erasure=%q,%v want=%q", desc, known, row.want)
			}
		})
	}
	desc, throws, known := EraseRawClassInstanceMethodSignatureWithThrows("<K:Ljava/lang/Number;>Ljava/lang/Object;", "<E:Ljava/io/IOException;>(TK;)TK;^TE;")
	if !known || desc != "(Ljava/lang/Number;)Ljava/lang/Number;" || len(throws) != 1 || throws[0] != "Ljava/io/IOException;" {
		t.Fatalf("checked erasure=%q throws=%v known=%v", desc, throws, known)
	}
}
