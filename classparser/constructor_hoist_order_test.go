package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestInstanceInitializerCannotCrossEffectOrControlFlow(t *testing.T) {
	for _, scenario := range []string{"literal prefix", "prior call", "prior branch", "effectful RHS", "other receiver", "array store", "prior parameter store", "delegation"} {
		t.Run(scenario, func(t *testing.T) {
			self := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.Owner"))
			self.IsThis = true
			field := &values.RefMember{Object: self, Member: "answer"}
			candidate := statements.NewAssignStatement(field, values.NewJavaLiteral(17, types.NewJavaPrimer(types.JavaInteger)), false)
			body := []statements.Statement{candidate}
			switch scenario {
			case "prior call":
				body = append([]statements.Statement{&statements.ExpressionStatement{Expression: &values.FunctionCallExpression{}}}, body...)
			case "prior branch":
				body = append([]statements.Statement{&statements.IfStatement{}}, body...)
			case "effectful RHS":
				candidate.JavaValue = &values.FunctionCallExpression{}
			case "other receiver":
				self.IsThis = false
			case "array store":
				candidate.ArrayMember = &values.JavaArrayMember{}
			case "prior parameter store":
				body = append([]statements.Statement{statements.NewAssignStatement(field, self, false)}, body...)
			case "delegation":
				body = append([]statements.Statement{&statements.ExpressionStatement{Expression: &values.FunctionCallExpression{ClassName: "example.Owner", FunctionName: "<init>", IsSpecialInvoke: true}}}, body...)
			}
			if got := inertConstructorFieldPrefix(body, "example.Owner")[candidate]; got != (scenario == "literal prefix") {
				t.Fatalf("hoist=%v", got)
			}
		})
	}
}
