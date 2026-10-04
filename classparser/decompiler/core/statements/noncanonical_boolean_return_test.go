package statements

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestBooleanReturnViewsRetainBranchAndNumericConsumers(t *testing.T) {
	ctx := &class_context.ClassContext{FunctionType: &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaBoolean)}}
	word := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
	word.Id.SetName("word")
	condition := NewConditionStatement(values.NewJavaCompare(word, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))), values.NEQ)
	if text := NewReturnStatement(word).String(ctx); !strings.Contains(text, "& 1") || strings.Count(text, "word") != 1 {
		t.Fatal(text)
	}
	if word.Type().String(ctx) != "int" || condition.Condition.String(ctx) != "(word) != (0)" {
		t.Fatal("branch word changed by return consumer")
	}
	for _, value := range []int{2, 3, -2, -1} {
		literal := values.NewJavaLiteral(value, types.NewJavaPrimer(types.JavaBoolean))
		want := "return false"
		if value&1 != 0 {
			want = "return true"
		}
		if text := NewReturnStatement(literal).String(ctx); text != want {
			t.Fatalf("%d: %s", value, text)
		}
		ctx.FunctionType = &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaInteger)}
		if text := NewReturnStatement(literal).String(ctx); strings.Contains(text, "false") || strings.Contains(text, "true") {
			t.Fatal("I return applied Z consumer semantics: " + text)
		}
		ctx.FunctionType = &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaBoolean)}
	}
	if text := NewReturnStatement(values.JavaNull).String(ctx); !strings.Contains(text, values.EmptySlotValuePlaceholder) {
		t.Fatal("invalid category proof fell through: " + text)
	}
}
