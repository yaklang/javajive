package values

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func decisionGate(name string) JavaValue {
	return NewCustomValue(func(*class_context.ClassContext) string { return name + "()" }, func() types.JavaType { return types.NewJavaPrimer(types.JavaBoolean) })
}
func sharedIntegerLadder(depth, first, last int) *TernaryExpression {
	var shared JavaValue = NewJavaLiteral(first, types.NewJavaPrimer(types.JavaInteger))
	no := NewJavaLiteral(last, types.NewJavaPrimer(types.JavaInteger))
	for i := depth - 1; i >= 0; i-- {
		inner := NewTernaryExpression(decisionGate(fmt.Sprintf("d%d", i)), shared, no)
		shared = NewTernaryExpression(decisionGate(fmt.Sprintf("c%d", i)), inner, shared)
	}
	return shared.(*TernaryExpression)
}
func TestIntegerDecisionDAGCompactViewsKeepWordsAndInputs(t *testing.T) {
	for _, words := range [][2]int{{1, 0}, {2, 3}, {-2, -1}, {7, 7}} {
		for _, depth := range []int{2, 8, 20, 48} {
			t.Run(fmt.Sprintf("%d-%d-%d", depth, words[0], words[1]), func(t *testing.T) {
				root := sharedIntegerLadder(depth, words[0], words[1])
				first := root.TrueValue
				second := root.FalseValue
				text := root.String(&class_context.ClassContext{})
				if text == EmptySlotValuePlaceholder || len(text) > depth*200+200 {
					t.Fatalf("shared graph did not render linearly len%d", len(text))
				}
				for i := 0; i < depth; i++ {
					for _, name := range []string{fmt.Sprintf("c%d()", i), fmt.Sprintf("d%d()", i)} {
						if strings.Count(text, name) != 1 {
							t.Fatalf("condition duplicated/lost %s: %s", name, text)
						}
					}
				}
				if !strings.HasSuffix(text, fmt.Sprintf("? (%d) : (%d)", words[0], words[1])) {
					t.Fatal("changed original integer words:", text)
				}
				if root.TrueValue != first || root.FalseValue != second || root.Type().String(&class_context.ClassContext{}) != "int" {
					t.Fatal("retyped/mutated original shared integer graph")
				}
			})
		}
	}
}
func TestIntegerDecisionDAGProofRefusesOtherLeavesAndCycles(t *testing.T) {
	ctx := &class_context.ClassContext{}
	for _, name := range []string{"thirdWord", "intVariable", "nonBooleanCondition", "cycle", "depth", "opaqueArm", "wideWord", "boolWord"} {
		t.Run(name, func(t *testing.T) {
			root := sharedIntegerLadder(2, 2, 3)
			switch name {
			case "thirdWord":
				root.FalseValue = NewJavaLiteral(4, types.NewJavaPrimer(types.JavaInteger))
			case "intVariable":
				root.FalseValue = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			case "nonBooleanCondition":
				root.Condition = NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))
			case "cycle":
				root.TrueValue = root
				root.SetCachedType(types.NewJavaPrimer(types.JavaInteger))
			case "depth":
				root = sharedIntegerLadder(100, 2, 3)
			case "opaqueArm":
				root.FalseValue = NewCustomValue(func(*class_context.ClassContext) string { return "word()" }, func() types.JavaType { return types.NewJavaPrimer(types.JavaInteger) })
			case "wideWord":
				root.FalseValue = NewJavaLiteral(int(1<<40), types.NewJavaPrimer(types.JavaInteger))
			case "boolWord":
				root.FalseValue = NewJavaLiteral(1, types.NewJavaPrimer(types.JavaBoolean))
				root.SetCachedType(types.NewJavaPrimer(types.JavaInteger))
			}
			if _, _, _, ok := integerDecisionView(root, ctx); ok {
				t.Fatal("unproved two-integer decision accepted")
			}
			if name == "cycle" || name == "depth" {
				if boundedDecisionSource(root, 65536) {
					t.Fatal("unbounded/cyclic expansion accepted")
				}
			}
		})
	}
}
func TestBooleanSharedLeafFactoringKeepsEffectfulConditionFirst(t *testing.T) {
	b := types.NewJavaPrimer(types.JavaBoolean)
	c, s, a := decisionGate("c"), decisionGate("s"), decisionGate("a")
	root := NewTernaryExpression(c, NewBinaryExpression(s, a, LOGICAL_OR, b), s)
	text := root.String(&class_context.ClassContext{})
	if strings.Index(text, "c()") > strings.Index(text, "s()") {
		t.Fatal("effectful shared leaf moved before condition:", text)
	}
	if !strings.Contains(text, "?") {
		t.Fatal("effectful leading shared leaf illegally factored:", text)
	}
	safe := NewTernaryExpression(c, NewBinaryExpression(a, s, LOGICAL_AND, b), s).String(&class_context.ClassContext{})
	if strings.Count(safe, "s()") != 1 || strings.Index(safe, "c()") > strings.Index(safe, "a()") || strings.Index(safe, "a()") > strings.Index(safe, "s()") {
		t.Fatal("trailing shared leaf failed ordered factoring:", safe)
	}
	same := NewTernaryExpression(c, s, s).String(&class_context.ClassContext{})
	if strings.Count(same, "c()") != 1 || strings.Count(same, "s()") != 1 || strings.Index(same, "c()") > strings.Index(same, "s()") {
		t.Fatal("identical shared arms lost condition/effects:", same)
	}
}

func TestIntegerDecisionCanonicalBooleanViewsAreConsumerLocal(t *testing.T) {
	ctx := &class_context.ClassContext{}
	root := sharedIntegerLadder(48, 1, 0)
	view, ok := BooleanStackConsumerView(root)
	if !ok || view.Type().String(ctx) != "boolean" {
		t.Fatal("canonical shared word sink not compacted")
	}
	text := view.String(ctx)
	if strings.Count(text, "c0()") != 1 || len(text) > 10000 || root.Type().String(ctx) != "int" {
		t.Fatal("sink changed producer or expanded", len(text))
	}
	root.SetCachedType(types.NewJavaPrimer(types.JavaBoolean))
	text = root.String(ctx)
	if strings.HasSuffix(text, "? (1) : (0)") || text == EmptySlotValuePlaceholder {
		t.Fatal("cached Z view emitted int materialization")
	}
	noncanonical := sharedIntegerLadder(2, 2, 3)
	if _, ok := canonicalIntegerDecisionCondition(noncanonical); ok {
		t.Fatal("noncanonical int words globally normalized to Boolean")
	}
	if noncanonical.Type().String(ctx) != "int" {
		t.Fatal("mutated noncanonical producer")
	}
}

func TestIntegerDecisionBoundsExpandedFormulaAndIncompleteGraph(t *testing.T) {
	ctx := &class_context.ClassContext{}
	var shared JavaValue = NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
	for i := 0; i < 24; i++ {
		left := NewTernaryExpression(decisionGate(fmt.Sprintf("left%d", i)), shared, NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)))
		right := NewTernaryExpression(decisionGate(fmt.Sprintf("right%d", i)), NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)), shared)
		shared = NewTernaryExpression(decisionGate(fmt.Sprintf("select%d", i)), left, right)
	}
	if !boundedDecisionGraph(shared, 8192, 256) || boundedDecisionSource(shared, 65536) {
		t.Fatal("unique-node proof conflated with expanded-formula bound")
	}
	if got := shared.String(ctx); got != EmptySlotValuePlaceholder {
		t.Fatalf("irreducible shared graph expanded instead of refusing: %d bytes", len(got))
	}
	for _, missing := range []string{"condition", "true", "false"} {
		root := sharedIntegerLadder(2, 1, 0)
		switch missing {
		case "condition":
			root.Condition = nil
		case "true":
			root.TrueValue = nil
		case "false":
			root.FalseValue = nil
		}
		if got := root.String(ctx); got != EmptySlotValuePlaceholder {
			t.Fatalf("incomplete %s decision rendered", missing)
		}
	}
	slot := NewSlotValue(nil, types.NewJavaPrimer(types.JavaInteger))
	slot.val = slot
	if _, ok := BooleanStackConsumerView(slot); ok {
		t.Fatal("consumer accepted cyclic slot before graph validation")
	}
}
