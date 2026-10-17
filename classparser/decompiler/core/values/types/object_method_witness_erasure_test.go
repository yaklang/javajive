package types

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestObjectMethodWitnessUsesOnlyClosedObjectBoundGraph(t *testing.T) {
	for _, row := range []struct {
		sig, descriptor string
		throws          []string
	}{
		{"<B:Ljava/lang/Object;T:TB;>(Ljava/lang/Class<TT;>;TB;)TT;", "(Ljava/lang/Class;Ljava/lang/Object;)Ljava/lang/Object;", nil},
		{"<T:TU;U:Ljava/lang/Object;>([TT;[[TU;J)[[TT;", "([Ljava/lang/Object;[[Ljava/lang/Object;J)[[Ljava/lang/Object;", nil},
		{"<A:Ljava/lang/Object;B:TA;C:TB;>(TC;)TA;^Ljava/io/IOException;", "(Ljava/lang/Object;)Ljava/lang/Object;", []string{"Ljava/io/IOException;"}},
		{"<T:Ljava/lang/Object;>(LT;)LT;", "(LT;)LT;", nil},
		{"<T:Ljava/lang/Object;U:TT;>(Ljava/util/List<+TU;>;Ljava/util/List<[I>;)Ljava/util/List<-TT;>;", "(Ljava/util/List;Ljava/util/List;)Ljava/util/List;", nil},
		{"<T:Ljava/lang/Number;U:TT;>()TU;", "", nil},
		{"<T:Ljava/lang/Object;U:TT;:Ljava/io/Serializable;>()TU;", "", nil},
		{"<T::Ljava/lang/Runnable;>()V", "", nil},
		{"<T:Ljava/lang/Comparable<TT;>;>()TT;", "", nil},
		{"<T:LT;>()TT;", "", nil},
		{"<T:TOuter;>()TT;", "", nil},
		{"<T:TU;U:TT;>()TT;", "", nil},
		{"<T:TT;>()TT;", "", nil},
		{"<T:Ljava/lang/Object;T:Ljava/lang/Object;>()V", "", nil},
		{"<T:Ljava/lang/Object;>(TForeign;)TT;", "", nil},
		{"<T:Ljava/lang/Object;>(Ljava/util/List<I>;)TT;", "", nil},
		{"<T:Ljava/lang/Object;>(Ljava/util/List<[V>;)TT;", "", nil},
		{"<T:Ljava/lang/Object;>(Ljava/util/List<+*>;)TT;", "", nil},
		{"<T:Ljava/lang/Object;>(V)V", "", nil},
		{"<T:Ljava/lang/Object;>()V^TT;", "", nil},
		{"<T:Ljava/lang/Object;U:TT;>()Vx", "", nil},
		{"<T:Ljava/lang/Object;>(" + strings.Repeat("[", 130) + "TT;)V", "", nil},
		{"<T:Ljava/lang/Object;>(" + strings.Repeat("[", 4100) + "TT;)V", "", nil},
	} {
		t.Run(row.sig, func(t *testing.T) {
			d, ex, own, ok := ObjectWitnessMethodErasure(row.sig)
			if ok != (row.descriptor != "") || d != row.descriptor || !reflect.DeepEqual(ex, row.throws) || ok && len(own) == 0 || !ok && len(own) != 0 {
				t.Fatalf("witness=%q throws=%v formals=%v known=%v", d, ex, own, ok)
			}
		})
	}
}

// Enumerate all unary-bound graphs through four formals. The oracle follows
// each node independently until Object or a repeated node; it does not reuse
// the implementation's tri-color traversal or erasure parser.
func TestObjectMethodWitnessMatchesIndependentFiniteGraphModel(t *testing.T) {
	graphs := 0
	for size := 1; size <= 4; size++ {
		limit := 1
		for i := 0; i < size; i++ {
			limit *= size + 1
		}
		for word := 0; word < limit; word++ {
			parents, rest := make([]int, size), word
			var declaration, parameters strings.Builder
			declaration.WriteByte('<')
			for i := range parents {
				parents[i], rest = rest%(size+1)-1, rest/(size+1)
				if parents[i] < 0 {
					fmt.Fprintf(&declaration, "N%d:Ljava/lang/Object;", i)
				} else {
					fmt.Fprintf(&declaration, "N%d:TN%d;", i, parents[i])
				}
				fmt.Fprintf(&parameters, "TN%d;", i)
			}
			want := true
			for start := range parents {
				seen := map[int]bool{}
				for current := start; current >= 0; current = parents[current] {
					if seen[current] {
						want = false
						break
					}
					seen[current] = true
				}
			}
			signature := declaration.String() + ">(" + parameters.String() + ")TN0;"
			d, _, own, ok := ObjectWitnessMethodErasure(signature)
			if ok != want || ok && (d != "("+strings.Repeat("Ljava/lang/Object;", size)+")Ljava/lang/Object;" || len(own) != size) {
				t.Fatalf("parents=%v witness=%v descriptor=%s expected=%v", parents, ok, d, want)
			}
			graphs++
		}
	}
	if graphs != 700 {
		t.Fatalf("incomplete graph model: %d", graphs)
	}
}

func TestObjectMethodWitnessBoundsFormalCountAndRetainsOldUnboundedDomain(t *testing.T) {
	for _, size := range []int{128, 129} {
		var signature strings.Builder
		signature.WriteByte('<')
		for i := 0; i < size; i++ {
			if i == 0 {
				signature.WriteString("N0:Ljava/lang/Object;")
			} else {
				fmt.Fprintf(&signature, "N%d:TN%d;", i, i-1)
			}
		}
		signature.WriteString(">()TN0;")
		_, _, own, ok := ObjectWitnessMethodErasure(signature.String())
		if ok != (size == 128) || ok && len(own) != size {
			t.Fatalf("formal count=%d admitted=%v count=%d", size, ok, len(own))
		}
	}
	const dependent = "<B:Ljava/lang/Object;T:TB;>(TB;)TT;"
	if _, _, _, ok := UnboundedMethodErasure(dependent); ok {
		t.Fatal("separate witness capability leaked into direct-unbounded domain")
	}
}
