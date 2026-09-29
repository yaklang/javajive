package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestInvocationTypeIndependentOfLocalInference(t *testing.T) {
	expected := types.NewJavaClass("sample.Left")
	call := &FunctionCallExpression{FuncType: &types.JavaFuncType{ReturnType: expected}}
	provisional := call.Type()
	provisional.ResetTypeRef(types.NewJavaClass("sample.Right"))
	slot := NewSlotValue(nil, types.NewJavaClass("sample.Right"))
	slot.ResetValue(call)
	if got, _ := types.RawClassFQN(call.Type()); got != "sample.Left" {
		t.Fatalf("descriptor overwritten: %s", got)
	}
	cast := &CastExpression{TargetType: types.NewJavaClass("sample.Left")}
	slot.ResetValue(cast)
	if got, _ := types.RawClassFQN(cast.Type()); got != "sample.Left" {
		t.Fatalf("checkcast overwritten: %s", got)
	}
}

func TestAllocationTypeIndependentOfLocalInference(t *testing.T) {
	ctx := &class_context.ClassContext{}
	for _, typ := range []types.JavaType{types.NewJavaClass("sample.Concrete"), types.NewJavaArrayType(types.NewJavaClass("sample.Element"))} {
		allocation := NewNewExpression(typ)
		want := allocation.Type().Copy()
		inferred := allocation.Type()
		inferred.ResetTypeRef(types.NewJavaClass("java.lang.Object"))
		NewSlotValue(nil, types.NewJavaClass("java.lang.Object")).ResetValue(allocation)
		if got := allocation.Type().String(ctx); got != want.String(ctx) {
			t.Fatalf("allocation instruction type overwritten: %s want %s", got, want.String(ctx))
		}
	}
}
