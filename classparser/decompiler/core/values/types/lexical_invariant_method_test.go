package types

import (
	"slices"
	"testing"
)

func TestLexicalInvariantMethodSignatureRequiresBoundIndependentPorts(t *testing.T) {
	const owner = "<T:Ljava/lang/Number;>Ljava/lang/Object;"
	for _, row := range []struct{ name, class, method, want string }{
		{"method scope", "", "<U:Ljava/lang/Number;>(Ljava/lang/Class<TU;>;)Ljava/util/List<TU;>;", "(Ljava/lang/Class;)Ljava/util/List;"},
		{"shadowing", owner, "<T:Ljava/lang/String;>(Ljava/lang/Class<TT;>;)Ljava/util/List<TT;>;", "(Ljava/lang/Class;)Ljava/util/List;"},
		{"both scopes", owner, "<U:Ljava/lang/Object;>(Ljava/util/Map<TT;TU;>;)V", "(Ljava/util/Map;)V"},
		{"nested arrays", "", "<T:Ljava/lang/Object;>([Ljava/util/List<[TT;>;)V", "([Ljava/util/List;)V"},
		{"recursive argument bound", "", "<T::Ljava/lang/Comparable<TT;>;>(Ljava/lang/Class<TT;>;)V", "(Ljava/lang/Class;)V"},
		{"literal class", "", "<T:Ljava/lang/Object;>(LT;Ljava/util/List<TT;>;)LT;", "(LT;Ljava/util/List;)LT;"},
		{"ordinary throws", "", "<T:Ljava/lang/Object;>()Ljava/util/List<TT;>;^Ljava/io/IOException;", "()Ljava/util/List;"},
		{"direct method input", "", "<T:Ljava/lang/Number;>(TT;)V", ""},
		{"direct method result", "", "<T:Ljava/lang/Number;>()TT;", ""},
		{"method array input", "", "<T:Ljava/lang/Number;>([TT;)V", ""},
		{"method array result", "", "<T:Ljava/lang/Number;>()[TT;", ""},
		{"direct class input", owner, "<U:Ljava/lang/Object;>(TT;)V", ""},
		{"direct class result", owner, "<U:Ljava/lang/Object;>()TT;", ""},
		{"dependent throws", "", "<T:Ljava/lang/Exception;>()V^TT;", ""},
		{"free argument", "", "<T:Ljava/lang/Object;>(Ljava/util/List<TMissing;>;)V", ""},
		{"free bound", "", "<T:Ljava/lang/Comparable<TMissing;>;>()V", ""},
		{"method cannot bind class scope", "<X:Ljava/util/List<TT;>;>Ljava/lang/Object;", "<T:Ljava/lang/Object;>()V", ""},
		{"dependent first bound", owner, "<U:TT;>(Ljava/lang/Class<TU;>;)V", ""},
		{"duplicate formal", "", "<T:Ljava/lang/Object;T:Ljava/lang/Object;>()V", ""},
		{"primitive argument", "", "<T:Ljava/lang/Object;>(Ljava/util/List<I>;)V", ""},
		{"void array", "", "<T:Ljava/lang/Object;>(Ljava/util/List<[V>;)V", ""},
		{"trailing grammar", "", "<T:Ljava/lang/Object;>()Vjunk", ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			got, throws, known := EraseLexicalInvariantMethodSignatureWithThrows(row.class, row.method)
			if known != (row.want != "") || known && got != row.want {
				t.Fatalf("got %q %v, want %q", got, known, row.want)
			}
			if known {
				var wantThrows []string
				if row.name == "ordinary throws" {
					wantThrows = []string{"Ljava/io/IOException;"}
				}
				if !slices.Equal(throws, wantThrows) {
					t.Fatalf("throws %v, want %v", throws, wantThrows)
				}
			}
		})
	}
}
