package values

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestNoncanonicalBooleanLiteralKeepsComputationalWord(t *testing.T) {
	ctx := &class_context.ClassContext{}
	for _, word := range []int{2, 3, -2, -1} {
		for _, name := range []string{types.JavaInteger, types.JavaBoolean} {
			literal := NewJavaLiteral(word, types.NewJavaPrimer(name))
			literal.Type().ResetType(types.NewJavaPrimer(types.JavaBoolean))
			if literal.Type().String(ctx) != "int" || literal.String(ctx) != fmt.Sprint(word) {
				t.Fatalf("producer word%d lost under %s: %s/%s", word, name, literal.Type().String(ctx), literal.String(ctx))
			}
			if _, ok := boolLiteralValue(literal); ok {
				t.Fatal("noncanonical literal entered Boolean reduction")
			}
			if _, ok := BoolTernaryCondition(NewTernaryExpression(NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), literal, NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)))); ok {
				t.Fatal("noncanonical ternary admitted as 1/0")
			}
			view, ok := BooleanStackConsumerView(literal)
			if !ok {
				t.Fatal("valid int consumer rejected")
			}
			want := fmt.Sprint(word&1 != 0)
			if view.String(ctx) != want {
				t.Fatalf("word%d low bit=%s want%s", word, view.String(ctx), want)
			}
			if literal.String(ctx) != fmt.Sprint(word) {
				t.Fatal("consumer mutated aliased producer")
			}
		}
	}
}

func TestBooleanSinkTernaryViewsPreserveSharedConditionWord(t *testing.T) {
	ctx := &class_context.ClassContext{}
	word := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
	word.Id.SetName("word")
	condition := NewBinaryExpression(word, NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)), NEQ, types.NewJavaPrimer(types.JavaBoolean))
	for _, constant := range []int{2, 3, -2, -1} {
		literal := NewJavaLiteral(constant, types.NewJavaPrimer(types.JavaInteger))
		tree := NewTernaryExpression(condition, word, literal)
		view, ok := BooleanStackConsumerView(tree)
		if !ok {
			t.Fatal("int arms rejected")
		}
		text := view.String(ctx)
		if !(strings.Contains(text, "(word) != (0)") || strings.Contains(text, "(word) == (0)")) || strings.Count(text, "word") != 2 || strings.Count(text, "& 1") != 1 {
			t.Fatalf("branch/return consumers conflated: %s", text)
		}
		if condition.String(ctx) != "(word) != (0)" {
			t.Fatal("consumer changed shared condition")
		}
		if word.Type().String(ctx) != "int" || literal.Type().String(ctx) != "int" || tree.TrueValue != word || tree.FalseValue != literal {
			t.Fatal("consumer copy changed shared graph")
		}
	}
	leaf := NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))
	cycle := NewTernaryExpression(condition, leaf, leaf)
	cycle.TrueValue = cycle
	if _, ok := BooleanStackConsumerView(cycle); ok {
		t.Fatal("cyclic graph accepted")
	}
	deep := JavaValue(leaf)
	for i := 0; i < 40; i++ {
		deep = NewTernaryExpression(condition, deep, leaf)
	}
	if _, ok := BooleanStackConsumerView(deep); ok {
		t.Fatal("depth budget ignored")
	}
	shared := JavaValue(word)
	for i := 0; i < 12; i++ {
		shared = NewTernaryExpression(condition, shared, shared)
	}
	if _, ok := BooleanStackConsumerView(shared); ok {
		t.Fatal("small shared DAG bypassed expanded rendering budget")
	}
	if got := NarrowBooleanStackWord(shared).String(ctx); got != EmptySlotValuePlaceholder {
		t.Fatal("rejected proof bypassed by legacy fallback: " + got)
	}
	for _, name := range []string{types.JavaLong, types.JavaFloat, types.JavaDouble, "reference"} {
		var value JavaValue
		if name == "reference" {
			value = JavaNull
		} else {
			value = NewJavaLiteral(2, types.NewJavaPrimer(name))
		}
		if _, ok := BooleanStackConsumerView(value); ok {
			t.Fatal("non-int computational category accepted: " + name)
		}
	}
}

func TestNoncanonicalBooleanLiteralOutputBudgetTracksNumericView(t *testing.T) {
	for _, word := range []int{22, -2, -2147483648} {
		literal := NewJavaLiteral(word, types.NewJavaPrimer(types.JavaBoolean))
		length := int64(len(fmt.Sprint(word)))
		assertPublicOutputLpm1(t, "numeric Boolean-typed word", length, literal.String)
	}
	single := NewJavaLiteral(2, types.NewJavaPrimer(types.JavaBoolean))
	if got := literalExactOutputBytes(single, &class_context.ClassContext{}); got != 1 {
		t.Fatal("single-digit numeric view has wrong output budget")
	}
	falseLiteral := NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean))
	assertPublicOutputLpm1(t, "canonical bool false", 5, falseLiteral.String)
}
