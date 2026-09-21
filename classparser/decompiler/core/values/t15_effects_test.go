package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestT15_C05_UnknownBarrierCustomValue(t *testing.T) {
	t.Run("T15-C05", func(t *testing.T) { testT15C05UnknownBarrier(t) })
}

func testT15C05UnknownBarrier(t *testing.T) {
	intType := types.NewJavaPrimer(types.JavaInteger)
	opaque := NewCustomValue(func(funcCtx *class_context.ClassContext) string {
		return "1+1"
	}, func() types.JavaType { return intType })
	if IsPure(opaque) {
		t.Fatal("opaque CustomValue must not be judged pure from text")
	}
	if MayFold(opaque) {
		t.Fatal("opaque CustomValue must not fold")
	}
	if EffectSummary(opaque) != "unknown" {
		t.Fatalf("summary=%s want unknown", EffectSummary(opaque))
	}
	if TryFoldTyped(opaque, NewJavaLiteral(2, intType)) != opaque {
		t.Fatal("unknown barrier must not be replaced")
	}

	inner := NewCustomValue(func(funcCtx *class_context.ClassContext) string { return "inner" }, func() types.JavaType { return intType })
	outer := NewCustomValue(func(funcCtx *class_context.ClassContext) string { return "outer" }, func() types.JavaType { return intType })
	outer.CapturesKnown = true
	outer.Flag = "nested-unknown"
	outer.Captures = []JavaValue{inner}
	if IsPure(outer) || MayFold(outer) {
		t.Fatal("nested unknown CustomValue is a barrier")
	}

	a := NewCustomValue(func(funcCtx *class_context.ClassContext) string { return "a" }, func() types.JavaType { return intType })
	b := NewCustomValue(func(funcCtx *class_context.ClassContext) string { return "b" }, func() types.JavaType { return intType })
	a.CapturesKnown = true
	a.Flag = "lambda"
	b.CapturesKnown = true
	b.Flag = "lambda"
	a.Captures = []JavaValue{b}
	b.Captures = []JavaValue{a}
	done := make(chan struct{})
	go func() {
		_, _ = InspectValue(a)
		close(done)
	}()
	select {
	case <-done:
	default:
		<-done
	}
	if IsPure(a) {
		t.Fatal("cyclic lambda captures are not pure")
	}

	unknownCycle := NewCustomValue(func(funcCtx *class_context.ClassContext) string { return "u" }, func() types.JavaType { return intType })
	unknownCycle.Captures = []JavaValue{unknownCycle}
	_, _ = InspectValue(unknownCycle)
	if MayFold(unknownCycle) {
		t.Fatal("cyclic unknown CustomValue is not foldable")
	}
}

func TestT15_MayFoldBarriers(t *testing.T) {
	t.Run("T15-C05", func(t *testing.T) { testT15MayFoldBarriers(t) })
}

func testT15MayFoldBarriers(t *testing.T) {
	intType := types.NewJavaPrimer(types.JavaInteger)
	lit := NewJavaLiteral(1, intType)
	if !MayFold(lit) || !IsPure(lit) || EffectSummary(lit) != "pure" {
		t.Fatalf("literal should be pure, got %s fold=%v", EffectSummary(lit), MayFold(lit))
	}
	call := &FunctionCallExpression{
		FunctionName: "f",
		FuncType:     &types.JavaFuncType{ReturnType: intType},
	}
	if MayFold(call) || IsPure(call) {
		t.Fatal("call is not foldable")
	}
	if got := EffectSummary(call); got == "pure" || got == "unknown" {
		t.Fatalf("call summary=%s", got)
	}
	vol := TagVolatile(lit)
	if MayFold(vol) || IsPure(vol) || EffectSummary(vol) != "volatile" {
		t.Fatalf("volatile barrier summary=%s fold=%v", EffectSummary(vol), MayFold(vol))
	}
	mon := TagMonitor(lit)
	if MayFold(mon) || EffectSummary(mon) != "monitor" {
		t.Fatalf("monitor barrier summary=%s", EffectSummary(mon))
	}
	clinit := TagClassInit(lit)
	if MayFold(clinit) || EffectSummary(clinit) != "init" {
		t.Fatalf("class-init barrier summary=%s", EffectSummary(clinit))
	}
	st := NewJavaClassMember("p.Init", "V", "I", intType)
	if MayFold(st) {
		t.Fatal("getstatic must not fold across class-init")
	}
	if EffectSummary(st) == "pure" {
		t.Fatal("getstatic is not pure")
	}

	same := types.NewJavaClass("java.lang.String")
	folded := NewCastExpression(NewJavaLiteral("x", same), same, 0)
	if _, ok := folded.(*CastExpression); ok {
		t.Fatal("identity cast of a foldable literal should drop")
	}
	obj := types.NewJavaClass("java.lang.Object")
	barrierCast := NewCastExpression(opaqueValue(obj), obj, 1)
	if _, ok := barrierCast.(*CastExpression); !ok {
		t.Fatal("opaque operand must keep the cast")
	}
}

func opaqueValue(typ types.JavaType) JavaValue {
	return NewCustomValue(func(funcCtx *class_context.ClassContext) string { return "opaque" }, func() types.JavaType { return typ })
}
