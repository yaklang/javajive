package values

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestT22OutputCapBeforeStringLiteralAlloc(t *testing.T) {
	huge := strings.Repeat("x", 8000)
	ctx := &class_context.ClassContext{
		Work: workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: 64}),
	}
	lit := &JavaLiteral{Data: huge, JavaType: types.NewJavaClass("java.lang.String")}
	got := lit.String(ctx)
	if len(got) >= len(huge) {
		t.Fatalf("quoted full literal anyway: len=%d", len(got))
	}
	if got != "" {
		t.Fatalf("want empty reject, got len=%d", len(got))
	}
	if !workbudget.Is(ctx.Work.Err()) {
		t.Fatalf("want budget error, got %v", ctx.Work.Err())
	}
	var be *workbudget.Error
	if !workbudget.Is(ctx.Work.Err()) {
		t.Fatal("not workbudget error")
	}
	_ = be
}

func TestT22OutputCapBeforeDeepExpressionConcat(t *testing.T) {
	ctx := &class_context.ClassContext{
		Work: workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: 40}),
	}
	left := &JavaLiteral{Data: strings.Repeat("a", 30), JavaType: types.NewJavaClass("java.lang.String")}
	right := &JavaLiteral{Data: strings.Repeat("b", 30), JavaType: types.NewJavaClass("java.lang.String")}
	expr := &JavaExpression{Op: ADD, Values: []JavaValue{left, right}}
	got := expr.String(ctx)
	if got != "" && strings.Count(got, "a") >= 30 && strings.Count(got, "b") >= 30 {
		t.Fatalf("built full concat under tiny MaxOutputBytes: %q", got)
	}
	if !workbudget.Is(ctx.Work.Err()) {
		t.Fatalf("want budget error after leaf/concat charge, got %v result=%q", ctx.Work.Err(), got)
	}
}

func TestT22BushyTreeStopsBeforeCompleteConcat(t *testing.T) {
	strT := types.NewJavaClass("java.lang.String")
	leaf := func(s string) *JavaLiteral {
		return &JavaLiteral{Data: s, JavaType: strT}
	}
	add := func(a, b JavaValue) *JavaExpression {
		return &JavaExpression{Op: ADD, Values: []JavaValue{a, b}}
	}
	l1 := add(leaf(strings.Repeat("a", 20)), leaf(strings.Repeat("b", 20)))
	l2 := add(leaf(strings.Repeat("c", 20)), leaf(strings.Repeat("d", 20)))
	l3 := add(leaf(strings.Repeat("e", 20)), leaf(strings.Repeat("f", 20)))
	l4 := add(leaf(strings.Repeat("g", 20)), leaf(strings.Repeat("h", 20)))
	tree := add(add(l1, l2), add(l3, l4))
	ctx := &class_context.ClassContext{
		Work: workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: 80}),
	}
	got := tree.String(ctx)
	if strings.Count(got, "a") >= 20 && strings.Count(got, "h") >= 20 {
		t.Fatalf("bushy tree rendered complete under cap: len=%d", len(got))
	}
	if !workbudget.Is(ctx.Work.Err()) {
		t.Fatalf("want resource_limit, got %v got=%q", ctx.Work.Err(), got)
	}
}

func TestT22LiteralLpm1ExactT03Length(t *testing.T) {
	lit := &JavaLiteral{
		Data:     "ignore",
		Units:    []uint16{'A', 0, 0xD800, '\n'},
		JavaType: types.NewJavaClass("java.lang.String"),
	}
	L := int64(JavaStringLiteralOutputBytes(lit.Data, lit.Units))
	if data, _ := lit.Data.(string); L == int64(len(data)+2) {
		t.Fatal("Units length collapsed to len(Data)+2")
	}
	for _, tc := range []struct {
		max      int64
		wantFail bool
	}{{L - 1, true}, {L, false}, {L + 1, false}} {
		ctx := &class_context.ClassContext{
			Work: workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: tc.max}),
		}
		got := lit.String(ctx)
		failed := workbudget.Is(ctx.Work.Err())
		if tc.wantFail {
			if !failed {
				t.Fatalf("Max=%d L=%d: want fail, got %q err=%v", tc.max, L, got, ctx.Work.Err())
			}
			if int64(len(got)) >= L {
				t.Fatalf("Max=%d allocated complete literal len=%d", tc.max, len(got))
			}
		} else if failed {
			t.Fatalf("Max=%d L=%d: exact T03 length rejected: %v", tc.max, L, ctx.Work.Err())
		}
	}
}

func TestT22IntegerZeroExactNotEight(t *testing.T) {
	lit := &JavaLiteral{Data: 0, JavaType: types.NewJavaPrimer(types.JavaInteger)}
	L := literalExactOutputBytes(lit, &class_context.ClassContext{})
	if L != 1 {
		t.Fatalf("integer 0 exact len=%d want 1", L)
	}
	ctx := &class_context.ClassContext{
		Work: workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: 1}),
	}
	got := lit.String(ctx)
	if workbudget.Is(ctx.Work.Err()) {
		t.Fatalf("exact L=1 rejected integer 0: %v got=%q", ctx.Work.Err(), got)
	}
	if got != "0" {
		t.Fatalf("got %q", got)
	}
	// MaxOutputBytes==0 is unlimited (T22-C01); L-1 for a 1-byte literal cannot be
	// expressed as a public zero cap.
}

func assertPublicOutputLpm1(t *testing.T, name string, L int64, emit func(*class_context.ClassContext) string) {
	t.Helper()
	if L < 2 {
		t.Fatalf("%s L=%d; L-1 collides with unlimited MaxOutputBytes=0", name, L)
	}
	for _, tc := range []struct {
		max      int64
		wantFail bool
	}{{L - 1, true}, {L, false}, {L + 1, false}} {
		ctx := &class_context.ClassContext{
			Work: workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: tc.max}),
		}
		got := emit(ctx)
		failed := workbudget.Is(ctx.Work.Err())
		if tc.wantFail {
			if !failed {
				t.Fatalf("%s Max=%d L=%d: want fail, got %q", name, tc.max, L, got)
			}
			if int64(len(got)) >= L {
				t.Fatalf("%s Max=%d allocated complete output len=%d", name, tc.max, len(got))
			}
		} else if failed {
			t.Fatalf("%s Max=%d L=%d: exact public length rejected: %v got=%q", name, tc.max, L, ctx.Work.Err(), got)
		} else if int64(len(got)) != L && tc.max == L {
			t.Fatalf("%s exact L=%d emitted len=%d %q", name, L, len(got), got)
		}
	}
}

func TestT22ExactPublicOutputLpm1PrimitivesAndBinary(t *testing.T) {
	intLit := &JavaLiteral{Data: 10, JavaType: types.NewJavaPrimer(types.JavaInteger)}
	assertPublicOutputLpm1(t, "int10", 2, func(ctx *class_context.ClassContext) string { return intLit.String(ctx) })

	falseLit := &JavaLiteral{Data: 0, JavaType: types.NewJavaPrimer(types.JavaBoolean)}
	assertPublicOutputLpm1(t, "false", 5, func(ctx *class_context.ClassContext) string { return falseLit.String(ctx) })

	trueLit := &JavaLiteral{Data: 1, JavaType: types.NewJavaPrimer(types.JavaBoolean)}
	assertPublicOutputLpm1(t, "true", 4, func(ctx *class_context.ClassContext) string { return trueLit.String(ctx) })

	charLit := &JavaLiteral{Data: int('A'), JavaType: types.NewJavaPrimer(types.JavaChar)}
	charOut := charLit.String(&class_context.ClassContext{})
	if charOut != "'A'" {
		t.Fatalf("char A public %q", charOut)
	}
	assertPublicOutputLpm1(t, "charA", int64(len(charOut)), func(ctx *class_context.ClassContext) string { return charLit.String(ctx) })

	bin := &JavaExpression{Op: ADD, Values: []JavaValue{
		&JavaLiteral{Data: 1, JavaType: types.NewJavaPrimer(types.JavaInteger)},
		&JavaLiteral{Data: 2, JavaType: types.NewJavaPrimer(types.JavaInteger)},
	}}
	binOut := bin.String(&class_context.ClassContext{})
	if binOut != "(1) + (2)" {
		t.Fatalf("binary public %q", binOut)
	}
	assertPublicOutputLpm1(t, "add", int64(len(binOut)), func(ctx *class_context.ClassContext) string { return bin.String(ctx) })
}

func TestT22DeepTreeHitsASTDepth(t *testing.T) {
	intT := types.NewJavaPrimer(types.JavaInteger)
	var v JavaValue = &JavaLiteral{Data: 1, JavaType: intT}
	for i := 0; i < 12; i++ {
		v = &JavaExpression{Op: ADD, Values: []JavaValue{
			v,
			&JavaLiteral{Data: 1, JavaType: intT},
		}}
	}
	ctx := &class_context.ClassContext{
		Work: workbudget.New(context.Background(), workbudget.Limits{MaxASTDepth: 4}),
	}
	got := v.String(ctx)
	if !workbudget.Is(ctx.Work.Err()) {
		t.Fatalf("deep tree must hit ast_depth, got %q err=%v", got, ctx.Work.Err())
	}
}

func TestT22CustomValueCallbackBoundedAfterEmit(t *testing.T) {
	called := false
	cv := NewCustomValue(func(*class_context.ClassContext) string {
		called = true
		return strings.Repeat("x", 400)
	}, func() types.JavaType { return types.NewJavaClass("java.lang.String") })
	ctx := &class_context.ClassContext{
		Work: workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: 32}),
	}
	got := cv.String(ctx)
	if called {
		t.Fatal("opaque callback ran before its output could be bounded")
	}
	if got != "" {
		t.Fatalf("rejected opaque callback returned %q", got)
	}
	var budgetErr *workbudget.Error
	if !errors.As(ctx.Work.Err(), &budgetErr) || budgetErr.Kind != workbudget.KindResource || budgetErr.Counter != workbudget.CounterOutputBytes {
		t.Fatalf("want budget error, got %v result=%q", ctx.Work.Err(), got)
	}
	if !strings.Contains(budgetErr.Error(), "opaque renderer") {
		t.Fatalf("missing rejection reason: %v", budgetErr)
	}
}

func TestT22StreamingCustomValueChecksBeforeEachAppend(t *testing.T) {
	const source = "(this).toString()"
	assertPublicOutputLpm1(t, "streamed custom", int64(len(source)), func(ctx *class_context.ClassContext) string {
		cv := NewStreamingCustomValue(func(_ *class_context.ClassContext, out *workbudget.Writer) error {
			for _, part := range []string{"(this)", ".toString", "()"} {
				if err := out.WriteString(part); err != nil {
					return err
				}
			}
			return nil
		}, func() types.JavaType { return types.NewJavaClass("java.lang.String") })
		return cv.String(ctx)
	})
}
