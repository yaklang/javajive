package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestTypedCatchConstantArmRequiresClosedBooleanProof(t *testing.T) {
	for _, scenario := range []string{"dead else", "dead if", "not", "equal", "live uncovered", "fake ref value", "numeric literal", "effectful guard", "cycle", "missing type", "budget"} {
		t.Run(scenario, func(t *testing.T) {
			b := types.NewJavaPrimer(types.JavaBoolean)
			var condition values.JavaValue = values.NewJavaLiteral(true, b)
			outside := &values.FunctionCallExpression{OriginPC: 20, HasOriginPC: true, Kind: values.InvokeStatic, IsStatic: true, Descriptor: "()V"}
			inside := &values.FunctionCallExpression{OriginPC: 2, HasOriginPC: true, Kind: values.InvokeStatic, IsStatic: true, Descriptor: "()V"}
			branch := &statements.IfStatement{Condition: condition, IfBody: []statements.Statement{&statements.ExpressionStatement{Expression: inside}}, ElseBody: []statements.Statement{&statements.ExpressionStatement{Expression: outside}}}
			switch scenario {
			case "dead if":
				condition = values.NewJavaLiteral(false, b)
				branch.IfBody, branch.ElseBody = branch.ElseBody, branch.IfBody
			case "not":
				condition = values.NewUnaryExpression(values.NewJavaLiteral(false, b), values.Not, b)
			case "equal":
				condition = values.NewBinaryExpression(values.NewJavaLiteral(false, b), values.NewJavaLiteral(false, b), values.EQ, b)
			case "live uncovered":
				branch.IfBody = branch.ElseBody
			case "fake ref value":
				condition = values.NewJavaRef(utils.NewRootVariableId(), values.NewJavaLiteral(true, b), b)
			case "numeric literal":
				condition = values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaInteger))
			case "effectful guard":
				condition = &values.FunctionCallExpression{OriginPC: 2, HasOriginPC: true, Kind: values.InvokeStatic, IsStatic: true, Descriptor: "()Z"}
			case "cycle":
				x := &values.JavaExpression{Op: values.Not}
				x.Values = []values.JavaValue{x}
				condition = x
			case "missing type":
				condition = values.NewJavaLiteral(true, nil)
			}
			branch.Condition = condition
			p := typedCatchCoverage{covered: func(pc int) bool { return pc >= 0 && pc < 10 }, remaining: 512}
			if scenario == "budget" {
				p.remaining = 1
			}
			got := p.block([]statements.Statement{branch})
			want := scenario == "dead else" || scenario == "dead if" || scenario == "not" || scenario == "equal"
			if got != want {
				t.Fatalf("covered=%v want=%v", got, want)
			}
			if branch.IfBody == nil || branch.ElseBody == nil {
				t.Fatal("deleted original arm")
			}
		})
	}
}
func TestTypedCatchPrimitiveConversionProvesCapturedEffects(t *testing.T) {
	for _, scenario := range []string{"numeric", "byte", "uncovered operand", "unknown capture", "opaque category", "boolean target", "boolean source", "reference source", "missing type function", "two captures", "nil capture", "cycle", "missing operand PC"} {
		t.Run(scenario, func(t *testing.T) {
			target := types.JavaLong
			call := &values.FunctionCallExpression{OriginPC: 2, HasOriginPC: true, Kind: values.InvokeStatic, IsStatic: true, Descriptor: "()I", FuncType: &types.JavaFuncType{ReturnType: types.NewJavaPrimer(types.JavaInteger)}}
			x := &values.CustomValue{Flag: "primitive_cast", CapturesKnown: true, Captures: []values.JavaValue{call}, TypeFunc: func() types.JavaType { return types.NewJavaPrimer(target) }}
			switch scenario {
			case "byte":
				target = types.JavaByte
			case "uncovered operand":
				call.OriginPC = 20
			case "unknown capture":
				x.CapturesKnown = false
			case "opaque category":
				x.Flag = "opaque"
			case "boolean target":
				target = types.JavaBoolean
			case "boolean source":
				call.FuncType.ReturnType = types.NewJavaPrimer(types.JavaBoolean)
			case "reference source":
				call.FuncType.ReturnType = types.NewJavaClass("java.lang.Integer")
			case "missing type function":
				x.TypeFunc = nil
			case "two captures":
				x.Captures = append(x.Captures, call)
			case "nil capture":
				x.Captures[0] = nil
			case "cycle":
				x.Captures[0] = x
			case "missing operand PC":
				call.HasOriginPC = false
			}
			p := typedCatchCoverage{covered: func(pc int) bool { return pc >= 0 && pc < 10 }, remaining: 512}
			got := p.value(x, 0)
			want := scenario == "numeric" || scenario == "byte"
			if got != want {
				t.Fatalf("covered=%v want=%v", got, want)
			}
		})
	}
}

func TestTypedCatchCoverageRejectsUnwitnessedResolutionAndCast(t *testing.T) {
	for _, scenario := range []string{"original cast", "uncovered cast", "synthetic cast", "negative PC", "class literal", "numeric comparison", "reference comparison", "missing call type"} {
		t.Run(scenario, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			c := values.NewOriginalCheckCast(ref, types.NewJavaClass("example.Value"), 2)
			var v values.JavaValue = c
			switch scenario {
			case "uncovered cast":
				c.OriginPC = 20
			case "synthetic cast":
				c.OriginalCheckCast = false
			case "negative PC":
				c.OriginPC = -1
			case "class literal":
				v = &values.JavaClassValue{JavaType: types.NewJavaClass("example.Value")}
			case "numeric comparison", "reference comparison", "missing call type":
				var left values.JavaValue = values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))
				if scenario == "reference comparison" {
					left = ref
				}
				if scenario == "missing call type" {
					left = &values.FunctionCallExpression{IsStatic: true, Kind: values.InvokeStatic, Descriptor: "()I", HasOriginPC: true, OriginPC: 2}
				}
				v = &values.JavaExpression{Values: []values.JavaValue{left, values.NewJavaLiteral(3, types.NewJavaPrimer(types.JavaInteger))}, Op: values.LT, Typ: types.NewJavaPrimer(types.JavaBoolean)}
			}
			p := typedCatchCoverage{covered: func(pc int) bool { return pc >= 0 && pc < 10 }, remaining: 512}
			got := p.value(v, 0)
			want := scenario == "original cast" || scenario == "numeric comparison"
			if got != want {
				t.Fatalf("covered=%v want=%v", got, want)
			}
		})
	}
}

func TestTypedCatchTotalNumericOperatorsKeepOperandCoverage(t *testing.T) {
	for _, op := range []string{values.ADD, values.SUB, values.MUL, values.AND, values.OR, values.XOR, values.SHL, values.SHR, values.USHR, values.DIV, values.REM} {
		for _, scenario := range []string{"covered", "uncovered", "string", "boxed"} {
			t.Run(op+"/"+scenario, func(t *testing.T) {
				typ := types.NewJavaPrimer(types.JavaInteger)
				field := values.NewRefMember(values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.Owner")), "word", typ)
				field.OriginPC = 2
				field.HasOriginPC = true
				switch scenario {
				case "uncovered":
					field.OriginPC = 20
				case "string":
					field.JavaType = types.NewJavaClass("java.lang.String")
				case "boxed":
					field.JavaType = types.NewJavaClass("java.lang.Integer")
				}
				expr := &values.JavaExpression{Op: op, Values: []values.JavaValue{field, values.NewJavaLiteral(2, typ)}, Typ: typ}
				p := typedCatchCoverage{covered: func(pc int) bool { return pc >= 0 && pc < 10 }, remaining: 512}
				want := scenario == "covered" && op != values.DIV && op != values.REM
				if got := p.value(expr, 0); got != want {
					t.Fatalf("covered=%v want=%v", got, want)
				}
			})
		}
	}
}
