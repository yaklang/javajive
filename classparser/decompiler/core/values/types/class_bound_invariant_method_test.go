package types

import "testing"

func TestClassBoundInvariantMethodSignatureRequiresErasureIndependentPorts(t *testing.T) {
	const owner = "<T:Ljava/lang/Comparable<TT;>;>Ljava/lang/Object;"
	for _, row := range []struct{ name, class, method, want string }{
		{"nested", owner, "(Ljava/util/List<TT;>;)Ljava/util/List<+TT;>;", "(Ljava/util/List;)Ljava/util/List;"},
		{"nested array", owner, "([Ljava/util/List<[TT;>;)V", "([Ljava/util/List;)V"},
		{"extra nested terminator", owner, "([Ljava/util/List<[TT;>;>;)V", ""},
		{"owner segments", owner, "(Lp/Outer<TT;>.Inner<TT;>;)V", "(Lp/Outer$Inner;)V"},
		{"literal class named T", owner, "(LT;Ljava/util/List<TT;>;)LT;", "(LT;Ljava/util/List;)LT;"},
		{"ordinary throws", owner, "(Ljava/util/List<TT;>;)V^Ljava/io/IOException;", "(Ljava/util/List;)V"},
		{"erasure matches but substitution changes array", "<T:Ljava/util/List<Ljava/lang/String;>;>Ljava/lang/Object;", "([TT;)V", ""},
		{"direct parameter", owner, "(TT;)V", ""},
		{"array parameter", owner, "([TT;)V", ""},
		{"direct return", owner, "()TT;", ""},
		{"array return", owner, "()[TT;", ""},
		{"dependent throws", "<T:Ljava/lang/Exception;>Ljava/lang/Object;", "()V^TT;", ""},
		{"free nested variable", owner, "(Ljava/util/List<TU;>;)V", ""},
		{"method formal", owner, "<U:Ljava/lang/Object;>(Ljava/util/List<TU;>;)V", ""},
		{"shadowing formal", owner, "<T:Ljava/lang/Object;>(Ljava/util/List<TT;>;)V", ""},
		{"primitive argument", owner, "(Ljava/util/List<I>;)V", ""},
		{"void array argument", owner, "(Ljava/util/List<[V>;)V", ""},
		{"incomplete method", owner, "(Ljava/util/List<TT;>;)Vgarbage", ""},
		{"undeclared class variable", "", "(Ljava/util/List<TT;>;)V", ""},
		{"malformed bound", "<T:Ljava/lang/Comparable<I>;>Ljava/lang/Object;", "(Ljava/util/List<TT;>;)V", ""},
		{"method-shaped owner", "<T:Ljava/lang/Object;>()V", "(Ljava/util/List<TT;>;)V", ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			got, throws, ok := EraseClassBoundInvariantMethodSignatureWithThrows(row.class, row.method)
			if ok != (row.want != "") || ok && got != row.want {
				t.Fatalf("got %q %v, want %q", got, ok, row.want)
			}
			if ok {
				if row.name == "ordinary throws" {
					if len(throws) != 1 || throws[0] != "Ljava/io/IOException;" {
						t.Fatal("throws lost", throws)
					}
				} else if len(throws) != 0 {
					t.Fatal("invented throws", throws)
				}
			}
		})
	}
}
