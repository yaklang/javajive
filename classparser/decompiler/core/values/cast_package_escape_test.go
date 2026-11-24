package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestCastExpressionBindingEscapesIllegalPackage(t *testing.T) {
	ctx := &class_context.ClassContext{PackageName: "com.caller"}
	target := types.NewJavaClass("\u266b.\u8d23")
	cast := &CastExpression{
		Binding:    true,
		TargetType: target,
		Value:      JavaNull,
	}
	if got := cast.String(ctx); got != "((_u266b.\u8d23)(null))" {
		t.Fatalf("binding cast = %q, want ((_u266b.\u8d23)(null))", got)
	}

	arrayCast := &CastExpression{
		Binding:    true,
		TargetType: types.NewJavaArrayType(target),
		Value:      JavaNull,
	}
	if got := arrayCast.String(ctx); got != "((_u266b.\u8d23[])(null))" {
		t.Fatalf("array binding cast = %q, want ((_u266b.\u8d23[])(null))", got)
	}

	intersection := &CastExpression{
		Binding:             true,
		TargetType:          target,
		bindingIntersection: []types.JavaType{target, types.NewJavaClass("java.lang.Runnable")},
		Value:               JavaNull,
	}
	if got := intersection.String(ctx); got != "((_u266b.\u8d23 & java.lang.Runnable)(null))" {
		t.Fatalf("intersection binding cast = %q, want ((_u266b.\u8d23 & java.lang.Runnable)(null))", got)
	}
}
