package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNativeCaptureInitializedOuterBinderRequiresUnwrittenLoop(t *testing.T) {
	for _, kind := range []string{"while", "do", "for"} {
		for _, variant := range []string{"capture after", "capture inside", "nested", "condition read", "write before capture", "write after capture", "conditional write", "blank outer", "self initializer", "opaque body"} {
			t.Run(kind+"/"+variant, func(t *testing.T) {
				typ := types.NewJavaPrimer(types.JavaLong)
				ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
				one := values.NewJavaLiteral(int64(1), typ)
				var condition values.JavaValue = values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
				declaration := statements.NewAssignStatement(ref, one, true)
				allocation := &values.NewExpression{JavaType: types.NewJavaClass("OuterProof$1"), HasOriginPC: true, OriginPC: 10, ConstructorCall: &values.FunctionCallExpression{ClassName: "OuterProof$1", FunctionName: "<init>", Descriptor: "(J)V", Arguments: []values.JavaValue{ref}, HasOriginPC: true, OriginPC: 15}}
				capture := &statements.ExpressionStatement{Expression: allocation}
				read := &statements.ExpressionStatement{Expression: ref}
				write := func() statements.Statement { return statements.NewAssignStatement(ref, one, false) }
				inside := []statements.Statement{read}
				after := []statements.Statement{capture, statements.NewReturnStatement(ref)}
				loop := func(body []statements.Statement) statements.Statement {
					switch kind {
					case "while":
						return &statements.WhileStatement{ConditionValue: condition, Body: body}
					case "do":
						return &statements.DoWhileStatement{ConditionValue: condition, Body: body}
					default:
						return &statements.ForStatement{Condition: &statements.ConditionStatement{Condition: condition}, SubStatements: body}
					}
				}
				good := variant == "capture after" || variant == "capture inside" || variant == "nested" || variant == "condition read"
				switch variant {
				case "capture inside":
					inside = append(inside, capture)
					after = []statements.Statement{statements.NewReturnStatement(ref)}
				case "nested":
					inside = []statements.Statement{loop(inside)}
				case "condition read":
					condition = values.NewBinaryExpression(ref, one, values.EQ, condition.Type())
				case "write before capture":
					inside = append(inside, write())
				case "write after capture":
					inside = append(inside, capture, write())
					after = nil
				case "conditional write":
					inside = append(inside, &statements.IfStatement{Condition: condition, IfBody: []statements.Statement{write()}})
				case "blank outer":
					declaration = statements.NewDeclareStatement(ref)
				case "self initializer":
					declaration.JavaValue = ref
				case "opaque body":
					inside = append(inside, statements.NewCustomStatement(nil, nil))
				}
				body := append([]statements.Statement{declaration, loop(inside)}, after...)
				before := declaration.JavaValue
				beforeDeclare, beforeFirst := declaration.IsDeclare, declaration.IsFirst
				got, known := nativeCaptureJoinedDeclaration(body, ref, allocation, nil)
				if known != good || known && got != declaration {
					t.Fatalf("closed=%v declaration=%v", known, got == declaration)
				}
				if declaration.JavaValue != before || declaration.IsDeclare != beforeDeclare || declaration.IsFirst != beforeFirst {
					t.Fatal("proof mutated source declaration")
				}
			})
		}
	}
}
