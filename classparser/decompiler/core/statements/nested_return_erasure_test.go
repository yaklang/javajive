package statements

import "testing"

func TestReturnRawViewRetainsEveryLexicalMemberSegment(t *testing.T) {
	for _, row := range []struct{ source, raw string }{
		{"Outer<T>.Entry", "Outer.Entry"}, {"Outer<T>.Middle<U>.Leaf<V>", "Outer.Middle.Leaf"}, {"pkg.Outer<Map<String,? extends List<X>>>.Entry<List<Y>>[][]", "pkg.Outer.Entry[][]"}, {"Map<K,V>", "Map"}, {" Outer$Entry<T> ", "Outer$Entry"}, {"T[][]", "T[][]"}, {"Outer<T>.Entry<U", ""}, {"Outer<T>>.Entry", ""},
	} {
		if got := erasureName(row.source); got != row.raw {
			t.Errorf("%q raw=%q want=%q", row.source, got, row.raw)
		}
	}
}
