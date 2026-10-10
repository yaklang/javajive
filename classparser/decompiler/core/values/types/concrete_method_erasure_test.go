package types

import (
	"reflect"
	"strings"
	"testing"
)

func TestConcreteMethodErasureNeedsNoFormalInference(t *testing.T) {
	for _, row := range []struct {
		sig, want string
		throws    []string
	}{
		{"(Ljava/util/List<Ljava/lang/String;>;J)Ljava/util/List<Ljava/lang/String;>;", "(Ljava/util/List;J)Ljava/util/List;", nil},
		{"([[Ljava/util/List<+[I>;D)Ljava/util/List<-Ljava/lang/Number;>;^Ljava/io/IOException;", "([[Ljava/util/List;D)Ljava/util/List;", []string{"Ljava/io/IOException;"}},
		{"(Lp/Outer<LT;>.Inner<Ljava/lang/String;>;)V", "(Lp/Outer$Inner;)V", nil},
		{"(Ljava/util/List<*>;)V", "(Ljava/util/List;)V", nil},
		{"(Ljava/lang/Object;J)V", "(Ljava/lang/Object;J)V", nil},
		{"<T:Ljava/lang/Object;>(Ljava/util/List<TT;>;)V", "", nil},
		{"(Ljava/util/List<TT;>;)V", "", nil},
		{"(Ljava/util/List<I>;)V", "", nil},
		{"(Ljava/util/List<+I>;)V", "", nil},
		{"(Ljava/util/List<[V>;)V", "", nil},
		{"(Ljava/util/List<Ljava/util/List<I>;>;)V", "", nil},
		{"(Ljava/util/List<+*>;)V", "", nil},
		{"(Ljava/util/List<>;)V", "", nil},
		{"(Ljava/util/List<Ljava/lang/String;>;)V^TT;", "", nil},
		{"(V)V", "", nil}, {"([V)V", "", nil}, {"()V^I", "", nil},
		{"(Ljava/util/List<Ljava/lang/String;>;)Vx", "", nil},
		{"(" + strings.Repeat("[", 130) + "Ljava/lang/String;)V", "", nil},
	} {
		t.Run(row.sig, func(t *testing.T) {
			got, throws, known := EraseConcreteMethodSignatureWithThrows(row.sig)
			if known != (row.want != "") || got != row.want || !reflect.DeepEqual(throws, row.throws) {
				t.Fatalf("got %q %v %v want %q %v", got, throws, known, row.want, row.throws)
			}
		})
	}
}
