package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

// Type inference may retain an int producer while a canonical terminal gains
// a Boolean source type. Reduction must not silently change its numeric view.
func TestReducedDecisionKeepsComputationalTypeAtNumericConsumers(t *testing.T) {
	ctx := &class_context.ClassContext{}
	for _, reverse := range []bool{false, true} {
		one := NewJavaLiteral(1, types.NewJavaPrimer(types.JavaBoolean))
		zero := NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))
		var yes, no JavaValue = one, zero
		if reverse {
			yes, no = no, yes
		}
		root := NewTernaryExpression(decisionGate("effect"), yes, no)
		root.SetCachedType(types.NewJavaPrimer(types.JavaInteger))
		for _, op := range []string{EQ, NEQ, ADD, MUL} {
			beforeYes, beforeNo := root.TrueValue, root.FalseValue
			expression := NewBinaryExpression(root, NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)), op, root.Type())
			text := expression.String(ctx)
			if !strings.Contains(text, "? 1 : 0") || strings.Count(text, "effect()") != 1 {
				t.Fatalf("numeric %s lost canonical word/evaluation: %s", op, text)
			}
			if root.Type().String(ctx) != "int" || root.TrueValue != beforeYes || root.FalseValue != beforeNo || one.Type().String(ctx) != "boolean" || zero.Type().String(ctx) != "int" {
				t.Fatal("consumer retyped or replaced the producer graph")
			}
		}
		view, ok := BooleanStackConsumerView(root)
		if !ok || view.Type().String(ctx) != "boolean" || strings.Contains(view.String(ctx), "? 1 : 0") {
			t.Fatal("Boolean sink failed to choose its own canonical view")
		}
	}
}
