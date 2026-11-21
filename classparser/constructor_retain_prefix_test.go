package javaclassparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
)

func TestRetainedConstructorPrefixPureEscapingDeferral(t *testing.T) {
	c := classfilebuilder.NewClass("adv/PureDeferCtor", "adv/BaseCtor", 0x0021)
	c.AddField(0x0002, "sum", "I")
	superCtor, err := c.Pool().Methodref("adv/BaseCtor", "<init>", "(I)V")
	if err != nil {
		t.Fatal(err)
	}
	sumField, err := c.Pool().Fieldref("adv/PureDeferCtor", "sum", "I")
	if err != nil {
		t.Fatal(err)
	}
	code := c.AddMethod(0x0001, "<init>", "(II)V").Code()
	code.
		Emit(classfilebuilder.ICONST_1).
		Store(classfilebuilder.IntLocal, 3).
		Load(classfilebuilder.IntLocal, 1).
		Store(classfilebuilder.IntLocal, 4).
		Load(classfilebuilder.RefLocal, 0).
		Load(classfilebuilder.IntLocal, 2).
		CP(classfilebuilder.INVOKESPECIAL, superCtor).
		Load(classfilebuilder.RefLocal, 0).
		Load(classfilebuilder.IntLocal, 3).
		Load(classfilebuilder.IntLocal, 4).
		Emit(classfilebuilder.IADD).
		CP(classfilebuilder.PUTFIELD, sumField).
		Emit(classfilebuilder.RETURN)

	raw := mustBuildClass(t, c)

	strictRes, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
	if err != nil {
		t.Fatalf("strict DecompileWithOptions failed: %v", err)
	}
	if len(strictRes.StubMethods) == 0 || !strings.Contains(strictRes.Source, DecompileStubMarker) {
		t.Fatalf("expected strict mode to reject escaping constructor prefix, got stubs=%v:\n%s", strictRes.StubMethods, strictRes.Source)
	}

	retainedRes, err := DecompileWithOptions(raw, DecompileOptions{
		Mode:                 Compatibility,
		RetainUnprovedSource: true,
	})
	if err != nil {
		t.Fatalf("retained DecompileWithOptions failed: %v", err)
	}
	if len(retainedRes.StubMethods) != 0 || strings.Contains(retainedRes.Source, DecompileStubMarker) {
		t.Fatalf("expected retained pure deferral to emit zero stubs, got stubs=%v:\n%s", retainedRes.StubMethods, retainedRes.Source)
	}
	superIdx := strings.Index(retainedRes.Source, "super(")
	fieldIdx := strings.Index(retainedRes.Source, "this.sum =")
	if superIdx < 0 || fieldIdx < 0 || superIdx >= fieldIdx {
		t.Fatalf("expected deferred assignments into this.sum after super(...), got:\n%s", retainedRes.Source)
	}
}

func TestRetainedConstructorPrefixOrderedMultiArgAndFailClosed(t *testing.T) {
	t.Run("ordered_custom_cast_and_calls", func(t *testing.T) {
		c := classfilebuilder.NewClass("adv/OrderedCastCtor", "adv/BaseCastCtor", 0x0021)
		fRef, err := c.Pool().Methodref("adv/OrderedCastCtor", "f", "()I")
		if err != nil {
			t.Fatal(err)
		}
		gRef, err := c.Pool().Methodref("adv/OrderedCastCtor", "g", "()I")
		if err != nil {
			t.Fatal(err)
		}
		superRef, err := c.Pool().Methodref("adv/BaseCastCtor", "<init>", "(SI)V")
		if err != nil {
			t.Fatal(err)
		}
		c.AddMethod(0x0008, "f", "()I").Code().Emit(classfilebuilder.ICONST_3).Emit(classfilebuilder.IRETURN)
		c.AddMethod(0x0008, "g", "()I").Code().Emit(classfilebuilder.ICONST_4).Emit(classfilebuilder.IRETURN)
		c.AddMethod(0x0001, "<init>", "()V").Code().
			CP(classfilebuilder.INVOKESTATIC, fRef).
			Store(classfilebuilder.IntLocal, 1).
			Load(classfilebuilder.IntLocal, 1).
			Emit(classfilebuilder.I2S).
			Store(classfilebuilder.IntLocal, 2).
			CP(classfilebuilder.INVOKESTATIC, gRef).
			Store(classfilebuilder.IntLocal, 3).
			Load(classfilebuilder.RefLocal, 0).
			Load(classfilebuilder.IntLocal, 2).
			Load(classfilebuilder.IntLocal, 3).
			CP(classfilebuilder.INVOKESPECIAL, superRef).
			Emit(classfilebuilder.RETURN)

		res, err := DecompileWithOptions(mustBuildClass(t, c), DecompileOptions{
			Mode:                 Compatibility,
			RetainUnprovedSource: true,
		})
		if err != nil {
			t.Fatalf("DecompileWithOptions failed: %v", err)
		}
		if len(res.StubMethods) != 0 || strings.Contains(res.Source, DecompileStubMarker) {
			t.Fatalf("expected zero stubs for ordered cast + call prefix, got stubs=%v:\n%s", res.StubMethods, res.Source)
		}
		fIdx := strings.Index(res.Source, "f()")
		gIdx := strings.Index(res.Source, "g()")
		if fIdx < 0 || gIdx < 0 || fIdx >= gIdx || !strings.Contains(res.Source, "(short)") {
			t.Fatalf("expected (short) cast and f() before g(), got:\n%s", res.Source)
		}
	})

	t.Run("out_of_order_effects_never_reorder", func(t *testing.T) {
		c := classfilebuilder.NewClass("adv/OutOfOrderCtor", "adv/BaseIntPair", 0x0021)
		fRef, err := c.Pool().Methodref("adv/OutOfOrderCtor", "f", "()I")
		if err != nil {
			t.Fatal(err)
		}
		gRef, err := c.Pool().Methodref("adv/OutOfOrderCtor", "g", "()I")
		if err != nil {
			t.Fatal(err)
		}
		superRef, err := c.Pool().Methodref("adv/BaseIntPair", "<init>", "(II)V")
		if err != nil {
			t.Fatal(err)
		}
		c.AddMethod(0x0008, "f", "()I").Code().Emit(classfilebuilder.ICONST_1).Emit(classfilebuilder.IRETURN)
		c.AddMethod(0x0008, "g", "()I").Code().Emit(classfilebuilder.ICONST_2).Emit(classfilebuilder.IRETURN)
		c.AddMethod(0x0001, "<init>", "()V").Code().
			CP(classfilebuilder.INVOKESTATIC, gRef).
			Store(classfilebuilder.IntLocal, 2).
			CP(classfilebuilder.INVOKESTATIC, fRef).
			Store(classfilebuilder.IntLocal, 1).
			Load(classfilebuilder.RefLocal, 0).
			Load(classfilebuilder.IntLocal, 1).
			Load(classfilebuilder.IntLocal, 2).
			CP(classfilebuilder.INVOKESPECIAL, superRef).
			Emit(classfilebuilder.RETURN)

		res, err := DecompileWithOptions(mustBuildClass(t, c), DecompileOptions{
			Mode:                 Compatibility,
			RetainUnprovedSource: true,
		})
		if err != nil {
			t.Fatalf("DecompileWithOptions failed: %v", err)
		}
		if strings.Contains(res.Source, "int int ") {
			t.Fatalf("unexpected duplicate type prefix in helper statements:\n%s", res.Source)
		}
		if strings.Contains(res.Source, "super(f(),") || (len(res.StubMethods) == 0 && strings.Index(res.Source, "f()") < strings.Index(res.Source, "g()")) {
			t.Fatalf("constructor prefix wrongly reordered g() after f():\n%s", res.Source)
		}
	})

	t.Run("unconditional_effect_before_conditional_never_folds", func(t *testing.T) {
		c := classfilebuilder.NewClass("adv/ConditionalArgCtor", "adv/BaseInt", 0x0021)
		fRef, err := c.Pool().Methodref("adv/ConditionalArgCtor", "f", "()I")
		if err != nil {
			t.Fatal(err)
		}
		superRef, err := c.Pool().Methodref("adv/BaseInt", "<init>", "(I)V")
		if err != nil {
			t.Fatal(err)
		}
		c.AddMethod(0x0008, "f", "()I").Code().Emit(classfilebuilder.ICONST_5).Emit(classfilebuilder.IRETURN)
		code := c.AddMethod(0x0001, "<init>", "(Z)V").Code()
		zeroLbl := code.NewLabel()
		joinLbl := code.NewLabel()
		code.
			CP(classfilebuilder.INVOKESTATIC, fRef).
			Store(classfilebuilder.IntLocal, 2).
			Load(classfilebuilder.IntLocal, 1).
			Branch(classfilebuilder.IFEQ, zeroLbl).
			Load(classfilebuilder.IntLocal, 2).
			Branch(classfilebuilder.GOTO, joinLbl).
			Mark(zeroLbl).
			Emit(classfilebuilder.ICONST_0).
			Mark(joinLbl).
			Store(classfilebuilder.IntLocal, 3).
			Load(classfilebuilder.RefLocal, 0).
			Load(classfilebuilder.IntLocal, 3).
			CP(classfilebuilder.INVOKESPECIAL, superRef).
			Emit(classfilebuilder.RETURN)

		res, err := DecompileWithOptions(mustBuildClass(t, c), DecompileOptions{
			Mode:                 Compatibility,
			RetainUnprovedSource: true,
		})
		if err != nil {
			t.Fatalf("DecompileWithOptions failed: %v", err)
		}
		if len(res.StubMethods) == 0 && strings.Contains(res.Source, "? f()") {
			t.Fatalf("unconditional effect f() must never fold inside conditional arm:\n%s", res.Source)
		}
	})
}

func TestRetainedConstructorPrefixArrayInitializer(t *testing.T) {
	t.Run("fresh_1d_array_initializer", func(t *testing.T) {
		c := classfilebuilder.NewClass("adv/FreshArrCtor", "adv/ArrBase", 0x0021)
		superArr, err := c.Pool().Methodref("adv/ArrBase", "<init>", "([I)V")
		if err != nil {
			t.Fatal(err)
		}
		c.AddMethod(0x0001, "<init>", "()V").Code().
			Emit(classfilebuilder.ICONST_2).
			Byte(classfilebuilder.NEWARRAY, 10).
			Store(classfilebuilder.RefLocal, 1).
			Load(classfilebuilder.RefLocal, 1).
			Emit(classfilebuilder.ICONST_0).
			Byte(classfilebuilder.BIPUSH, 10).
			Emit(classfilebuilder.IASTORE).
			Load(classfilebuilder.RefLocal, 1).
			Emit(classfilebuilder.ICONST_1).
			Byte(classfilebuilder.BIPUSH, 20).
			Emit(classfilebuilder.IASTORE).
			Load(classfilebuilder.RefLocal, 0).
			Load(classfilebuilder.RefLocal, 1).
			CP(classfilebuilder.INVOKESPECIAL, superArr).
			Emit(classfilebuilder.RETURN)

		res, err := DecompileWithOptions(mustBuildClass(t, c), DecompileOptions{
			Mode:                 Compatibility,
			RetainUnprovedSource: true,
		})
		if err != nil {
			t.Fatalf("DecompileWithOptions failed: %v", err)
		}
		if len(res.StubMethods) != 0 || strings.Contains(res.Source, DecompileStubMarker) {
			t.Fatalf("expected fresh 1D array init to succeed without stub, got stubs=%v:\n%s", res.StubMethods, res.Source)
		}
		if !strings.Contains(res.Source, "super(new int[]{10,20})") && !strings.Contains(res.Source, "new int[]{10, 20}") {
			t.Fatalf("expected collapsed int[] initializer in super(...), got:\n%s", res.Source)
		}
	})

	t.Run("escaping_array_ref_fails_closed", func(t *testing.T) {
		c := classfilebuilder.NewClass("adv/EscapingArrCtor", "adv/ArrBase", 0x0021)
		c.AddField(0x0002, "arr", "[I")
		superArr, err := c.Pool().Methodref("adv/ArrBase", "<init>", "([I)V")
		if err != nil {
			t.Fatal(err)
		}
		arrField, err := c.Pool().Fieldref("adv/EscapingArrCtor", "arr", "[I")
		if err != nil {
			t.Fatal(err)
		}
		c.AddMethod(0x0001, "<init>", "()V").Code().
			Emit(classfilebuilder.ICONST_2).
			Byte(classfilebuilder.NEWARRAY, 10).
			Store(classfilebuilder.RefLocal, 1).
			Load(classfilebuilder.RefLocal, 1).
			Emit(classfilebuilder.ICONST_0).
			Byte(classfilebuilder.BIPUSH, 10).
			Emit(classfilebuilder.IASTORE).
			Load(classfilebuilder.RefLocal, 1).
			Emit(classfilebuilder.ICONST_1).
			Byte(classfilebuilder.BIPUSH, 20).
			Emit(classfilebuilder.IASTORE).
			Load(classfilebuilder.RefLocal, 0).
			Load(classfilebuilder.RefLocal, 1).
			CP(classfilebuilder.INVOKESPECIAL, superArr).
			Load(classfilebuilder.RefLocal, 0).
			Load(classfilebuilder.RefLocal, 1).
			CP(classfilebuilder.PUTFIELD, arrField).
			Emit(classfilebuilder.RETURN)

		res, err := DecompileWithOptions(mustBuildClass(t, c), DecompileOptions{
			Mode:                 Compatibility,
			RetainUnprovedSource: true,
		})
		if err != nil {
			t.Fatalf("DecompileWithOptions failed: %v", err)
		}
		if len(res.StubMethods) == 0 || !strings.Contains(res.Source, DecompileStubMarker) {
			t.Fatalf("expected escaping array reference to fail closed, got:\n%s", res.Source)
		}
	})

	t.Run("out_of_order_array_store_fails_closed", func(t *testing.T) {
		c := classfilebuilder.NewClass("adv/OutOfOrderArrCtor", "adv/ArrBase", 0x0021)
		superArr, err := c.Pool().Methodref("adv/ArrBase", "<init>", "([I)V")
		if err != nil {
			t.Fatal(err)
		}
		c.AddMethod(0x0001, "<init>", "()V").Code().
			Emit(classfilebuilder.ICONST_2).
			Byte(classfilebuilder.NEWARRAY, 10).
			Store(classfilebuilder.RefLocal, 1).
			Load(classfilebuilder.RefLocal, 1).
			Emit(classfilebuilder.ICONST_1).
			Byte(classfilebuilder.BIPUSH, 20).
			Emit(classfilebuilder.IASTORE).
			Load(classfilebuilder.RefLocal, 1).
			Emit(classfilebuilder.ICONST_0).
			Byte(classfilebuilder.BIPUSH, 10).
			Emit(classfilebuilder.IASTORE).
			Load(classfilebuilder.RefLocal, 0).
			Load(classfilebuilder.RefLocal, 1).
			CP(classfilebuilder.INVOKESPECIAL, superArr).
			Emit(classfilebuilder.RETURN)

		res, err := DecompileWithOptions(mustBuildClass(t, c), DecompileOptions{
			Mode:                 Compatibility,
			RetainUnprovedSource: true,
		})
		if err != nil {
			t.Fatalf("DecompileWithOptions failed: %v", err)
		}
		if strings.Contains(res.Source, "new int[]{") || strings.Contains(res.Source, "int[] int[]") {
			t.Fatalf("expected out-of-order array stores not to collapse or double-type:\n%s", res.Source)
		}
		if len(res.StubMethods) == 0 {
			i1 := strings.Index(res.Source, "[1] = 20")
			i0 := strings.Index(res.Source, "[0] = 10")
			if i1 < 0 || i0 < 0 || i1 >= i0 {
				t.Fatalf("expected out-of-order array helper to preserve store order:\n%s", res.Source)
			}
		}
	})
}
