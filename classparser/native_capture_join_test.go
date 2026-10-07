package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeCaptureJoinRequiresSingleInitializationOnEveryReachingPath(t *testing.T) {
	for _, mode := range []string{"both branches", "nested join", "early return", "early throw", "blank then assignment", "one uninitialized predecessor", "same path twice", "write after capture", "parameter write", "declaration in one branch", "duplicate declaration", "self initialization", "folded write", "increment", "loop write", "caught assignment", "unknown transfer", "opaque annotated throw", "opaque value", "value cycle", "statement cycle", "foreign capture ID", "unseen allocation", "copied allocation", "missing NEW origin", "missing invoke origin", "nil ID", "current work", "current allocation", "current depth", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			typ := types.NewJavaPrimer(types.JavaLong)
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			declaration := &statements.AssignStatement{LeftValue: ref, IsDeclare: true}
			one := values.NewJavaLiteral(int64(1), typ)
			two := values.NewJavaLiteral(int64(2), typ)
			write := func(v values.JavaValue) *statements.AssignStatement {
				return statements.NewAssignStatement(ref, v, false)
			}
			condition := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
			join := &statements.IfStatement{Condition: condition, IfBody: []statements.Statement{write(one)}, ElseBody: []statements.Statement{write(two)}}
			call := &values.FunctionCallExpression{ClassName: "JoinProof$1", FunctionName: "<init>", Descriptor: "(J)V", Arguments: []values.JavaValue{ref}, OriginPC: 19, HasOriginPC: true}
			allocation := &values.NewExpression{JavaType: types.NewJavaClass("JoinProof$1"), ConstructorCall: call, OriginPC: 13, HasOriginPC: true}
			query := allocation
			body := []statements.Statement{declaration, join, statements.NewReturnStatement(allocation)}
			var work *workbudget.Budget
			good := mode == "both branches" || mode == "nested join" || mode == "early return" || mode == "early throw" || mode == "blank then assignment"
			switch mode {
			case "nested join":
				join.ElseBody = []statements.Statement{&statements.IfStatement{Condition: condition, IfBody: []statements.Statement{write(one)}, ElseBody: []statements.Statement{write(two)}}}
			case "early return":
				join.ElseBody = []statements.Statement{statements.NewReturnStatement(values.NewJavaLiteral(nil, types.NewJavaClass("java.lang.Object")))}
			case "early throw":
				join.ElseBody = []statements.Statement{statements.NewThrowStatement(values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.RuntimeException")))}
			case "blank then assignment":
				body[1] = write(one)
			case "one uninitialized predecessor":
				join.ElseBody = nil
			case "same path twice":
				join.IfBody = append(join.IfBody, write(two))
			case "write after capture":
				body = append(body, write(two))
			case "parameter write":
				body = body[1:]
				ref.IsParam = true
			case "declaration in one branch":
				body = body[1:]
				join.IfBody = append([]statements.Statement{declaration}, join.IfBody...)
			case "duplicate declaration":
				body = append([]statements.Statement{&statements.AssignStatement{LeftValue: ref, IsDeclare: true}}, body...)
			case "self initialization":
				join.IfBody = []statements.Statement{write(allocation)}
			case "folded write":
				body = append(body, &statements.ExpressionStatement{Expression: &values.AssignmentExpression{Target: ref, Value: two}})
			case "increment":
				body = append(body, &statements.ExpressionStatement{Expression: values.NewUnaryExpression(ref, values.INC, typ)})
			case "loop write":
				body[1] = &statements.WhileStatement{ConditionValue: condition, Body: []statements.Statement{write(one)}}
			case "caught assignment":
				body[1] = &statements.TryCatchStatement{TryBody: []statements.Statement{write(one)}, CatchBodies: [][]statements.Statement{{write(two)}}}
			case "unknown transfer":
				join.ElseBody = []statements.Statement{statements.NewSourceTransferStatement("break", "OUTER")}
			case "opaque annotated throw":
				custom := statements.NewCustomStatement(func(*class_context.ClassContext) string { return "arbitrary side effects" }, nil)
				custom.ThrownValue = values.JavaNull
				join.ElseBody = []statements.Statement{custom}
			case "opaque value":
				join.Condition = values.NewCustomValue(func(*class_context.ClassContext) string { return "true" }, func() types.JavaType { return types.NewJavaPrimer(types.JavaBoolean) })
			case "value cycle":
				cycle := &values.JavaExpression{}
				cycle.Values = []values.JavaValue{cycle}
				join.Condition = cycle
			case "statement cycle":
				join.IfBody = []statements.Statement{join}
			case "foreign capture ID":
				call.Arguments[0] = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			case "unseen allocation":
				body[2] = statements.NewReturnStatement(one)
			case "copied allocation":
				copied := *allocation
				query = &copied
			case "missing NEW origin":
				allocation.HasOriginPC = false
			case "missing invoke origin":
				call.HasOriginPC = false
			case "nil ID":
				ref.Id = nil
			case "current work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "current allocation":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "current depth":
				work = workbudget.New(nil, workbudget.Limits{MaxASTDepth: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			proved, known := nativeCaptureJoinedDeclaration(body, ref, query, work)
			if known != good || known && proved != declaration {
				t.Fatalf("accepted=%v declaration identity=%v", known, proved == declaration)
			}
			if work != nil && work.Err() == nil {
				t.Fatal("current resource refusal was not sticky")
			}
		})
	}
}
