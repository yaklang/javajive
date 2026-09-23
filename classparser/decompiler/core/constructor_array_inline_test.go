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

func arrayForConstructorTempTest() values.JavaValue {
	arrayType := types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))
	array := values.NewNewExpression(arrayType)
	array.Initializer = []values.JavaValue{values.NewJavaLiteral("item", types.NewJavaClass("java.lang.String"))}
	return array
}
