package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestConstructorArrayThrowDependencyViewRequiresCompleteOperand(t *testing.T) {
	temp := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(types.NewJavaClass("java.lang.Object")))
	other := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.RuntimeException"))
	readingTemp := &values.FunctionCallExpression{FunctionName: "failureFor", Arguments: []values.JavaValue{temp}}
	opaque := statements.NewCustomStatement(func(*class_context.ClassContext) string { return "throw failureFor(hiddenTemp)" }, nil)
	opaque.ThrownValue = other
	for _, tc := range []struct {
		name            string
		statement       statements.Statement
		known, mentions bool
	}{
		{"unrelated throw", statements.NewThrowStatement(other), true, false},
		{"throw null", statements.NewThrowStatement(values.JavaNull), true, false},
		{"direct alias", statements.NewThrowStatement(temp), true, true},
		{"nested alias", statements.NewThrowStatement(readingTemp), true, true},
		{"opaque annotation", opaque, false, false},
		{"missing operand", statements.NewThrowStatement(nil), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			operands, known := constructorInlineNodeValues(tc.statement)
			if known != tc.known {
				t.Fatalf("complete=%t want=%t", known, tc.known)
			}
			if known && (len(operands) != 1 || valueMentionsLocal(operands[0], temp) != tc.mentions) {
				t.Fatal("throw dependency was dropped or invented")
			}
		})
	}
}
