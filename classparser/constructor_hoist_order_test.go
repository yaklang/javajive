package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
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

func TestAdversarialSynchronizedAbruptCompletionRespectsSwitchNormalExits(t *testing.T) {
	terminal := &statements.ReturnStatement{}
	normal := &statements.ExpressionStatement{Expression: values.JavaNull}
	for _, scenario := range []string{"return", "both arms", "fallthrough arm", "switch break", "switch no default", "switch all return", "grouped switch"} {
		t.Run(scenario, func(t *testing.T) {
			var body []statements.Statement
			switch scenario {
			case "return":
				body = []statements.Statement{terminal}
			case "both arms":
				body = []statements.Statement{&statements.IfStatement{IfBody: []statements.Statement{terminal}, ElseBody: []statements.Statement{terminal}}}
			case "fallthrough arm":
				body = []statements.Statement{&statements.IfStatement{IfBody: []statements.Statement{terminal}, ElseBody: []statements.Statement{normal}}}
			default:
				sw := &statements.SwitchStatement{Cases: []*statements.CaseItem{{Body: []statements.Statement{terminal}}, {IsDefault: true, Body: []statements.Statement{terminal}}}}
				if scenario == "switch break" {
					sw.Cases[0].Body = []statements.Statement{&statements.CustomStatement{Name: "break", StringFunc: func(*class_context.ClassContext) string { return "break" }}}
				}
				if scenario == "switch no default" {
					sw.Cases[1].IsDefault = false
				}
				if scenario == "grouped switch" {
					sw.Cases[0].Body = nil
				}
				body = []statements.Statement{sw}
			}
			want := scenario == "return" || scenario == "both arms" || scenario == "switch all return" || scenario == "grouped switch"
			sync := &statements.SynchronizedStatement{Body: body}
			if got := isUnconditionalTerminalStatement(sync, nil); got != want {
				t.Fatalf("terminal=%v want=%v", got, want)
			}
		})
	}
}
