package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestUniqueParameterizedConstraintRejectsIncompatibleTargets(t *testing.T) {
	function := func(input, output string) types.JavaType {
		return types.NewParameterizedType("java.util.function.Function", []types.JavaType{
			types.NewJavaClass(input),
			types.NewJavaClass(output),
		})
	}
	stringsTarget := function("java.lang.String", "java.lang.String")
	integersTarget := function("java.lang.Integer", "java.lang.Integer")

	target, conflict := uniqueParameterizedConstraint("java.util.function.Function", []types.JavaType{
		stringsTarget,
		stringsTarget.Copy(),
		types.NewJavaClass("java.util.function.Function"), // Raw evidence does not narrow the target.
	})
	if conflict || target == nil {
		t.Fatalf("identical generic targets should agree: target=%v conflict=%v", target, conflict)
	}

	target, conflict = uniqueParameterizedConstraint("java.util.function.Function", []types.JavaType{
		stringsTarget,
		integersTarget,
	})
	if !conflict || target != nil {
		t.Fatalf("incompatible generic uses must preserve erasure: target=%v conflict=%v", target, conflict)
	}

	// A parameterized constraint for another erasure is unrelated evidence and
	// cannot target the Function value.
	target, conflict = uniqueParameterizedConstraint("java.util.function.Function", []types.JavaType{
		types.NewParameterizedType("java.util.function.Supplier", []types.JavaType{
			types.NewJavaClass("java.lang.String"),
		}),
	})
	if conflict || target != nil {
		t.Fatalf("different erasure must be ignored: target=%v conflict=%v", target, conflict)
	}
}
