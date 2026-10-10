package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestBooleanReturnInferenceDoesNotRetypeSharedInt(t *testing.T) {
	ctx := &class_context.ClassContext{FunctionType: &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaBoolean)}}
	for _, value := range []values.JavaValue{values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger)), values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger)), values.NewBinaryExpression(values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger)), values.NewJavaLiteral(3, types.NewJavaPrimer(types.JavaInteger)), values.ADD, types.NewJavaPrimer(types.JavaInteger))} {
		before := value.String(ctx)
		resetReturnValueTypeSafe(value, ctx)
		if value.Type().String(ctx) != "int" || value.String(ctx) != before {
			t.Fatalf("Z sink mutated computational producer: %s", value.String(ctx))
		}
	}
}
