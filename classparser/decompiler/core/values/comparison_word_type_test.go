package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestComparisonWordCannotBecomeCanonicalBooleanByConsumerRetyping(t *testing.T) {
	ctx := &class_context.ClassContext{}
	for _, typ := range []string{types.JavaLong, types.JavaFloat, types.JavaDouble} {
		for _, high := range []bool{false, true} {
			a := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(typ))
			b := NewJavaRef(utils.NewRootVariableId().Next(), nil, types.NewJavaPrimer(typ))
			word := NewJavaComparisonWord(a, b, high)
			if structurallyBooleanForIntCoerce(word, ctx) || OriginalBooleanStackWord(word) {
				t.Fatal("three-way result narrowed to 0/1")
			}
			if CoerceIntAssignRHS(types.NewJavaPrimer(types.JavaInteger), word, ctx) != word {
				t.Fatal("int consumer changed numeric word")
			}
			// Type() returns its intrinsic category; a branch consumer may not mutate
			// the original producer into a Boolean value used by another consumer.
			word.Type().ResetType(types.NewJavaPrimer(types.JavaBoolean))
			if word.Type().String(nil) != types.JavaInteger || structurallyBooleanForIntCoerce(word, ctx) {
				t.Fatal("consumer retyping escaped")
			}
			copy := *word
			if copy.Type().String(nil) != types.JavaInteger || copy.JavaValue1 != a || copy.JavaValue2 != b {
				t.Fatal("comparison copy lost category/inputs")
			}
			ref := NewJavaRef(utils.NewRootVariableId().Next(), word, types.NewJavaPrimer(types.JavaBoolean))
			ref.MarkOriginalStackMaterialization(12, 148, word)
			if OriginalBooleanStackWord(ref) {
				t.Fatal("shared capture certified noncanonical three-way word")
			}
			predicate := word.Predicate(LT)
			if !isBooleanTyped(predicate) {
				t.Fatal("zero branch failed to produce a Boolean")
			}
		}
	}
	// The same coercion still applies to actual predicate comparisons.
	predicate := NewJavaCompare(NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)), NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger)))
	if !structurallyBooleanForIntCoerce(predicate, ctx) {
		t.Fatal("existing predicate no longer Boolean")
	}
}
