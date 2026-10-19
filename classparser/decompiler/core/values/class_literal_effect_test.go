package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestEvaluatedClassLiteralResolutionIsThrowingWithoutClassInitialization(t *testing.T) {
	for _, subject := range []types.JavaType{types.NewJavaClass("example.Owner"), types.NewJavaArrayType(types.NewJavaClass("example.Owner")), types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))} {
		literal := NewJavaClassValue(subject)
		literal.HasOriginPC, literal.OriginPC = true, 17
		effects, refs := InspectValue(literal)
		if effects&EffectThrow == 0 || effects&EffectClassInit != 0 || effects&EffectOpaque != 0 || len(refs) != 0 || IsPure(literal) || MayFold(literal) {
			t.Fatalf("ldc Class must preserve resolution without introducing initialization: effects=%v refs=%v", effects, refs)
		}
		owner := NewJavaClassValue(subject)
		if effects, _ := InspectValue(owner); effects != 0 {
			t.Fatal("symbolic invocation owner was turned into an evaluated class literal")
		}
	}
}
