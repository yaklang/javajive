package statements

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func ternaryCastTestRef(name, typeName string) *values.JavaRef {
	id := utils.NewRootVariableId()
	id.SetName(name)
	return values.NewJavaRef(id, nil, types.NewJavaClass(typeName))
}

func TestTernaryArmCastOnlyPinsTheIncompatibleArm(t *testing.T) {
	ctx := &class_context.ClassContext{}
	condition := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
	listTarget := ternaryCastTestRef("target", "java.util.List")
	listArm := ternaryCastTestRef("listValue", "java.util.List")
	mapArm := ternaryCastTestRef("mapValue", "java.util.Map")
	assignment := NewAssignStatement(listTarget, values.NewTernaryExpression(condition, listArm, mapArm), false)

	t.Setenv("JDEC_TERNARY_ARM_CAST_OFF", "")
	got := assignment.String(ctx)
	if !strings.Contains(got, "listValue") || !strings.Contains(got, "(List)(mapValue)") {
		t.Fatalf("fix must cast only the sibling Map arm to the List target: %q", got)
	}
	if strings.Contains(got, "(List)(listValue)") {
		t.Fatalf("compatible List arm must remain uncast: %q", got)
	}

	t.Setenv("JDEC_TERNARY_ARM_CAST_OFF", "1")
	off := assignment.String(ctx)
	if strings.Contains(off, "(List)(mapValue)") {
		t.Fatalf("kill switch must restore the uncast Map arm: %q", off)
	}
	if !strings.Contains(off, "? (listValue) : (mapValue)") {
		t.Fatalf("kill switch should preserve the original conditional shape: %q", off)
	}

	// Casting both arms would conceal a real type conflict rather than repair
	// the one-sided JVM merge this rule is designed for.
	bothIncompatible := NewAssignStatement(
		ternaryCastTestRef("stringTarget", "java.lang.String"),
		values.NewTernaryExpression(condition, listArm, mapArm), false,
	)
	t.Setenv("JDEC_TERNARY_ARM_CAST_OFF", "")
	if got := bothIncompatible.String(ctx); strings.Contains(got, "(String)(") {
		t.Fatalf("two incompatible arms must not receive speculative casts: %q", got)
	}
}
