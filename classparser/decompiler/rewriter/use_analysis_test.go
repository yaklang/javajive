package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func useTestRef(name, typeName string) *values.JavaRef {
	id := utils.NewRootVariableId()
	id.SetName(name)
	ref := values.NewJavaRef(id, values.JavaNull, types.NewJavaClass(typeName))
	return ref
}

func useTestString(value string) *values.JavaLiteral {
	return values.NewJavaLiteral(value, types.NewJavaClass("java.lang.String"))
}

func useTestStaticCall(ref values.JavaValue, formal types.JavaType) *values.FunctionCallExpression {
	return &values.FunctionCallExpression{
		IsStatic:     true,
		FunctionName: "consume",
		ClassName:    "example.Owner",
		Arguments:    []values.JavaValue{ref},
		FuncType: types.NewJavaFuncType("", []types.JavaType{formal},
			types.NewJavaPrimer(types.JavaVoid)),
	}
}

func useTestContext(returnType types.JavaType) *class_context.ClassContext {
	return &class_context.ClassContext{
		FunctionType: types.NewJavaFuncType("", nil, returnType),
	}
}

func TestAnalyzeWidenSafetyAcceptsOnlyStructuralSafeUses(t *testing.T) {
	objectType := types.NewJavaClass("java.lang.Object")
	stringType := types.NewJavaClass("java.lang.String")
	boolType := types.NewJavaPrimer(types.JavaBoolean)
	ref := useTestRef("var7", "java.lang.String")
	trueLiteral := values.NewJavaLiteral(true, boolType)
	nullLiteral := values.NewJavaLiteral("null", objectType)

	safe := []statements.Statement{
		statements.NewAssignStatement(ref, useTestString("x"), true),
		statements.NewExpressionStatement(values.NewOriginalInstanceOf(ref, stringType, 1)),
		statements.NewExpressionStatement(&values.CastExpression{Value: ref, TargetType: stringType}),
		statements.NewExpressionStatement(useTestStaticCall(ref, objectType)),
		&statements.ReturnStatement{JavaValue: ref},
		statements.NewSynchronizedStatement(ref, nil),
		statements.NewExpressionStatement(values.NewBinaryExpression(ref, nullLiteral, values.EQ, boolType)),
		statements.NewExpressionStatement(values.NewBinaryExpression(trueLiteral, trueLiteral, values.AND, boolType)),
	}
	safety := analyzeWidenSafety(safe, ref, useTestContext(objectType))
	if !safety.object || !safety.number {
		t.Fatalf("safe uses rejected: %+v", safety)
	}

	unsafe := []struct {
		name string
		st   statements.Statement
	}{
		{"specific call argument", statements.NewExpressionStatement(useTestStaticCall(ref, stringType))},
		{"ternary arm", statements.NewExpressionStatement(&values.TernaryExpression{
			Condition: trueLiteral, TrueValue: ref, FalseValue: useTestString("y"),
		})},
		{"numeric arithmetic", statements.NewExpressionStatement(values.NewBinaryExpression(
			ref, values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)), values.MUL,
			types.NewJavaPrimer(types.JavaInteger)))},
		{"array length", statements.NewExpressionStatement(&values.ArrayLengthExpression{Array: ref})},
		{"field receiver", statements.NewExpressionStatement(&values.RefMember{
			Object: ref, Member: "value", JavaType: types.NewJavaClass("java.lang.String"),
		})},
	}
	for _, tc := range unsafe {
		t.Run(tc.name, func(t *testing.T) {
			got := analyzeWidenSafety([]statements.Statement{tc.st}, ref, useTestContext(objectType))
			if got.object || got.number {
				t.Fatalf("unsafe use accepted: %+v", got)
			}
		})
	}
}

func TestAnalyzeWidenSafetyRejectsLambdaCapture(t *testing.T) {
	ref := useTestRef("var8", "java.lang.String")
	lambda := values.NewCustomValue(func(*class_context.ClassContext) string {
		return "(l0) -> l0"
	}, func() types.JavaType { return types.NewJavaClass("java.lang.Object") })
	lambda.Flag = "lambda"
	lambda.CapturesKnown = true
	lambda.Captures = []values.JavaValue{ref}
	safety := analyzeWidenSafety(
		[]statements.Statement{statements.NewExpressionStatement(lambda)},
		ref, useTestContext(types.NewJavaClass("java.lang.Object")),
	)
	if safety.object || safety.number {
		t.Fatalf("lambda capture accepted: %+v", safety)
	}
}

func TestCollectInstanceOfRefsFromNestedStatements(t *testing.T) {
	ref := useTestRef("var9", "java.lang.String")
	instanceOf := values.NewOriginalInstanceOf(ref, types.NewJavaClass("java.lang.String"), 3)
	nested := &statements.SwitchStatement{
		Value: values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)),
		Cases: []*statements.CaseItem{{
			IntValue: 0,
			Body: []statements.Statement{
				&statements.TryCatchStatement{
					TryBody: []statements.Statement{statements.NewExpressionStatement(instanceOf)},
				},
			},
		}},
	}
	refs := collectInstanceOfRefs([]statements.Statement{nested})
	if refs[ref.VarUid] != ref {
		t.Fatalf("nested instanceof operand not collected: %#v", refs)
	}
}

func TestWidenNumericMixedStructuredUsesStructuralSafety(t *testing.T) {
	objectType := types.NewJavaClass("java.lang.Object")
	numberType := types.NewJavaClass("java.lang.Number")
	integerType := types.NewJavaClass("java.lang.Integer")
	longType := types.NewJavaClass("java.lang.Long")

	t.Run("number-safe uses widen", func(t *testing.T) {
		ref := useTestRef("var10", "java.lang.Integer")
		decl := statements.NewAssignStatement(ref, values.NewJavaLiteral(1, integerType), true)
		reassign := statements.NewAssignStatement(ref, values.NewJavaLiteral(int64(2), longType), false)
		read := statements.NewExpressionStatement(&values.FunctionCallExpression{
			FunctionName: "longValue",
			ClassName:    "java.lang.Integer",
			Object:       ref,
			FuncType: types.NewJavaFuncType("", nil,
				types.NewJavaPrimer(types.JavaLong)),
		})
		stmtList := []statements.Statement{decl, reassign, read}
		if got := analyzeWidenSafety(stmtList, ref, useTestContext(objectType)); !got.number {
			t.Fatalf("Number-safe uses rejected: %+v", got)
		}
		widenNumericMixedStructured(&stmtList, useTestContext(objectType))
		fqn, ok := types.ClassFQNOf(ref.Type())
		if !ok || fqn != "java.lang.Number" {
			t.Fatalf("declaration not widened: %v", ref.Type())
		}
	})

	t.Run("incompatible use blocks widen", func(t *testing.T) {
		ref := useTestRef("var11", "java.lang.Integer")
		decl := statements.NewAssignStatement(ref, values.NewJavaLiteral(1, integerType), true)
		reassign := statements.NewAssignStatement(ref, values.NewJavaLiteral(int64(2), longType), false)
		read := statements.NewExpressionStatement(useTestStaticCall(ref, types.NewJavaClass("java.lang.String")))
		stmtList := []statements.Statement{decl, reassign, read}
		widenNumericMixedStructured(&stmtList, useTestContext(objectType))
		fqn, _ := types.ClassFQNOf(ref.Type())
		if fqn == "java.lang.Number" {
			t.Fatal("declaration widened despite an incompatible call argument")
		}
		_ = numberType
	})
}

func TestWidenInstanceofStructuredRequiresAllUsesSafe(t *testing.T) {
	objectType := types.NewJavaClass("java.lang.Object")
	stringType := types.NewJavaClass("java.lang.String")
	ctx := useTestContext(objectType)

	t.Run("safe operand widens", func(t *testing.T) {
		ref := useTestRef("var12", "java.lang.String")
		sts := []statements.Statement{
			statements.NewAssignStatement(ref, useTestString("x"), true),
			statements.NewExpressionStatement(values.NewOriginalInstanceOf(ref, stringType, 1)),
			&statements.ReturnStatement{JavaValue: ref},
		}
		widenInstanceofStructured(&sts, ctx)
		fqn, ok := types.ClassFQNOf(ref.Type())
		if !ok || fqn != "java.lang.Object" {
			t.Fatalf("safe instanceof operand was not widened: %v", ref.Type())
		}
	})

	t.Run("ternary use blocks widen", func(t *testing.T) {
		ref := useTestRef("var13", "java.lang.String")
		sts := []statements.Statement{
			statements.NewAssignStatement(ref, useTestString("x"), true),
			statements.NewExpressionStatement(values.NewOriginalInstanceOf(ref, stringType, 1)),
			statements.NewExpressionStatement(&values.TernaryExpression{
				Condition:  values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)),
				TrueValue:  ref,
				FalseValue: useTestString("y"),
			}),
		}
		widenInstanceofStructured(&sts, ctx)
		fqn, _ := types.ClassFQNOf(ref.Type())
		if fqn == "java.lang.Object" || fqn == "java.lang.Number" {
			t.Fatal("instanceof operand widened despite a ternary-arm use")
		}
	})
}
