package types

import (
	"reflect"
	"strings"
	"testing"
)

func TestFormalBoundConstraintsKeepGrammarAndDeclarationRoles(t *testing.T) {
	for _, fixture := range []struct {
		signature string
		bounds    map[string]int
		inputs    map[int]string
		valid     bool
	}{
		{"<T:Lprobe/Base;:Lprobe/Facet;>(TT;I)TT;", map[string]int{"T": 2}, map[int]string{0: "T"}, true},
		{"<T::Lprobe/Facet;>(TT;)V", map[string]int{"T": 1}, map[int]string{0: "T"}, true},
		{"<T:LT;:Lprobe/Facet;>(LT;TT;[TT;)V", map[string]int{"T": 2}, map[int]string{1: "T"}, true},
		{"<A:Ljava/lang/Comparable<TA;>;:Ljava/io/Serializable;B:Ljava/lang/Object;>(TB;Ljava/util/List<TA;>;TA;)V", map[string]int{"A": 2, "B": 1}, map[int]string{0: "B", 2: "A"}, true},
		{"<T:Ljava/lang/Object;>Ljava/lang/Object;", nil, nil, false},
		{"<T:Ljava/lang/Object;T:Ljava/lang/Object;>(TT;)V", nil, nil, false},
		{"<T:TU;>(TT;)V", nil, nil, false},
		{"<T:[Ljava/lang/Object;>(TT;)V", nil, nil, false},
		{"<T:>(TT;)V", nil, nil, false},
		{"<T:Ljava/lang/Object;>(V)V", nil, nil, false},
		{"<T:Ljava/lang/Object;>(TT;", nil, nil, false},
		{"<T:Ljava/lang/Object;>(TT;)Vgarbage", nil, nil, false},
	} {
		t.Run(fixture.signature, func(t *testing.T) {
			bounds, inputs, valid := FormalBoundConstraints(fixture.signature)
			if valid != fixture.valid {
				t.Fatalf("valid=%v", valid)
			}
			if !valid {
				return
			}
			lengths := map[string]int{}
			for name, list := range bounds {
				lengths[name] = len(list)
			}
			if !reflect.DeepEqual(lengths, fixture.bounds) || !reflect.DeepEqual(inputs, fixture.inputs) {
				t.Fatalf("bounds=%v inputs=%v", lengths, inputs)
			}
		})
	}
}

func TestFormalBoundConstraintsRetainSignatureResourceLimits(t *testing.T) {
	for _, signature := range []string{
		"<T:" + strings.Repeat("Lprobe/Box<", 129) + "Ljava/lang/Object;" + strings.Repeat(">;", 129) + ">(TT;)V",
		strings.Repeat("x", 65536),
	} {
		if _, _, valid := FormalBoundConstraints(signature); valid {
			t.Fatal("admitted unbounded signature")
		}
	}
}

func TestFormalTypeBoundsKeepClassAndMethodOrderedIntersections(t *testing.T) {
	for _, tail := range []string{"Ljava/lang/Object;", "(TT;)TT;"} {
		for _, bounds := range []string{"Ljava/lang/Object;:Ljava/lang/CharSequence;", "Ljava/lang/Object;:Ljava/io/Serializable;:Ljava/lang/CharSequence;", ":Ljava/lang/CharSequence;"} {
			signature := "<T:" + bounds + ">" + tail
			t.Run(signature, func(t *testing.T) {
				constraints, valid := FormalTypeBounds(signature)
				if !valid || len(constraints["T"]) != strings.Count(bounds, ";") {
					t.Fatalf("ordered bounds=%v valid=%v", constraints, valid)
				}
				first, known := RawClassFQN(constraints["T"][0])
				want := "java.lang.Object"
				if strings.HasPrefix(bounds, ":") {
					want = "java.lang.CharSequence"
				}
				if !known || first != want {
					t.Fatalf("first bound=%q want=%q", first, want)
				}
			})
		}
	}
	for _, signature := range []string{"<T:TU;>Ljava/lang/Object;", "<T:Ljava/lang/Object;T:Ljava/lang/Object;>Ljava/lang/Object;", "<T:[Ljava/lang/Object;>Ljava/lang/Object;", "<T:Ljava/lang/Object;>Ljava/lang/Object;garbage", strings.Repeat("x", 65536)} {
		if _, valid := FormalTypeBounds(signature); valid {
			t.Fatalf("invalid signature admitted: %.80s", signature)
		}
	}
}

func TestFormalSourceBoundsDoNotEraseFirstObjectInIntersection(t *testing.T) {
	for _, fixture := range []struct{ signature, want string }{
		{"<T:Ljava/lang/Object;>", "<T>"},
		{"<T:Ljava/lang/Object;:Ljava/lang/CharSequence;>", "<T extends Object & CharSequence>"},
		{"<T:Ljava/lang/Object;:Ljava/io/Serializable;:Ljava/lang/CharSequence;>", "<T extends Object & Serializable & CharSequence>"},
		{"<T::Ljava/lang/CharSequence;>", "<T extends CharSequence>"},
		{"<T:Lprobe/Object;>", "<T extends Object>"},
	} {
		t.Run(fixture.signature, func(t *testing.T) {
			if got := ParseMethodSignatureTypeParams(fixture.signature + "(TT;)TT;"); got != fixture.want {
				t.Fatalf("method header=%q want=%q", got, fixture.want)
			}
			bounds := ClassFormalTypeParamBounds(fixture.signature+"Ljava/lang/Object;", nil)
			clause := ""
			if b, exists := bounds["T"]; exists {
				clause = " extends " + b.Clause
			}
			if got := "<T" + clause + ">"; got != fixture.want {
				t.Fatalf("redeclared class bound=%q want=%q", got, fixture.want)
			}
		})
	}
}
