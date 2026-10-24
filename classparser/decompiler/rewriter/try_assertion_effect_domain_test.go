package rewriter

import (
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Assertion projection must retain all conditional effects, including the
// allocation which happens before message evaluation and the terminal throw.
// A single covered print leaf cannot substitute for this complete domain.
func TestStructuredAssertionRequiresEveryOriginalExceptionDomain(t *testing.T) {
	for _, missingPC := range []int{-1, 10, 12, 15, 20, 23} {
		t.Run(fmt.Sprintf("excluded=%d", missingPC), func(t *testing.T) {
			boolType := types.NewJavaPrimer(types.JavaBoolean)
			primary := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
			call := func(pc int, returnType types.JavaType) *values.FunctionCallExpression {
				return &values.FunctionCallExpression{ClassName: "example.Effects", FunctionName: "effect", Descriptor: "()Z", Kind: values.InvokeStatic, IsStatic: true, OriginPC: pc, HasOriginPC: true, FuncType: types.NewJavaFuncType("()Z", nil, returnType)}
			}
			allocation := &values.NewExpression{JavaType: types.NewJavaClass("java.lang.AssertionError"), OriginPC: 10, HasOriginPC: true}
			constructor := &values.FunctionCallExpression{Object: allocation, ClassName: "java.lang.AssertionError", FunctionName: "<init>", Descriptor: "(Ljava/lang/Object;)V", Kind: values.InvokeSpecial, OriginPC: 20, HasOriginPC: true, Arguments: []values.JavaValue{call(15, primary.Type())}}
			allocation.ConstructorCall = constructor
			assertion, ok := statements.NewSourceAssertionStatement(call(12, boolType), constructor, 23)
			if !ok {
				t.Fatal("physical assertion fixture")
			}
			body := []statements.Statement{assertion}
			covered := func(pc int) bool { return pc >= 10 && pc <= 23 && pc != missingPC }
			want := missingPC == -1
			layer := handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}}
			if got := layer.block(body, covered, 0); got != want {
				t.Fatalf("handler domain=%v want=%v", got, want)
			}
			cleanup := finallyProof{remaining: 512}
			_, exits, got := cleanup.block(body, false, covered)
			if got != want || exits {
				t.Fatalf("finally domain=%v exits=%v", got, exits)
			}
			if resourceWritesLocal(body, primary) {
				t.Fatal("known conditional reads misclassified as primary definition")
			}
			constructor.Descriptor = "()V"
			if !resourceWritesLocal(body, primary) {
				t.Fatal("invalidated assertion packet gained write-closure permission")
			}
			layer = handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}}
			if layer.block(body, func(int) bool { return true }, 0) {
				t.Fatal("invalidated assertion packet gained domain permission")
			}
		})
	}
}

func TestResourcePrimaryWriteProofVisitsEveryConditionalOperand(t *testing.T) {
	primary := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
	boolValue := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
	for _, arm := range []string{"reads", "condition", "true", "false", "nested", "missing", "typed nil", "unknown"} {
		t.Run(arm, func(t *testing.T) {
			conditional := &values.TernaryExpression{Condition: boolValue, TrueValue: primary, FalseValue: values.JavaNull}
			write := values.NewAssignmentExpression(primary, values.JavaNull, 15, nil)
			switch arm {
			case "condition":
				conditional.Condition = write
			case "true":
				conditional.TrueValue = write
			case "false":
				conditional.FalseValue = write
			case "nested":
				conditional.FalseValue = &values.TernaryExpression{Condition: boolValue, TrueValue: primary, FalseValue: write}
			case "missing":
				conditional.FalseValue = nil
			case "typed nil":
				conditional.FalseValue = (*values.TernaryExpression)(nil)
			case "unknown":
				conditional.FalseValue = &values.CustomValue{CapturesKnown: false}
			}
			if got := resourceWritesValue(conditional, primary); got != (arm != "reads") {
				t.Fatalf("hidden-write/unknown=%v", got)
			}
		})
	}
}
