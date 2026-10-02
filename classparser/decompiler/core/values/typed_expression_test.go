package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func TestDescriptorBindingNamesRetainRawHeadAndArrayRank(t *testing.T) {
	ctx := &class_context.ClassContext{PackageName: "example", ClassName: "example.Caller"}
	value := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	value.Id.SetName("value")
	for rank := 0; rank <= 3; rank++ {
		target := types.NewJavaClass("example.Container")
		for i := 0; i < rank; i++ {
			target = types.NewJavaArrayType(target)
		}
		cast := &CastExpression{Value: value, TargetType: target, Binding: true, OriginPC: 17}
		want := "((example.Container" + strings.Repeat("[]", rank) + ")(value))"
		if got := cast.String(ctx); got != want || cast.Value != value || cast.OriginPC != 17 || target.ArrayDim() != rank {
			t.Fatalf("rank%d: %s", rank, got)
		}
	}
	pinned := types.NewParameterizedType("example.Container", []types.JavaType{types.NewJavaClass("java.lang.String")})
	cast := &CastExpression{Value: value, TargetType: pinned, Binding: true}
	if got := cast.String(ctx); !strings.Contains(got, "Container<String>") {
		t.Fatalf("erased an explicit parameterized binding: %s", got)
	}
	ordinary := &CastExpression{Value: value, TargetType: types.NewJavaClass("example.Container"), OriginPC: 17}
	if got := ordinary.String(ctx); got != "((Container)(value))" {
		t.Fatal(got)
	}
}

func TestPrimitiveConversionPreservesBooleanIntEncoding(t *testing.T) {
	for _, target := range []string{types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble, types.JavaByte, types.JavaShort, types.JavaChar} {
		source := NewCustomValue(func(*class_context.ClassContext) string { return "effect()" }, func() types.JavaType { return types.NewJavaPrimer(types.JavaBoolean) })
		got := RenderPrimitiveConversion(source, types.NewJavaPrimer(target), &class_context.ClassContext{})
		if strings.Count(got, "effect()") != 1 || !strings.Contains(got, "? (1) : (0)") {
			t.Fatalf("%s: repeated operand or wrong encoding: %s", target, got)
		}
	}
	source := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
	target := types.NewJavaPrimer(types.JavaDouble)
	if got := RenderPrimitiveConversion(source, target, &class_context.ClassContext{}); strings.Contains(got, "?") {
		t.Fatalf("arbitrary int treated as boolean: %s", got)
	}
	source.ResetVarType(types.NewJavaPrimer(types.JavaBoolean))
	if got := RenderPrimitiveConversion(source, target, &class_context.ClassContext{}); !strings.Contains(got, "? (1) : (0)") {
		t.Fatalf("ignored late solved operand type: %s", got)
	}
	if source.Type().RawType().(*types.JavaPrimer).Name != types.JavaBoolean {
		t.Fatal("changed shared operand type")
	}
}
