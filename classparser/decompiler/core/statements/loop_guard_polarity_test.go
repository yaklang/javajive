package statements

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// The structurer already assigns the true and false arms. Rendering cannot
// infer a different exit direction from the comparison's spelling.
func TestDoWhileRenderingKeepsGuardPolarity(t *testing.T) {
	ctx := &class_context.ClassContext{}
	id := utils.NewRootVariableId()
	id.SetName("reader")
	receiver := values.NewJavaRef(id, nil, types.NewJavaClass("Reader"))
	for _, operator := range []string{"<", "<=", ">", ">=", "==", "!="} {
		for _, prefixed := range []bool{false, true} {
			condition := values.NewBinaryExpression(
				&values.FunctionCallExpression{Object: receiver, FunctionName: "remaining", FuncType: &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaInteger)}},
				values.NewJavaLiteral(23, types.NewJavaPrimer(types.JavaInteger)),
				operator, types.NewJavaPrimer(types.JavaBoolean),
			)
			guard := NewIfStatement(condition, []Statement{NewCustomStatement(func(*class_context.ClassContext) string { return "break" }, nil)},
				[]Statement{NewCustomStatement(func(*class_context.ClassContext) string { return "consume()" }, nil)})
			body := []Statement{guard}
			if prefixed {
				body = append([]Statement{NewCustomStatement(func(*class_context.ClassContext) string { return "mark()" }, nil)}, body...)
			}
			loop := NewDoWhileStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), body)
			loop.Label = "scan"
			want := "if (" + condition.String(ctx) + "){"
			for pass := 0; pass < 2; pass++ {
				got := loop.String(ctx)
				if !strings.Contains(got, want) || strings.Count(got, "remaining()") != 1 || guard.Condition != condition {
					t.Errorf("operator=%s prefixed=%v pass=%d: rendering changed the condition or its evaluation: %s", operator, prefixed, pass, got)
				}
			}
		}
	}
}
