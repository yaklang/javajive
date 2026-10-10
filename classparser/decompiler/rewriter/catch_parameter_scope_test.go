package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestCatchParameterIsDefinedAtHandlerEntry(t *testing.T) {
	for _, shape := range []string{"direct", "branch", "loop", "nested catch", "sibling catch", "unrelated same name"} {
		t.Run(shape, func(t *testing.T) {
			typ := types.NewJavaClass("java.lang.Exception")
			ex := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, typ)
			originalID := ex.Id
			condition := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
			assign := statements.NewAssignStatement(ex, ex, false)
			var body []statements.Statement
			switch shape {
			case "branch", "unrelated same name":
				body = []statements.Statement{&statements.IfStatement{Condition: condition, IfBody: []statements.Statement{assign}}, statements.NewReturnStatement(ex)}
			case "loop":
				body = []statements.Statement{&statements.WhileStatement{ConditionValue: condition, Body: []statements.Statement{assign}}}
			case "nested catch":
				nested := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, typ)
				body = []statements.Statement{&statements.TryCatchStatement{Exception: []*values.JavaRef{nested}, CatchBodies: [][]statements.Statement{{assign}}}}
			default:
				body = []statements.Statement{assign}
			}
			handler := &statements.TryCatchStatement{Exception: []*values.JavaRef{ex}, CatchBodies: [][]statements.Statement{body}}
			var other *statements.AssignStatement
			if shape == "sibling catch" {
				second := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, typ)
				other = statements.NewAssignStatement(second, second, false)
				handler.Exception = append(handler.Exception, second)
				handler.CatchBodies = append(handler.CatchBodies, []statements.Statement{other})
			} else if shape == "unrelated same name" {
				local := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, typ)
				local.Id.SetName(ex.Id.String())
				other = statements.NewAssignStatement(local, values.NewJavaLiteral(nil, typ), true)
				handler.TryBody = []statements.Statement{other}
			}
			root := []statements.Statement{handler}
			RewriteVar(&root, 1, nil)
			if ex.Id != originalID || assign.LeftValue.(*values.JavaRef).Id != originalID || assign.IsFirst || assign.IsDeclare {
				t.Fatal("handler reassignment split or redeclared its entry definition")
			}
			if len(root) != 1 || root[0] != handler {
				t.Fatal("catch parameter declaration escaped its handler")
			}
			if other != nil {
				if shape == "sibling catch" && (other.IsFirst || other.IsDeclare) {
					t.Fatal("sibling handler parameter was redeclared")
				}
				if shape == "unrelated same name" && (!other.IsFirst || other.LeftValue.(*values.JavaRef).Id == originalID) {
					t.Fatal("catch definition leaked into unrelated local scope")
				}
			}
			before := assign.LeftValue.String(&class_context.ClassContext{})
			RewriteVar(&root, 1, nil)
			if assign.IsFirst || assign.LeftValue.String(&class_context.ClassContext{}) != before {
				t.Fatal("handler binding is not idempotent")
			}
		})
	}
}
