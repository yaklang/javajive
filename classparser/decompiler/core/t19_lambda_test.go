package core

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func t19IntOpType() types.JavaType {
	return types.NewJavaClass("java.util.function.IntUnaryOperator")
}

func t19SAM() values.JavaValue {
	return values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
		return "(I)I"
	}, func() types.JavaType {
		return types.NewJavaClass("java.lang.invoke.MethodType")
	})
}

func t19Impl(owner, name, desc string, kind uint8) *values.JavaClassMember {
	m := values.NewJavaClassMember(owner, name, desc, types.NewJavaClass(owner))
	m.RefKind = kind
	return m
}

func TestTaskT19C04(t *testing.T) {
	t.Log("T19-C04")
	typ := t19IntOpType()
	sam := t19SAM()
	impl := t19Impl("Sample", "lambda$main$0", "(I)I", RefInvokeStatic)
	flags := func(n int) values.JavaValue {
		return values.NewJavaLiteral(n, types.NewJavaPrimer(types.JavaInteger))
	}
	marker := values.NewJavaClassValue(types.NewJavaClass("Marker"))

	serial := CallSiteRequest{
		Identity:            IdentityLambdaAltMetafactory,
		CallSiteName:        "applyAsInt",
		CallSiteDescriptor:  "()Ljava/util/function/IntUnaryOperator;",
		StaticArgs:          []values.JavaValue{sam, impl, sam, flags(lambdaFlagSerializable)},
		DynamicArgs:         nil,
		TargetSourceVersion: 8,
		ClassMajor:          52,
	}
	res := DispatchInvokeDynamic(serial, nil, nil, typ)
	if res.Status != "unsupported" {
		t.Fatalf("T19-C04 SERIALIZABLE must be unsupported, got %+v", res)
	}
	if res.ExecutedBootstrap {
		t.Fatal("T19-C04 executed bootstrap")
	}
	if !strings.Contains(strings.ToLower(res.Reason), "serializable") {
		t.Fatalf("T19-C04 must not silently drop SERIALIZABLE: %q", res.Reason)
	}

	unknown := serial
	unknown.StaticArgs = []values.JavaValue{sam, impl, sam, flags(8)}
	res = DispatchInvokeDynamic(unknown, nil, nil, typ)
	if res.Status != "unsupported" {
		t.Fatalf("T19-C04 unknown flags: %+v", res)
	}

	trunc := serial
	trunc.StaticArgs = []values.JavaValue{sam, impl, sam, flags(lambdaFlagMarkers)}
	res = DispatchInvokeDynamic(trunc, nil, nil, typ)
	if res.Status != "invalid_input" {
		t.Fatalf("T19-C04 FLAG_MARKERS missing count: %+v", res)
	}

	badCount := serial
	badCount.StaticArgs = []values.JavaValue{sam, impl, sam, flags(lambdaFlagMarkers), flags(2), marker}
	res = DispatchInvokeDynamic(badCount, nil, nil, typ)
	if res.Status != "invalid_input" {
		t.Fatalf("T19-C04 truncated markers: %+v", res)
	}

	// Valid markers/bridges (no SERIALIZABLE) must not be rejected at the flag boundary.
	okFlags := serial
	okFlags.StaticArgs = []values.JavaValue{sam, impl, sam, flags(lambdaFlagMarkers | lambdaFlagBridges), flags(1), marker, flags(0)}
	handledRes, handled := inspectAltMetafactoryFlags(okFlags, typ)
	if handled {
		t.Fatalf("T19-C04 valid markers/bridges must proceed to reconstruct, got %+v", handledRes)
	}

	extra := serial
	extra.StaticArgs = []values.JavaValue{sam, impl, sam, flags(0), flags(99)}
	res = DispatchInvokeDynamic(extra, nil, nil, typ)
	if res.Status != "invalid_input" {
		t.Fatalf("T19-C04 extra static args: %+v", res)
	}
}

func TestTaskT19MethodRefKindIdentity(t *testing.T) {
	t.Log("T19-C02")
	ctx := &class_context.ClassContext{ClassName: "MethodRefs"}
	if got := t19RenderMethodRef(ctx, RefNewInvokeSpecial, "MethodRefs", "<init>", nil); got != "MethodRefs::new" {
		t.Fatalf("ctor ref: %s", got)
	}
	if got := t19RenderMethodRef(ctx, RefInvokeStatic, "MethodRefs", "st", nil); !strings.Contains(got, "MethodRefs::st") {
		t.Fatalf("static ref: %s", got)
	}
	recv := values.NewJavaLiteral("o", types.NewJavaClass("MethodRefs"))
	if got := t19RenderMethodRef(ctx, RefInvokeVirtual, "MethodRefs", "inst", []values.JavaValue{recv}); !strings.Contains(got, "::inst") {
		t.Fatalf("bound ref: %s", got)
	}
	if got := t19RenderMethodRef(ctx, RefInvokeVirtual, "java.lang.String", "length", nil); !strings.Contains(got, "String::length") {
		t.Fatalf("unbound ref: %s", got)
	}
	if got := t19RenderMethodRef(ctx, RefNewInvokeSpecial, "[I", "<init>", nil); got != "int[]::new" {
		t.Fatalf("array ctor ref: %s", got)
	}
	for _, tc := range []struct {
		kind     uint8
		owner    string
		member   string
		captured []values.JavaValue
	}{
		{RefNewInvokeSpecial, "MethodRefs", "<init>", nil},
		{RefInvokeStatic, "MethodRefs", "st", nil},
		{RefInvokeVirtual, "MethodRefs", "inst", []values.JavaValue{recv}},
		{RefInvokeVirtual, "java.lang.String", "length", nil},
		{RefNewInvokeSpecial, "[I", "<init>", nil},
	} {
		out := workbudget.NewWriter(nil)
		if err := t19WriteMethodRef(ctx, out, tc.kind, tc.owner, tc.member, tc.captured); err != nil {
			t.Fatal(err)
		}
		if got, want := out.String(), t19RenderMethodRef(ctx, tc.kind, tc.owner, tc.member, tc.captured); got != want {
			t.Fatalf("streamed method reference changed: got=%q want=%q", got, want)
		}
	}
}

func TestTaskT19BudgetedLambdaCaptureRender(t *testing.T) {
	body := "() -> { return \x00LCAP0\x00 + \x00LCAP1\x00; }"
	captured := []values.JavaValue{
		values.NewJavaLiteral("first", types.NewJavaClass("java.lang.String")),
		values.NewJavaLiteral("second", types.NewJavaClass("java.lang.String")),
	}
	want := `() -> { return "first" + "second"; }`
	value := values.NewStreamingCustomValue(func(ctx *class_context.ClassContext, out *workbudget.Writer) error {
		return t19WriteLambdaBody(ctx, out, body, captured, "")
	}, func() types.JavaType { return types.NewJavaClass("java.lang.String") })
	if got := value.String(&class_context.ClassContext{}); got != want {
		t.Fatalf("unlimited output changed: got=%q want=%q", got, want)
	}
	for _, tc := range []struct {
		max      int64
		wantFail bool
	}{{int64(len(want) - 1), true}, {int64(len(want)), false}, {int64(len(want) + 1), false}} {
		ctx := &class_context.ClassContext{Work: workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: tc.max})}
		got := value.String(ctx)
		if tc.wantFail {
			if !workbudget.Is(ctx.Work.Err()) || got == want || len(got) >= len(want) {
				t.Fatalf("cap=%d should reject incomplete lambda: got=%q err=%v", tc.max, got, ctx.Work.Err())
			}
		} else if ctx.Work.Err() != nil || got != want {
			t.Fatalf("cap=%d changed lambda: got=%q want=%q err=%v", tc.max, got, want, ctx.Work.Err())
		}
	}

	castBody := "() -> { return value; }"
	projected, ok := lambdaReturnCastOutputLen(castBody, "T")
	if !ok || projected != int64(len(injectLambdaReturnCast(castBody, "T"))) {
		t.Fatalf("cast allocation preflight inaccurate: projected=%d ok=%v actual=%d", projected, ok, len(injectLambdaReturnCast(castBody, "T")))
	}
}

func TestTaskT19InlineVsMethodRef(t *testing.T) {
	t.Log("T19-C06")
	d := &Decompiler{FunctionContext: &class_context.ClassContext{ClassName: "LambdaCapture"}}
	lam := t19Impl("LambdaCapture", "lambda$main$0", "(III)I", RefInvokeStatic)
	if !t19ShouldInline(d, lam, 2, 1) {
		t.Fatal("lambda$ hint must inline")
	}
	renamed := t19Impl("LambdaCapture", "implBody$0000", "(III)I", RefInvokeStatic)
	if !t19ShouldInline(d, renamed, 2, 1) {
		t.Fatal("renamed same-class capturing body must still inline via handle arity")
	}
	plus := t19Impl("LambdaCapture", "plus", "(I)I", RefInvokeStatic)
	if t19ShouldInline(d, plus, 0, 1) {
		t.Fatal("static method ref plus must not be inlined as a lambda body")
	}
	plux := t19Impl("LambdaCapture", "plux", "(I)I", RefInvokeStatic)
	if t19ShouldInline(d, plux, 0, 1) {
		t.Fatal("renamed static method ref must stay a method ref")
	}
	inst := t19Impl("LambdaCapture", "lambda$instanceCapture$5", "(II)I", RefInvokeVirtual)
	if !t19ShouldInline(d, inst, 2, 1) {
		t.Fatal("instance lambda$ invokevirtual must inline, not become int::lambda$...")
	}
	special := t19Impl("LambdaCapture", "lambda$instanceCapture$5", "(II)I", RefInvokeSpecial)
	if !t19ShouldInline(d, special, 1, 1) {
		t.Fatal("instance lambda$ invokespecial must inline")
	}
	kt := t19Impl("LambdaCapture", "set$lambda-0", "(Ljava/lang/Object;Ljava/lang/Object;)V", RefInvokeStatic)
	if t19ShouldInline(d, kt, 1, 1) {
		t.Fatal("Kotlin set$lambda-0 must stay a method reference")
	}
}
