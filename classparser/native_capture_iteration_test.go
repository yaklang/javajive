package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Loop scope, definite assignment and effective finality are separate facts.
// A fresh declaration may be assigned on alternative paths in each iteration;
// an outer blank binder or a second write on one path must still be refused.
func TestNativeCaptureIterationRequiresFreshSingleAssignmentAndClosedScope(t *testing.T) {
	for _, kind := range []string{"while", "do", "for"} {
		for _, variant := range []string{"joined", "nested", "early return", "sealed continue", "sealed break", "blank outside", "initialized outside", "read before initialization", "missing predecessor", "second write", "write after capture", "read after loop", "capture twice", "opaque transfer"} {
			t.Run(kind+"/"+variant, func(t *testing.T) {
				typ := types.NewJavaPrimer(types.JavaLong)
				ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
				declaration := statements.NewDeclareStatement(ref)
				one := values.NewJavaLiteral(int64(1), typ)
				write := func() *statements.AssignStatement { return statements.NewAssignStatement(ref, one, false) }
				condition := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
				join := &statements.IfStatement{Condition: condition, IfBody: []statements.Statement{write()}, ElseBody: []statements.Statement{write()}}
				allocation := &values.NewExpression{JavaType: types.NewJavaClass("IterationProof$1"), HasOriginPC: true, OriginPC: 10,
					ConstructorCall: &values.FunctionCallExpression{ClassName: "IterationProof$1", FunctionName: "<init>", Descriptor: "(J)V", Arguments: []values.JavaValue{ref}, HasOriginPC: true, OriginPC: 15}}
				capture := &statements.ExpressionStatement{Expression: allocation}
				inside := []statements.Statement{declaration, join, capture}
				var before, after []statements.Statement
				good := variant == "joined" || variant == "nested" || variant == "early return" || variant == "sealed continue" || variant == "sealed break"
				switch variant {
				case "early return":
					join.ElseBody = []statements.Statement{statements.NewReturnStatement(nil)}
				case "sealed continue", "sealed break":
					keyword := "continue"
					if variant == "sealed break" {
						keyword = "break"
					}
					join.ElseBody = []statements.Statement{statements.NewSourceTransferStatement(keyword, "")}
				case "blank outside":
					before, inside = []statements.Statement{declaration}, inside[1:]
				case "initialized outside":
					before, inside = []statements.Statement{statements.NewAssignStatement(ref, one, true)}, inside[1:]
				case "read before initialization":
					join.Condition = values.NewBinaryExpression(ref, one, values.EQ, condition.Type())
				case "missing predecessor":
					join.ElseBody = nil
				case "second write":
					join.IfBody = append(join.IfBody, write())
				case "write after capture":
					inside = append(inside, write())
				case "read after loop":
					after = []statements.Statement{statements.NewReturnStatement(ref)}
				case "capture twice":
					inside = append(inside, capture)
				case "opaque transfer":
					join.ElseBody = []statements.Statement{statements.NewCustomStatement(nil, nil)}
				}
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
				if variant == "nested" {
					inside = []statements.Statement{loop(inside)}
				}
				body := append(append(before, loop(inside)), after...)
				proved, known := nativeCaptureJoinedDeclaration(body, ref, allocation, nil)
				if known != good || known && proved != declaration {
					t.Fatalf("closed=%v same declaration=%v", known, proved == declaration)
				}
				if declaration.JavaValue != nil || !declaration.IsDeclare {
					t.Fatal("proof mutated declaration")
				}
			})
		}
	}
}
