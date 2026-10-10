package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

type countedDefinitionType struct {
	values.JavaValue
	calls int
}

func (v *countedDefinitionType) Type() types.JavaType {
	v.calls++
	return v.JavaValue.Type()
}

func TestWebDefinitionTypesVisitsSharedValuesOnce(t *testing.T) {
	left := &countedDefinitionType{JavaValue: values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))}
	right := &countedDefinitionType{JavaValue: values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))}
	var a, b values.JavaValue = left, right
	condition := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
	// Only 96 ternaries, but a recursive tree expansion would visit 2^48 leaves.
	for depth := 0; depth < 48; depth++ {
		a, b = values.NewTernaryExpression(condition, a, b), values.NewTernaryExpression(condition, b, a)
	}
	got := webDefinitionTypes(a, nil)
	if len(got) != 2 || left.calls != 1 || right.calls != 1 {
		t.Fatalf("constraints=%d leaf visits=(%d,%d); shared nodes must not multiply definitions", len(got), left.calls, right.calls)
	}
	if got[0].RawType().(*types.JavaPrimer).Name != types.JavaInteger || got[1].RawType().(*types.JavaPrimer).Name != types.JavaInteger {
		t.Fatal("equal static types must retain both independent value constraints")
	}
}

func TestWebDefinitionTypesRefusesCyclesAndIncompleteArms(t *testing.T) {
	condition := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
	leaf := values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
	cycle := values.NewTernaryExpression(condition, leaf, nil)
	cycle.FalseValue = cycle
	for _, value := range []values.JavaValue{cycle, values.NewTernaryExpression(condition, leaf, nil), (*values.TernaryExpression)(nil)} {
		got := webDefinitionTypes(value, nil)
		if len(got) != 1 || got[0] != nil {
			t.Fatal("invalid decision graph must decline the declaration proof")
		}
	}
}

func TestWebDefinitionTypesKeepsRecurrenceAndDiscoveryOrder(t *testing.T) {
	self := &values.JavaRef{}
	left := values.NewJavaLiteral("left", types.NewJavaClass("proof.Left"))
	right := values.NewJavaLiteral("right", types.NewJavaClass("proof.Right"))
	condition := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
	value := values.NewTernaryExpression(condition, values.NewTernaryExpression(condition, self, left), right)
	got := webDefinitionTypes(value, map[*values.JavaRef]bool{self: true})
	if len(got) != 2 || got[0] != left.Type() || got[1] != right.Type() {
		t.Fatal("recurrence supplies no constraint; independent leaves retain true-before-false order")
	}
}
