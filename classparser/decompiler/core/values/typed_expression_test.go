package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

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
