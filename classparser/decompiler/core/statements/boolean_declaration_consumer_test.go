package statements

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestBooleanLocalDeclarationUsesItsConsumerDomain(t *testing.T) {
	for _, target := range []string{types.JavaBoolean, types.JavaInteger} {
		for _, words := range [][2]int{{0, 1}, {2, -1}, {-2, 3}} {
			for _, first := range []bool{false, true} {
				condition := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
				condition.IsParam = true
				condition.Id.SetName("condition")
				right := values.NewTernaryExpression(condition, values.NewJavaLiteral(words[0], types.NewJavaPrimer(types.JavaInteger)), values.NewJavaLiteral(words[1], types.NewJavaPrimer(types.JavaInteger)))
				left := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(target))
				left.Id.SetName("result")
				statement := &AssignStatement{LeftValue: left, JavaValue: right, IsFirst: first}
				got := statement.String(&class_context.ClassContext{})
				prefix := "result = "
				if first {
					prefix = target + " " + prefix
				}
				if !strings.HasPrefix(got, prefix) {
					t.Fatalf("target=%s words=%v first=%v source=%s", target, words, first, got)
				}
				if right.Type().String(nil) != types.JavaInteger || left.Type().String(nil) != target {
					t.Fatal("consumer rendering changed the producer or binding type")
				}
			}
		}
	}
}
