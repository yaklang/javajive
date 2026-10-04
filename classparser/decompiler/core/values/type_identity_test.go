package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestArrayLoadInferenceCannotMutateArrayComponentOrCast(t *testing.T) {
	ctx := &class_context.ClassContext{}
	for _, element := range []types.JavaType{types.NewJavaPrimer(types.JavaInteger), types.NewJavaPrimer(types.JavaBoolean), types.NewJavaClass("java.lang.StackTraceElement"), types.NewJavaClass("T"), types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))} {
		t.Run(element.String(ctx), func(t *testing.T) {
			arrayType := types.NewJavaArrayType(element)
			want := arrayType.String(ctx)
			array := NewJavaRef(utils.NewRootVariableId(), nil, arrayType.Copy())
			array.WebDeclType = arrayType.Copy()
			cast := &CastExpression{Value: JavaNull, TargetType: arrayType.Copy()}
			load := NewJavaArrayMember(array, NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)))
			inferred := load.Type()
			inferred.ResetTypeRef(types.NewJavaClass("java.lang.Object"))
			NewSlotValue(nil, types.NewJavaClass("java.lang.Object")).ResetValue(load)
			for _, typ := range []types.JavaType{array.Type(), array.WebDeclType, cast.Type()} {
				if got := typ.String(ctx); got != want {
					t.Fatalf("array descriptor mutated by element inference: %s want %s", got, want)
				}
			}
			if got := load.Type().String(ctx); got != element.String(ctx) {
				t.Fatalf("load lost its component type: %s", got)
			}
		})
	}
}

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
