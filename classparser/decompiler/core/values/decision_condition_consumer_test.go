package values

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A canonical materialization may itself use an int-valued decision as its
// condition. Extracting its condition must select a Boolean use-site view,
// without changing the shared producer or using Z's low-bit storage rule.
func TestMaterializedDecisionConditionUsesNonzeroView(t *testing.T) {
	ctx := &class_context.ClassContext{}
	for _, words := range [][2]int{{1, 0}, {2, 3}, {-2, 0}, {7, 7}} {
		inner := NewTernaryExpression(decisionGate("effect"),
			NewJavaLiteral(words[0], types.NewJavaPrimer(types.JavaInteger)),
			NewJavaLiteral(words[1], types.NewJavaPrimer(types.JavaInteger)))
		inner.SetCachedType(types.NewJavaPrimer(types.JavaInteger))
		outer := NewTernaryExpression(inner,
			NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)),
			NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)))
		condition, ok := BoolTernaryCondition(outer)
		if !ok || condition.Type().String(ctx) != "boolean" {
			t.Fatalf("integer condition escaped a Boolean consumer: %v", condition)
		}
		text := condition.String(ctx)
		if strings.Count(text, "effect()") != 1 || strings.Contains(text, "& 1") {
			t.Fatalf("wrong condition evaluation/narrowing: %s", text)
		}
		if words != [2]int{1, 0} && (!strings.Contains(text, "!=") || !strings.Contains(text, "(0)")) {
			t.Fatalf("noncanonical words lost nonzero semantics: %s", text)
		}
		if inner.Type().String(ctx) != "int" || outer.Condition != inner {
			t.Fatal("consumer mutated the producer")
		}
	}
}

func TestIntegerAssignmentDoesNotRematerializeDecisionWords(t *testing.T) {
	ctx := &class_context.ClassContext{}
	integer := types.NewJavaPrimer(types.JavaInteger)
	boolean := types.NewJavaPrimer(types.JavaBoolean)
	for _, reverse := range []bool{false, true} {
		var yes, no JavaValue = NewJavaLiteral(1, boolean), NewJavaLiteral(0, integer)
		if reverse {
			yes, no = no, yes
		}
		root := NewTernaryExpression(decisionGate("effect"), yes, no)
		root.SetCachedType(integer.Copy())
		coerced := CoerceIntAssignRHS(integer, root, ctx)
		if coerced != root || root.Type().String(ctx) != "int" {
			t.Fatal("already computational int was materialized or retyped again")
		}
		text := coerced.String(ctx)
		if strings.Count(text, "effect()") != 1 || strings.Count(text, "?") != 1 {
			t.Fatalf("integer assignment lost single materialization: %s", text)
		}
	}
	// A Boolean producer still needs a word view at an integer consumer.
	gate := decisionGate("booleanEffect")
	text := CoerceIntAssignRHS(integer, NewUnaryExpression(gate, Not, boolean), ctx).String(ctx)
	if strings.Count(text, "booleanEffect()") != 1 || !strings.Contains(text, "?") {
		t.Fatal("Boolean-to-int conversion disappeared:", text)
	}
}
