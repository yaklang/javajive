package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestDelegatingConstructorArrayTempNestedEagerUse(t *testing.T) {
	arrayType := types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))
	temp := values.NewJavaRef(utils.NewRootVariableId(), nil, arrayType)
	array := values.NewNewExpression(arrayType)
	array.Initializer = []values.JavaValue{values.NewJavaLiteral("item", types.NewJavaClass("java.lang.String"))}
	boxType := types.NewJavaClass("example.Box")
	box := &values.NewExpression{
		JavaType: boxType,
		ConstructorCall: &values.FunctionCallExpression{
			FunctionName: "<init>",
			Arguments:    []values.JavaValue{temp},
		},
	}

	uses, supported := delegatingConstructorTempUses(box, temp)
	if !supported || uses != 1 {
		t.Fatalf("nested eager constructor arg analysis = (%d, %t), want (1, true)", uses, supported)
	}
	rewritten := replaceDelegatingConstructorTemp(box, temp, array)
	got := rewritten.(*values.NewExpression).ConstructorCall.Arguments[0]
	if got != array {
		t.Fatalf("nested constructor arg was not replaced: %#v", got)
	}
}

func TestDelegatingConstructorArrayTempRejectsRepeatedAndDeferredUses(t *testing.T) {
	arrayType := types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))
	temp := values.NewJavaRef(utils.NewRootVariableId(), nil, arrayType)
	repeated := &values.NewExpression{
		JavaType: types.NewJavaClass("example.Pair"),
		ConstructorCall: &values.FunctionCallExpression{
			FunctionName: "<init>",
			Arguments:    []values.JavaValue{temp, temp},
		},
	}
	if uses, supported := delegatingConstructorTempUses(repeated, temp); !supported || uses != 2 {
		t.Fatalf("repeated-use analysis = (%d, %t), want (2, true)", uses, supported)
	}

	deferred := &values.TernaryExpression{TrueValue: temp, FalseValue: values.JavaNull}
	if uses, supported := delegatingConstructorTempUses(deferred, temp); supported || uses != 0 {
		t.Fatalf("conditional use must fail closed, got (%d, %t)", uses, supported)
	}

	assigned := &values.AssignmentExpression{Target: temp, Value: arrayForConstructorTempTest()}
	if uses, supported := delegatingConstructorTempUses(assigned, temp); supported || uses != 0 {
		t.Fatalf("assignment to the temporary must fail closed, got (%d, %t)", uses, supported)
	}
}

func TestDelegatingConstructorArraySpillsKeepArgumentOrder(t *testing.T) {
	arrayType := types.NewJavaArrayType(types.NewJavaClass("byte"))
	first := values.NewJavaRef(utils.NewRootVariableId(), nil, arrayType)
	second := values.NewJavaRef(utils.NewRootVariableId(), nil, arrayType)
	separator := values.NewJavaLiteral("|", types.NewJavaClass("java.lang.String"))

	indexes, ok := orderedDelegatingConstructorTempArgIndexes(
		[]values.JavaValue{first, separator, second}, []*values.JavaRef{first, second},
	)
	if !ok || len(indexes) != 2 || indexes[0] != 0 || indexes[1] != 2 {
		t.Fatalf("ordered spills = (%v, %t), want ([0 2], true)", indexes, ok)
	}
	nested := &values.NewExpression{
		JavaType: types.NewJavaClass("example.Format"),
		ConstructorCall: &values.FunctionCallExpression{
			FunctionName: "<init>",
			Arguments:    []values.JavaValue{first, second},
		},
	}
	indexes, ok = orderedDelegatingConstructorTempArgIndexes([]values.JavaValue{nested}, []*values.JavaRef{first, second})
	if !ok || len(indexes) != 2 || indexes[0] != 0 || indexes[1] != 0 {
		t.Fatalf("nested ordered spills = (%v, %t), want ([0 0], true)", indexes, ok)
	}
	nested.ConstructorCall.Arguments = []values.JavaValue{second, first}
	if _, ok := orderedDelegatingConstructorTempArgIndexes([]values.JavaValue{nested}, []*values.JavaRef{first, second}); ok {
		t.Fatal("reversed nested argument evaluation must not reorder array creation")
	}
	cyclic := &values.NewExpression{JavaType: types.NewJavaClass("example.Cycle")}
	cyclic.ConstructorCall = &values.FunctionCallExpression{
		FunctionName: "<init>",
		Arguments:    []values.JavaValue{first, cyclic},
	}
	if _, ok := orderedDelegatingConstructorTempArgIndexes([]values.JavaValue{cyclic}, []*values.JavaRef{first}); ok {
		t.Fatal("cyclic expression trees must fail closed instead of recursing indefinitely")
	}
	if _, ok := orderedDelegatingConstructorTempArgIndexes(
		[]values.JavaValue{second, first}, []*values.JavaRef{first, second},
	); ok {
		t.Fatal("reverse argument mapping must not reorder array creation")
	}
	if _, ok := orderedDelegatingConstructorTempArgIndexes(
		[]values.JavaValue{first, first, second}, []*values.JavaRef{first, second},
	); ok {
		t.Fatal("repeated spill use must be rejected")
	}
	deferred := &values.TernaryExpression{TrueValue: first, FalseValue: second}
	if _, ok := orderedDelegatingConstructorTempArgIndexes(
		[]values.JavaValue{deferred, second}, []*values.JavaRef{first, second},
	); ok {
		t.Fatal("conditional spill use must be rejected")
	}
}

func arrayForConstructorTempTest() values.JavaValue {
	arrayType := types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))
	array := values.NewNewExpression(arrayType)
	array.Initializer = []values.JavaValue{values.NewJavaLiteral("item", types.NewJavaClass("java.lang.String"))}
	return array
}
