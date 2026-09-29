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

func TestTaskT19DirectoryStreamFilterInstantiatedType(t *testing.T) {
	raw := types.NewJavaClass("java.nio.file.DirectoryStream$Filter")
	instantiated := func(desc string) values.JavaValue {
		return values.NewCustomValue(
			func(*class_context.ClassContext) string { return desc },
			func() types.JavaType { return types.NewJavaClass("java.lang.invoke.MethodType") },
		)
	}

	got, ok := types.AsParameterizedType(inferLambdaTypeFromInstantiated(raw, instantiated("(Ljava/nio/file/Path;)Z")))
	if !ok {
		t.Fatal("DirectoryStream.Filter did not retain its instantiated target type")
	}
	if got.RawClassName != "java.nio.file.DirectoryStream$Filter" || len(got.TypeArgs) != 1 {
		t.Fatalf("unexpected target type: %#v", got)
	}
	pathType, ok := got.TypeArgs[0].RawType().(*types.JavaClass)
	if !ok || pathType == nil || pathType.Name != "java.nio.file.Path" {
		t.Fatalf("Filter type argument = %#v, want java.nio.file.Path", got.TypeArgs[0])
	}

	for _, desc := range []string{
		"(Ljava/nio/file/Path;Ljava/lang/Object;)Z", // Wrong SAM arity must not invent Filter<T>.
		"(Ljava/nio/file/Path;)Ljava/lang/Object;",  // Filter.accept returns primitive boolean.
	} {
		if inferred := inferLambdaTypeFromInstantiated(raw, instantiated(desc)); inferred != nil {
			t.Errorf("malformed Filter SAM %q unexpectedly inferred %s", desc, inferred.String(&class_context.ClassContext{}))
		}
	}
	if inferred := inferLambdaTypeFromInstantiated(types.NewJavaClass("java.nio.file.OpenOption"), instantiated("(Ljava/nio/file/Path;)Z")); inferred != nil {
		t.Errorf("unrelated JDK interface unexpectedly inferred %s", inferred.String(&class_context.ClassContext{}))
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

func TestResolveLambdaReturnTypevarFromCovariantTarget(t *testing.T) {
	tests := []struct {
		name string
		fi   string
		sig  string
		want string
	}{
		{
			name: "bare supplier type variable",
			fi:   "java.util.function.Supplier",
			sig:  "<T:Ljava/lang/Object;>()Ljava/util/function/Supplier<TT;>;",
			want: "T",
		},
		{
			name: "covariant function result",
			fi:   "java.util.function.Function",
			sig:  "<T:Ljava/lang/Object;R:Ljava/lang/Object;>(Ljava/util/function/Function<-TT;+TR;>;)Ljava/util/function/Function<-TT;+TR;>;",
			want: "R",
		},
		{
			name: "covariant bifunction result",
			fi:   "java.util.function.BiFunction",
			sig:  "<T:Ljava/lang/Object;U:Ljava/lang/Object;R:Ljava/lang/Object;>()Ljava/util/function/BiFunction<-TT;-TU;+TR;>;",
			want: "R",
		},
		{
			name: "lower bound is not an exact return target",
			fi:   "java.util.function.Function",
			sig:  "<T:Ljava/lang/Object;R:Ljava/lang/Object;>()Ljava/util/function/Function<TT;-TR;>;",
		},
		{
			name: "concrete result needs no typevar cast",
			fi:   "java.util.function.Function",
			sig:  "()Ljava/util/function/Function<Ljava/lang/Object;Ljava/lang/String;>;",
		},
		{
			name: "unbounded result is not evidence",
			fi:   "java.util.function.Function",
			sig:  "()Ljava/util/function/Function<Ljava/lang/Object;*>;",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := &class_context.ClassContext{CurrentMethodSig: test.sig}
			if got := resolveLambdaReturnTypevar(ctx, test.fi); got != test.want {
				t.Fatalf("resolveLambdaReturnTypevar(%q, %q) = %q, want %q", test.sig, test.fi, got, test.want)
			}
		})
	}
}

func TestPolyLambdaReturnTargetUsesGenericSAMResult(t *testing.T) {
	function := func(result types.JavaType) types.JavaType {
		return types.NewParameterizedType("java.util.function.Function", []types.JavaType{
			types.NewJavaClass("java.lang.Object"), result,
		})
	}

	t.Run("bare type variable", func(t *testing.T) {
		lambda := values.NewCustomValue(func(*class_context.ClassContext) string { return "" }, func() types.JavaType {
			return function(types.NewJavaClass("java.lang.Object"))
		})
		lambda.Flag = "lambda"
		lambda.InstantiatedMtdDesc = "(Ljava/lang/Object;)Ljava/lang/Object;"
		target := types.NewParameterizedType("java.util.function.Function", []types.JavaType{
			&types.JavaWildcardType{Variant: "super", Bound: types.NewJavaClass("K")},
			&types.JavaWildcardType{Variant: "extends", Bound: types.NewJavaClass("V")},
		})
		applyPolyLambdaReturnTarget(lambda, target)
		if lambda.LambdaReturnTarget == nil || lambda.LambdaReturnTarget.String(&class_context.ClassContext{}) != "V" || lambda.LambdaReturnRawBridge {
			t.Fatalf("type-variable target not recovered: target=%v bridge=%v", lambda.LambdaReturnTarget, lambda.LambdaReturnRawBridge)
		}
	})

	t.Run("nested parameterization", func(t *testing.T) {
		rawFuture := types.NewJavaClass("java.util.concurrent.CompletableFuture")
		lambda := values.NewCustomValue(func(*class_context.ClassContext) string { return "" }, func() types.JavaType {
			return function(rawFuture)
		})
		lambda.Flag = "lambda"
		lambda.InstantiatedMtdDesc = "(Ljava/lang/Object;)Ljava/util/concurrent/CompletableFuture;"
		futureV := types.NewParameterizedType("java.util.concurrent.CompletableFuture", []types.JavaType{types.NewJavaClass("V")})
		target := types.NewParameterizedType("java.util.function.Function", []types.JavaType{
			types.NewJavaClass("K"), futureV,
		})
		applyPolyLambdaReturnTarget(lambda, target)
		if lambda.LambdaReturnTarget == nil || lambda.LambdaReturnTarget.String(&class_context.ClassContext{}) != "CompletableFuture<V>" || !lambda.LambdaReturnRawBridge {
			t.Fatalf("parameterized target not recovered: target=%v bridge=%v", lambda.LambdaReturnTarget, lambda.LambdaReturnRawBridge)
		}
	})

	t.Run("consumer and method reference stay untouched", func(t *testing.T) {
		for _, methodRef := range []bool{false, true} {
			lambda := values.NewCustomValue(func(*class_context.ClassContext) string { return "" }, func() types.JavaType {
				return types.NewParameterizedType("java.util.function.Consumer", []types.JavaType{types.NewJavaClass("java.lang.Object")})
			})
			lambda.Flag = "lambda"
			lambda.IsMethodRef = methodRef
			lambda.InstantiatedMtdDesc = "(Ljava/lang/Object;)V"
			target := types.NewParameterizedType("java.util.function.Consumer", []types.JavaType{types.NewJavaClass("K")})
			applyPolyLambdaReturnTarget(lambda, target)
			if lambda.LambdaReturnTarget != nil {
				t.Fatalf("void/method-reference target was modified: methodRef=%v", methodRef)
			}
		}
	})

	t.Run("kill switch", func(t *testing.T) {
		t.Setenv("JDEC_POLY_LAMBDA_RETURN_CAST_OFF", "1")
		lambda := values.NewCustomValue(func(*class_context.ClassContext) string { return "" }, func() types.JavaType {
			return function(types.NewJavaClass("java.lang.Object"))
		})
		lambda.Flag = "lambda"
		lambda.InstantiatedMtdDesc = "(Ljava/lang/Object;)Ljava/lang/Object;"
		applyPolyLambdaReturnTarget(lambda, function(types.NewJavaClass("V")))
		if lambda.LambdaReturnTarget != nil {
			t.Fatal("kill switch retained return target")
		}
	})
}

func TestLambdaReturnParameterizedRawBridge(t *testing.T) {
	body := "(x) -> { return future; }"
	want := "(x) -> { return (CompletableFuture<V>) (CompletableFuture) (future); }"
	if got := injectLambdaReturnCastWithBridge(body, "CompletableFuture<V>", "CompletableFuture"); got != want {
		t.Fatalf("raw bridge = %q, want %q", got, want)
	}
	projected, ok := lambdaReturnCastOutputLenWithBridge(body, "CompletableFuture<V>", "CompletableFuture")
	if !ok || projected != int64(len(want)) {
		t.Fatalf("raw bridge preflight = %d/%v, want %d", projected, ok, len(want))
	}

	multi := `(x) -> {
  if (x) { return first; }
  Runnable nested = () -> { return; };
  java.util.function.Supplier<Object> child = () -> { return childValue; };
  return (V) (last);
}`
	wantMulti := `(x) -> {
  if (x) { return (V) (first); }
  Runnable nested = () -> { return; };
  java.util.function.Supplier<Object> child = () -> { return childValue; };
  return (V) (last);
}`
	if got := injectLambdaReturnCast(multi, "V"); got != wantMulti {
		t.Fatalf("multi-return targeting crossed a nested lambda:\ngot:\n%s\nwant:\n%s", got, wantMulti)
	}
	projected, ok = lambdaReturnCastOutputLen(multi, "V")
	if !ok || projected != int64(len(wantMulti)) {
		t.Fatalf("multi-return preflight = %d/%v, want %d", projected, ok, len(wantMulti))
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

func TestPolyLambdaInputErasureRequiresContravariantSameErasure(t *testing.T) {
	ctx := &class_context.ClassContext{}
	raw := types.NewJavaClass("java.util.Map$Entry")
	entry := types.NewParameterizedType("java.util.Map$Entry", []types.JavaType{types.NewJavaClass("K"), types.NewJavaClass("V")})
	lambda := values.NewCustomValue(nil, func() types.JavaType {
		return types.NewParameterizedType("java.util.function.Function", []types.JavaType{raw, raw})
	})
	lambda.Flag = "lambda"
	for _, test := range []struct {
		name  string
		input types.JavaType
		want  string
	}{
		{"contravariant", &types.JavaWildcardType{Variant: "super", Bound: entry}, "Function<Map.Entry, Map.Entry<K, V>>"},
		{"invariant", entry, "Function<Map.Entry<K, V>, Map.Entry<K, V>>"},
		{"covariant", &types.JavaWildcardType{Variant: "extends", Bound: entry}, "Function<? extends Map.Entry<K, V>, Map.Entry<K, V>>"},
		{"other erasure", &types.JavaWildcardType{Variant: "super", Bound: types.NewParameterizedType("java.util.List", []types.JavaType{types.NewJavaClass("V")})}, "Function<? super List<V>, Map.Entry<K, V>>"},
	} {
		t.Run(test.name, func(t *testing.T) {
			target := types.NewParameterizedType("java.util.function.Function", []types.JavaType{test.input, entry})
			got := polyLambdaInputTarget(lambda, target)
			if got.String(ctx) != test.want {
				t.Fatalf("target = %s, want %s", got.String(ctx), test.want)
			}
		})
	}
}
