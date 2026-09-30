package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func enumLambdaFixture() (*Decompiler, []statements.Statement, *values.CustomValue, *values.NewExpression, *values.JavaRef) {
	d := NewDecompiler(nil, nil)
	d.FunctionContext = &class_context.ClassContext{ClassName: "example.Choice", FunctionName: "<clinit>"}
	fi := types.NewJavaClass("example.Step")
	lambda := &values.CustomValue{Flag: "lambda", CapturesKnown: true, NoOuterCapture: true, OriginPC: 10, HasOriginPC: true,
		TypeFunc: func() types.JavaType { return fi }}
	temp := values.NewJavaRef(utils.NewRootVariableId(), nil, fi)
	allocation := values.NewNewExpression(types.NewJavaClass("example.Choice"))
	allocation.OriginPC, allocation.HasOriginPC = 1, true
	allocation.ConstructorCall = &values.FunctionCallExpression{Object: allocation, ClassName: "example.Choice", FunctionName: "<init>", OriginPC: 20,
		Arguments: []values.JavaValue{values.NewJavaLiteral("FIRST", types.NewJavaClass("java.lang.String")), values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)), temp}}
	producer := &statements.AssignStatement{LeftValue: temp, JavaValue: lambda, OriginPC: 15, HasOriginPC: true}
	consumer := &statements.AssignStatement{LeftValue: &values.JavaClassMember{Name: "example.Choice", Member: "FIRST"}, JavaValue: allocation, OriginPC: 25, HasOriginPC: true}
	return d, []statements.Statement{producer, consumer}, lambda, allocation, temp
}

func TestEnumLambdaArgumentMotionRequiresIdentityOrderAndHandlerProof(t *testing.T) {
	for _, scenario := range []string{"valid", "later use", "duplicate argument", "nested use", "opaque use", "earlier effect", "captures", "unknown captures", "other field owner", "other constructor owner", "other receiver", "not constant", "wrong name", "wrong ordinal", "missing origin", "allocation after lambda", "lambda after store", "store after invoke", "lambda handler", "allocation handler", "consumer handler", "intervening statement", "other method", "same name different identity"} {
		t.Run(scenario, func(t *testing.T) {
			d, root, lambda, allocation, temp := enumLambdaFixture()
			call := allocation.ConstructorCall
			constants := map[string]int{"FIRST": 0}
			switch scenario {
			case "later use":
				root = append(root, &statements.ReturnStatement{JavaValue: temp})
			case "duplicate argument":
				call.Arguments = append(call.Arguments, temp)
			case "nested use":
				call.Arguments = append(call.Arguments, &values.CastExpression{Value: temp, TargetType: temp.Type()})
			case "opaque use":
				root = append(root, &statements.ExpressionStatement{Expression: &values.CustomValue{}})
			case "earlier effect":
				call.Arguments = append(call.Arguments[:2], &values.FunctionCallExpression{}, temp)
			case "captures":
				lambda.Captures = []values.JavaValue{temp}
			case "unknown captures":
				lambda.CapturesKnown = false
			case "other field owner":
				root[1].(*statements.AssignStatement).LeftValue.(*values.JavaClassMember).Name = "example.Other"
			case "other constructor owner":
				call.ClassName = "example.Other"
			case "other receiver":
				call.Object = values.NewNewExpression(allocation.Type())
			case "not constant":
				constants = map[string]int{"SECOND": 0}
			case "wrong name":
				call.Arguments[0] = values.NewJavaLiteral("SECOND", types.NewJavaClass("java.lang.String"))
			case "wrong ordinal":
				call.Arguments[1] = values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
			case "missing origin":
				lambda.HasOriginPC = false
			case "allocation after lambda":
				allocation.OriginPC = 11
			case "lambda after store":
				lambda.OriginPC = 16
			case "store after invoke":
				root[0].(*statements.AssignStatement).OriginPC = 21
			case "lambda handler":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 10, EndPc: 11}}
			case "allocation handler":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 2}}
			case "consumer handler":
				d.ExceptionTable = []*ExceptionTableEntry{{StartPc: 25, EndPc: 26}}
			case "intervening statement":
				root = []statements.Statement{root[0], &statements.ReturnStatement{}, root[1]}
			case "other method":
				d.FunctionContext.FunctionName = "make"
			case "same name different identity":
				other := values.NewJavaRef(utils.NewRootVariableId(), nil, temp.Type())
				other.Id.SetName(temp.String(d.FunctionContext))
				call.Arguments[2] = other
			}
			before := len(root)
			changed := d.InlineEnumLambdaArgumentTemps(&root, constants)
			if scenario == "valid" {
				if changed != 1 || len(root) != 1 || call.Arguments[2] != lambda {
					t.Fatal("proven argument was not restored")
				}
			} else if changed != 0 || len(root) != before {
				t.Fatal("unproven argument motion changed the tree")
			}
		})
	}
}
