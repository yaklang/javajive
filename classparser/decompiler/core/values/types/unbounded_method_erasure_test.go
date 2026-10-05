package types

import (
	"reflect"
	"strings"
	"testing"
)

func TestUnboundedMethodErasureUsesOnlyItsOwnDeclaredObjectBounds(t *testing.T) {
	for _, row := range []struct {
		sig, desc string
		ex        []string
		count     int
	}{
		{"<T:Ljava/lang/Object;>(TT;)TT;", "(Ljava/lang/Object;)Ljava/lang/Object;", nil, 1},
		{"<Payload:Ljava/lang/Object;Other:Ljava/lang/Object;>([TPayload;[[TOther;J)[[TPayload;", "([Ljava/lang/Object;[[Ljava/lang/Object;J)[[Ljava/lang/Object;", nil, 2},
		{"<T:Ljava/lang/Object;>(Ljava/util/List<+TT;>;)Ljava/util/List<-TT;>;^Ljava/io/IOException;", "(Ljava/util/List;)Ljava/util/List;", []string{"Ljava/io/IOException;"}, 1},
		{"<T:Ljava/lang/Object;>(LT;)LT;", "(LT;)LT;", nil, 1},
		{"<T:Ljava/lang/Object;>(Ljava/util/List<[I>;)TT;", "(Ljava/util/List;)Ljava/lang/Object;", nil, 1},
		{"<T:Ljava/lang/Number;>(TT;)TT;", "", nil, 0},
		{"<T::Ljava/lang/Runnable;>()V", "", nil, 0},
		{"<T:Ljava/lang/Object;:Ljava/io/Serializable;>()TT;", "", nil, 0},
		{"<T:Ljava/lang/Comparable<TT;>;>()TT;", "", nil, 0},
		{"<T:Ljava/lang/Object;U:TT;>()TT;", "", nil, 0},
		{"<T:Ljava/lang/Object;>(TForeign;)TT;", "", nil, 0},
		{"(Ljava/util/List<TT;>;)TT;", "", nil, 0},
		{"<T:Ljava/lang/Object;T:Ljava/lang/Object;>()V", "", nil, 0},
		{"<T:Ljava/lang/Object;>(Ljava/util/List<I>;)TT;", "", nil, 0},
		{"<T:Ljava/lang/Object;>(Ljava/util/List<[V>;)TT;", "", nil, 0},
		{"<T:Ljava/lang/Object;>(Ljava/util/List<+*>;)TT;", "", nil, 0},
		{"<T:Ljava/lang/Object;>(Ljava/util/List<>;)TT;", "", nil, 0},
		{"<T:Ljava/lang/Object;>(V)V", "", nil, 0},
		{"<T:Ljava/lang/Object;>([V)V", "", nil, 0},
		{"<T:Ljava/lang/Object;>()V^TT;", "", nil, 0},
		{"<T:Ljava/lang/Object;>()Vx", "", nil, 0},
		{"<T:Ljava/lang/Object;>(" + strings.Repeat("[", 130) + "TT;)V", "", nil, 0},
		{"<T:Ljava/lang/Object;>(" + strings.Repeat("[", 4100) + "TT;)V", "", nil, 0},
	} {
		t.Run(row.sig, func(t *testing.T) {
			d, ex, own, ok := UnboundedMethodErasure(row.sig)
			if ok != (row.desc != "") || d != row.desc || !reflect.DeepEqual(ex, row.ex) || len(own) != row.count {
				t.Fatalf("got %q %v %v %v", d, ex, own, ok)
			}
		})
	}
}
