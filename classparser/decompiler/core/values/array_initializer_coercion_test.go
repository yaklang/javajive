package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func TestArrayInitializerRestoresOnlyNarrowStoreConversions(t *testing.T) {
	ctx := &class_context.ClassContext{}
	operand := NewCustomValue(func(*class_context.ClassContext) string { return "effect()" }, func() types.JavaType { return types.NewJavaPrimer(types.JavaInteger) })
	for _, kind := range []string{types.JavaByte, types.JavaChar, types.JavaShort, types.JavaInteger, types.JavaLong, types.JavaDouble} {
		array := NewNewArrayExpression(types.NewJavaArrayType(types.NewJavaPrimer(kind)))
		array.Initializer = []JavaValue{operand}
		text := array.String(ctx)
		want := kind == types.JavaByte || kind == types.JavaChar || kind == types.JavaShort
		if strings.Contains(text, "(("+kind+")") != want || strings.Count(text, "effect()") != 1 || operand.Type().String(ctx) != "int" {
			t.Fatal(kind, text)
		}
	}
	// A two-dimensional initializer stores a row, not a primitive element.
	row := NewNewArrayExpression(types.NewJavaArrayType(types.NewJavaPrimer(types.JavaChar)))
	row.Initializer = []JavaValue{operand}
	outer := NewNewArrayExpression(types.NewJavaArrayType(row.Type()))
	outer.Initializer = []JavaValue{row}
	if text := outer.String(ctx); strings.Count(text, "((char)") != 1 || strings.Count(text, "effect()") != 1 {
		t.Fatal(text)
	}
}

func TestBooleanArrayInitializerUsesJvmLowBit(t *testing.T) {
	ctx := &class_context.ClassContext{}
	for _, n := range []int{-2, -1, 0, 1, 2, 3} {
		array := NewNewArrayExpression(types.NewJavaArrayType(types.NewJavaPrimer(types.JavaBoolean)))
		array.Initializer = []JavaValue{NewJavaLiteral(n, types.NewJavaPrimer(types.JavaInteger))}
		want := "false"
		if n&1 != 0 {
			want = "true"
		}
		if got := array.String(ctx); got != "new boolean[]{"+want+"}" {
			t.Fatal(n, got)
		}
	}
}
