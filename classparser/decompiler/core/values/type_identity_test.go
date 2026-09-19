package values

import (
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
