package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func TestBooleanNumericConsumersKeepFullIntegerOperand(t *testing.T) {
	ctx := &class_context.ClassContext{}
	for _, op := range []string{ADD, SUB, MUL, DIV, REM, SHL, SHR, USHR, LT, LTE, GT, GTE, EQ, NEQ} {
		for _, reverse := range []bool{false, true} {
			flag := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
			flag.Id.SetName("flag")
			word := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			word.IsParam = true
			word.Id.SetName("word")
			a, b := JavaValue(flag), JavaValue(word)
			if reverse {
				a, b = b, a
			}
			expr := NewBinaryExpression(a, b, op, types.NewJavaPrimer(types.JavaInteger))
			s := expr.String(ctx)
			if strings.Count(s, "? 1 : 0") != 1 || strings.Count(s, "word") != 1 || strings.Contains(s, "word) != (0)") {
				t.Fatalf("%s reverse%v: %s", op, reverse, s)
			}
			if flag.Type().String(ctx) != "boolean" || word.Type().String(ctx) != "int" {
				t.Fatal("mutated operand ABI")
			}
		}
	}
}
func TestBooleanNumericUnaryNegationHasIntegerResult(t *testing.T) {
	ctx := &class_context.ClassContext{}
	flag := NewJavaLiteral(1, types.NewJavaPrimer(types.JavaBoolean))
	e := NewUnaryExpression(flag, SUB, flag.Type())
	if e.Type().String(ctx) != "int" || strings.Count(e.String(ctx), "? 1 : 0") != 1 || flag.Type().String(ctx) != "boolean" {
		t.Fatal(e.String(ctx))
	}
}
func TestNumericBooleanViewKeepsSingleEffectCapture(t *testing.T) {
	ctx := &class_context.ClassContext{}
	value := NewCustomValue(func(*class_context.ClassContext) string { return "readOnce()" }, func() types.JavaType { return types.NewJavaPrimer(types.JavaBoolean) })
	value.CapturesKnown = true
	e := NewBinaryExpression(value, NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger)), ADD, types.NewJavaPrimer(types.JavaInteger))
	if strings.Count(e.String(ctx), "readOnce()") != 1 || strings.Count(e.String(ctx), "? 1 : 0") != 1 {
		t.Fatal(e.String(ctx))
	}
	if e.Values[0] != value {
		t.Fatal("replaced original effect producer")
	}
}

func TestBooleanIntegerComparisonPreservesEffectOrder(t *testing.T) {
	ctx := &class_context.ClassContext{}
	makeValue := func(name, kind string) JavaValue {
		v := NewCustomValue(func(*class_context.ClassContext) string { return name + "()" }, func() types.JavaType { return types.NewJavaPrimer(kind) })
		v.CapturesKnown = true
		return v
	}
	for _, op := range []string{EQ, NEQ} {
		for _, reverse := range []bool{false, true} {
			first, second := makeValue("flagEffect", types.JavaBoolean), makeValue("wordEffect", types.JavaInteger)
			if reverse {
				first, second = second, first
			}
			text := NewBinaryExpression(first, second, op, types.NewJavaPrimer(types.JavaBoolean)).String(ctx)
			if strings.Count(text, "flagEffect()") != 1 || strings.Count(text, "wordEffect()") != 1 || strings.Index(text, first.String(ctx)) > strings.Index(text, second.String(ctx)) {
				t.Fatal(text)
			}
		}
	}
}
