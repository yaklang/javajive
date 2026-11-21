package javaclassparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
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

func TestRetainedConstructorThisAliasAndRemainingFieldStores(t *testing.T) {
	t.Run("this_alias_this_delegation", func(t *testing.T) {
		c := classfilebuilder.NewClass("adv/ThisAliasSelfCtor", "java/lang/Object", 0x0021)
		c.AddField(0x0018, "STYLE", "I")
		styleField, err := c.Pool().Fieldref("adv/ThisAliasSelfCtor", "STYLE", "I")
		if err != nil {
			t.Fatal(err)
		}
		intCtor, err := c.Pool().Methodref("adv/ThisAliasSelfCtor", "<init>", "(II)V")
		if err != nil {
			t.Fatal(err)
		}
		objCtor, err := c.Pool().Methodref("java/lang/Object", "<init>", "()V")
		if err != nil {
			t.Fatal(err)
		}
		c.AddMethod(0x0001, "<init>", "(II)V").Code().
			Load(classfilebuilder.RefLocal, 0).
			CP(classfilebuilder.INVOKESPECIAL, objCtor).
			Emit(classfilebuilder.RETURN)
		// Place a prefix barrier after aliasing `this` so `var1` stays a distinct IsThisAlias local.
		c.AddMethod(0x0001, "<init>", "(I)V").Code().
			Load(classfilebuilder.RefLocal, 0).
			Store(classfilebuilder.RefLocal, 2).
			CP(classfilebuilder.GETSTATIC, styleField).
			Store(classfilebuilder.IntLocal, 3).
			Load(classfilebuilder.RefLocal, 2).
			Load(classfilebuilder.IntLocal, 1).
			Load(classfilebuilder.IntLocal, 3).
			CP(classfilebuilder.INVOKESPECIAL, intCtor).
			Emit(classfilebuilder.RETURN)

		raw := mustBuildClass(t, c)

		strictRes, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
		if err != nil {
			t.Fatalf("strict DecompileWithOptions failed: %v", err)
		}
		if len(strictRes.StubMethods) == 0 && !strings.Contains(strictRes.Source, DecompileStubMarker) {
			t.Fatalf("expected Precision mode to reject unproved IsThisAlias prefix:\n%s", strictRes.Source)
		}

		t.Setenv("JDEC_THIS_ALIAS_SUPER_OFF", "")
		t.Setenv("JDEC_CTOR_RETAIN_PREFIX_OFF", "")
		res, err := DecompileWithOptions(raw, DecompileOptions{
			Mode:                 Compatibility,
			RetainUnprovedSource: true,
		})
		if err != nil {
			t.Fatalf("retained DecompileWithOptions failed: %v", err)
		}
		if len(res.StubMethods) != 0 || !strings.Contains(res.Source, "this(var1,") {
			t.Fatalf("expected this(var1, ...); with 0 stubs, got stubs=%v:\n%s", res.StubMethods, res.Source)
		}
		if strings.Contains(res.Source, "= this;") {
			t.Fatalf("expected dead this-alias copy dropped:\n%s", res.Source)
		}

		t.Setenv("JDEC_THIS_ALIAS_SUPER_OFF", "1")
		offRes, err := DecompileWithOptions(raw, DecompileOptions{
			Mode:                 Compatibility,
			RetainUnprovedSource: true,
		})
		if err != nil {
			t.Fatalf("kill-switch DecompileWithOptions failed: %v", err)
		}
		if strings.Contains(offRes.Source, "this(var1,") {
			t.Fatalf("JDEC_THIS_ALIAS_SUPER_OFF=1 did not disable IsThisAlias this(...) delegation:\n%s", offRes.Source)
		}
	})

	t.Run("this_alias_super_with_pre_super_barrier_and_field_capture", func(t *testing.T) {
		c := classfilebuilder.NewClass("adv/ThisAliasSuperCap", "adv/BaseCap", 0x0021)
		c.AddField(0x0010, "m", "Ljava/lang/Object;")
		c.AddField(0x0018, "STYLE", "I")
		styleField, err := c.Pool().Fieldref("adv/ThisAliasSuperCap", "STYLE", "I")
		if err != nil {
			t.Fatal(err)
		}
		capField, err := c.Pool().Fieldref("adv/ThisAliasSuperCap", "m", "Ljava/lang/Object;")
		if err != nil {
			t.Fatal(err)
		}
		superCtor, err := c.Pool().Methodref("adv/BaseCap", "<init>", "(II)V")
		if err != nil {
			t.Fatal(err)
		}
		c.AddMethod(0x0001, "<init>", "(Ljava/lang/Object;I)V").Code().
			Load(classfilebuilder.RefLocal, 0).
			Load(classfilebuilder.RefLocal, 1).
			CP(classfilebuilder.PUTFIELD, capField).
			Load(classfilebuilder.RefLocal, 0).
			Store(classfilebuilder.RefLocal, 4).
			CP(classfilebuilder.GETSTATIC, styleField).
			Store(classfilebuilder.IntLocal, 3).
			Load(classfilebuilder.RefLocal, 4).
			Load(classfilebuilder.IntLocal, 2).
			Load(classfilebuilder.IntLocal, 3).
			CP(classfilebuilder.INVOKESPECIAL, superCtor).
			Emit(classfilebuilder.RETURN)

		raw := mustBuildClass(t, c)
		t.Setenv("JDEC_THIS_ALIAS_SUPER_OFF", "")
		t.Setenv("JDEC_CTOR_RETAIN_PREFIX_OFF", "")
		res, err := DecompileWithOptions(raw, DecompileOptions{
			Mode:                 Compatibility,
			RetainUnprovedSource: true,
		})
		if err != nil {
			t.Fatalf("DecompileWithOptions failed: %v", err)
		}
		if len(res.StubMethods) != 0 || strings.Contains(res.Source, DecompileStubMarker) {
			t.Fatalf("expected 0 stubs, got stubs=%v:\n%s", res.StubMethods, res.Source)
		}
		superIdx := strings.Index(res.Source, "super(")
		fieldIdx := strings.Index(res.Source, "this.m =")
		if superIdx < 0 || fieldIdx < 0 || superIdx >= fieldIdx {
			t.Fatalf("expected super(...) before this.m = ...:\n%s", res.Source)
		}

		strictRes, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
		if err != nil {
			t.Fatalf("strict DecompileWithOptions failed: %v", err)
		}
		if len(strictRes.StubMethods) == 0 || !strings.Contains(strictRes.Source, DecompileStubMarker) {
			t.Fatalf("expected Precision mode to reject escaping pre-super field store + alias:\n%s", strictRes.Source)
		}

		t.Setenv("JDEC_THIS_ALIAS_SUPER_OFF", "1")
		offRes, err := DecompileWithOptions(raw, DecompileOptions{
			Mode:                 Compatibility,
			RetainUnprovedSource: true,
		})
		if err != nil {
			t.Fatalf("kill-switch DecompileWithOptions failed: %v", err)
		}
		if !strings.Contains(offRes.Source, "= this;") {
			t.Fatalf("JDEC_THIS_ALIAS_SUPER_OFF=1 did not disable IsThisAlias alias folding/boundary recognition:\n%s", offRes.Source)
		}
	})

	t.Run("canCommitWithRemainingThisFieldStores_fail_closed", func(t *testing.T) {
		ctx := &class_context.ClassContext{ClassName: "adv.Sub"}
		thisRef := values.NewJavaRef(coreutils.NewRootVariableId().Next(), nil, types.NewJavaClass("adv.Sub"))
		thisRef.IsThis = true
		pID := coreutils.NewRootVariableId().Next()
		pID.SetName("var1")
		pRef := values.NewJavaRef(pID, nil, types.NewJavaPrimer(types.JavaInteger))
		pRef.IsParam = true
		tmpID := coreutils.NewRootVariableId().Next()
		tmpID.SetName("var3")
		tmpRef := values.NewJavaRef(tmpID, nil, types.NewJavaPrimer(types.JavaInteger))

		fieldStore := statements.NewAssignStatement(&values.RefMember{Object: thisRef, Member: "m"}, pRef, false)
		delegate := &values.FunctionCallExpression{ClassName: "adv.Base", FunctionName: "<init>", Kind: values.InvokeSpecial}

		if !canCommitWithRemainingThisFieldStores([]statements.Statement{fieldStore}, nil, map[*coreutils.VariableId]values.JavaValue{}, []values.JavaValue{values.NewJavaLiteral(1, pRef.Type())}, delegate, "adv/Base", ctx) {
			t.Fatal("pure this.m = param remaining store should commit")
		}

		// Negative 1: non-this-field barrier in remaining.
		callStmt := statements.NewExpressionStatement(&values.FunctionCallExpression{ClassName: "adv.Sub", FunctionName: "tick", Kind: values.InvokeStatic})
		if canCommitWithRemainingThisFieldStores([]statements.Statement{callStmt}, nil, map[*coreutils.VariableId]values.JavaValue{}, nil, delegate, "adv/Base", ctx) {
			t.Fatal("non-field call in remaining must fail closed")
		}

		// Negative 2: super argument writes parameter read by remaining this.m store.
		mutArg := values.NewAssignmentExpression(pRef, values.NewJavaLiteral(9, pRef.Type()), -1, nil)
		if canCommitWithRemainingThisFieldStores([]statements.Statement{fieldStore}, nil, map[*coreutils.VariableId]values.JavaValue{}, []values.JavaValue{mutArg}, delegate, "adv/Base", ctx) {
			t.Fatal("remaining store reading arg-mutated parameter must fail closed")
		}

		// Negative 3: remaining store reads a folded local still in env.
		foldedStore := statements.NewAssignStatement(&values.RefMember{Object: thisRef, Member: "m"}, tmpRef, false)
		env := map[*coreutils.VariableId]values.JavaValue{tmpID: pRef}
		if canCommitWithRemainingThisFieldStores([]statements.Statement{foldedStore}, nil, env, nil, delegate, "adv/Base", ctx) {
			t.Fatal("remaining store referencing folded local must fail closed")
		}
	})
}
