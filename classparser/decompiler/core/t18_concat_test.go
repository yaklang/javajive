package core

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func t18StringType() types.JavaType {
	return types.NewJavaPrimer(types.JavaString)
}

func TestTaskT18C03(t *testing.T) {
	t.Log("T18-C03")
	n := values.NewJavaLiteral(3, types.NewJavaPrimer(types.JavaInteger))
	ff := values.NewJavaLiteral(255, types.NewJavaPrimer(types.JavaInteger))
	one := values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
	two := values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))
	ops := []struct {
		name string
		v    values.JavaValue
	}{
		{"&", values.NewBinaryExpression(n, ff, values.AND, types.NewJavaPrimer(types.JavaInteger))},
		{"|", values.NewBinaryExpression(n, one, values.OR, types.NewJavaPrimer(types.JavaInteger))},
		{"^", values.NewBinaryExpression(n, one, values.XOR, types.NewJavaPrimer(types.JavaInteger))},
		{"<<", values.NewBinaryExpression(n, two, values.SHL, types.NewJavaPrimer(types.JavaInteger))},
		{">>", values.NewBinaryExpression(n, one, values.SHR, types.NewJavaPrimer(types.JavaInteger))},
		{">>>", values.NewBinaryExpression(n, one, values.USHR, types.NewJavaPrimer(types.JavaInteger))},
		{"?:", values.NewTernaryExpression(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaBoolean)), n, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)))},
	}
	for _, op := range ops {
		if !t18ConcatArgNeedsParens(op.v) {
			t.Errorf("T18-C03 %s concat arg must need parens", op.name)
		}
		recipe := values.NewJavaLiteral("x=\u0001", t18StringType())
		req := CallSiteRequest{
			Identity:            IdentityMakeConcatWithConstants,
			CallSiteName:        "makeConcatWithConstants",
			CallSiteDescriptor:  "(I)Ljava/lang/String;",
			StaticArgs:          []values.JavaValue{recipe},
			DynamicArgs:         []values.JavaValue{op.v},
			TargetSourceVersion: 17,
			ClassMajor:          61,
		}
		res := DispatchInvokeDynamic(req, nil, nil, t18StringType())
		if res.Status != "" {
			t.Fatalf("T18-C03 %s status=%s reason=%s", op.name, res.Status, res.Reason)
		}
		src := res.Value.String(&class_context.ClassContext{})
		if !strings.Contains(src, "+") {
			t.Fatalf("T18-C03 %s lost concat: %s", op.name, src)
		}
		// Whole operand must be parenthesized so `"x=" + (n) << (2)` cannot parse.
		if !strings.Contains(src, "(") {
			t.Fatalf("T18-C03 %s missing parens: %s", op.name, src)
		}
		t.Logf("T18-C03 %s => %s", op.name, src)
	}
}

func TestTaskT18C05(t *testing.T) {
	t.Log("T18-C05")
	typ := t18StringType()
	baseRecipe := values.NewJavaLiteral("\u0001", typ)
	base := CallSiteRequest{
		Identity:            IdentityMakeConcatWithConstants,
		CallSiteName:        "makeConcatWithConstants",
		CallSiteDescriptor:  "(Ljava/lang/Object;)Ljava/lang/String;",
		StaticArgs:          []values.JavaValue{baseRecipe},
		DynamicArgs:         []values.JavaValue{values.JavaNull},
		TargetSourceVersion: 17,
		ClassMajor:          61,
	}

	extraArg := base
	extraArg.StaticArgs = []values.JavaValue{values.NewJavaLiteral("\u0001\u0001", typ)}
	res := DispatchInvokeDynamic(extraArg, nil, nil, typ)
	if res.Status != "invalid_input" {
		t.Fatalf("T18-C05 extra TAG_ARG expected invalid_input, got %+v", res)
	}
	if res.ExecutedBootstrap {
		t.Fatal("T18-C05 must not execute bootstrap")
	}

	extraConst := base
	extraConst.StaticArgs = []values.JavaValue{
		values.NewJavaLiteral("\u0001", typ),
		values.NewJavaLiteral("extra", typ),
	}
	res = DispatchInvokeDynamic(extraConst, nil, nil, typ)
	if res.Status != "invalid_input" || !strings.Contains(res.Reason, "TAG_CONST") {
		t.Fatalf("T18-C05 extra TAG_CONST: %+v", res)
	}

	missingConst := base
	missingConst.StaticArgs = []values.JavaValue{values.NewJavaLiteral("\u0002\u0001", typ)}
	res = DispatchInvokeDynamic(missingConst, nil, nil, typ)
	if res.Status != "invalid_input" {
		t.Fatalf("T18-C05 missing TAG_CONST payload: %+v", res)
	}

	trailing := base
	trailing.CallSiteDescriptor = "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/String;"
	trailing.DynamicArgs = []values.JavaValue{values.JavaNull, values.JavaNull}
	res = DispatchInvokeDynamic(trailing, nil, nil, typ)
	if res.Status != "invalid_input" {
		t.Fatalf("T18-C05 trailing dynamic args must not be dropped: %+v", res)
	}
}

func TestTaskT18RecipeDataNotPrinterReplace(t *testing.T) {
	t.Log("T18-C02")
	typ := t18StringType()
	// Recipe TAG_CONST then ':' then TAG_ARG. Constant payload contains U+0001/U+0002 as DATA.
	recipe := values.NewJavaLiteral("\u0002:\u0001", typ)
	cst := values.NewJavaLiteral("\u0001\u0002:", typ)
	dyn := values.NewJavaLiteral("Z", typ)
	req := CallSiteRequest{
		Identity:            IdentityMakeConcatWithConstants,
		CallSiteName:        "makeConcatWithConstants",
		CallSiteDescriptor:  "(Ljava/lang/String;)Ljava/lang/String;",
		StaticArgs:          []values.JavaValue{recipe, cst},
		DynamicArgs:         []values.JavaValue{dyn},
		TargetSourceVersion: 17,
		ClassMajor:          61,
	}
	res := DispatchInvokeDynamic(req, nil, nil, typ)
	if res.Status != "" {
		t.Fatalf("recipe data concat: %+v", res)
	}
	src := res.Value.String(&class_context.ClassContext{})
	has1 := strings.Contains(src, `\u0001`) || strings.Contains(src, `\001`)
	has2 := strings.Contains(src, `\u0002`) || strings.Contains(src, `\002`)
	if !has1 || !has2 {
		t.Fatalf("expected U+0001/U+0002 data preserved in source, got %s", src)
	}
	if strings.Count(src, ` + `) < 1 {
		t.Fatalf("expected structured + concat, got %s", src)
	}
	t.Logf("T18-C02 recipe render: %s", src)
}

func TestTaskT18MakeConcatEvalOrder(t *testing.T) {
	t.Log("T18-C02")
	a := values.NewJavaLiteral("A", t18StringType())
	b := values.NewJavaLiteral("B", t18StringType())
	req := CallSiteRequest{
		Identity:            IdentityMakeConcat,
		CallSiteName:        "makeConcat",
		CallSiteDescriptor:  "(Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;",
		DynamicArgs:         []values.JavaValue{b, a}, // pop order: last param first
		TargetSourceVersion: 17,
		ClassMajor:          61,
	}
	res := DispatchInvokeDynamic(req, nil, nil, t18StringType())
	if res.Status != "" {
		t.Fatalf("makeConcat: %+v", res)
	}
	src := res.Value.String(&class_context.ClassContext{})
	ai := strings.Index(src, `"A"`)
	bi := strings.Index(src, `"B"`)
	if ai < 0 || bi < 0 || ai > bi {
		t.Fatalf("makeConcat eval order want A then B, got %s", src)
	}
}

func TestTaskT18UnsnapshottedEffectfulConversionBoundary(t *testing.T) {
	typ := t18StringType()
	stringArgs := []values.JavaValue{
		values.TagEffects(values.NewJavaLiteral("R", typ), values.EffectCall),
		values.TagEffects(values.NewJavaLiteral("L", typ), values.EffectCall),
	}
	res := DispatchInvokeDynamic(CallSiteRequest{
		Identity: IdentityMakeConcat, CallSiteName: "makeConcat",
		CallSiteDescriptor: "(Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;",
		DynamicArgs:        stringArgs, TargetSourceVersion: 17, ClassMajor: 61,
	}, nil, nil, typ)
	if res.Status != "" {
		t.Fatalf("effectful String operands are safe inline: %+v", res)
	}

	objectType := types.NewJavaClass("java.lang.Object")
	objectArgs := []values.JavaValue{
		values.TagEffects(values.NewJavaLiteral("R", objectType), values.EffectCall),
		values.TagEffects(values.NewJavaLiteral("L", objectType), values.EffectCall),
	}
	res = DispatchInvokeDynamic(CallSiteRequest{
		Identity: IdentityMakeConcat, CallSiteName: "makeConcat",
		CallSiteDescriptor: "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/String;",
		DynamicArgs:        objectArgs, TargetSourceVersion: 17, ClassMajor: 61,
	}, nil, nil, typ)
	if res.Status != "unsupported" {
		t.Fatalf("effectful Object conversion requires snapshots: %+v", res)
	}
}

func TestTaskT18NullObjectCast(t *testing.T) {
	t.Log("T18-C01")
	recipe := values.NewJavaLiteral("x=\u0001,o=\u0001", t18StringType())
	x := values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger))
	req := CallSiteRequest{
		Identity:            IdentityMakeConcatWithConstants,
		CallSiteName:        "makeConcatWithConstants",
		CallSiteDescriptor:  "(ILjava/lang/Object;)Ljava/lang/String;",
		StaticArgs:          []values.JavaValue{recipe},
		DynamicArgs:         []values.JavaValue{values.JavaNull, x}, // pop: last param first
		TargetSourceVersion: 17,
		ClassMajor:          61,
	}
	res := DispatchInvokeDynamic(req, nil, nil, t18StringType())
	if res.Status != "" {
		t.Fatalf("T18-C01 unit: %+v", res)
	}
	src := res.Value.String(&class_context.ClassContext{})
	if strings.Contains(src, "String.valueOf(null)") && !strings.Contains(src, "String.valueOf((Object)null)") {
		t.Fatalf("T18-C01 must not emit uncast valueOf(null): %s", src)
	}
	if !strings.Contains(src, "(Object)null") && !strings.Contains(src, "+ null") {
		t.Fatalf("T18-C01 expected Object-null concat form: %s", src)
	}
	t.Logf("T18-C01 unit render: %s", src)
}
