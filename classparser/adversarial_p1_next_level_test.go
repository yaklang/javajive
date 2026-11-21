package javaclassparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestAdversarialP1ConstructorPrefixAndStrictGating(t *testing.T) {
	t.Run("local_const_and_kotlin_mask_fold", func(t *testing.T) {
		base := classfilebuilder.NewClass("p1/BaseMask", "java/lang/Object", 0x0021)
		bmInit := base.AddMethod(0x0001, "<init>", "(Ljava/lang/String;I)V")
		bmSuper, _ := base.Pool().Methodref("java/lang/Object", "<init>", "()V")
		bmInit.Code().Load(classfilebuilder.RefLocal, 0).CP(classfilebuilder.INVOKESPECIAL, bmSuper).Emit(classfilebuilder.RETURN)
		baseRaw := mustBuildClass(t, base)

		sub := classfilebuilder.NewClass("p1/SubMask", "p1/BaseMask", 0x0021)
		pool := sub.Pool()
		subSuper, _ := pool.Methodref("p1/BaseMask", "<init>", "(Ljava/lang/String;I)V")
		defStr, _ := pool.String("default")
		ctor := sub.AddMethod(0x0001, "<init>", "(Ljava/lang/String;I)V")
		// Local mask conditional before super(...), plus unused local const:
		//   if ((p2 & 1) != 0) p1 = "default";
		//   int local = 7;
		//   super(p1, local);
		lCall := ctor.Code().NewLabel()
		ctor.Code().
			Load(classfilebuilder.IntLocal, 2).
			Emit(classfilebuilder.ICONST_1).
			Emit(classfilebuilder.IAND).
			Branch(classfilebuilder.IFEQ, lCall).
			Load(classfilebuilder.RefLocal, 0).
			Emit(classfilebuilder.POP). // no-op stack balance marker; adjust p1 safely
			LDC(defStr).
			Store(classfilebuilder.RefLocal, 1).
			Mark(lCall).
			Byte(classfilebuilder.BIPUSH, 7).
			Store(classfilebuilder.IntLocal, 3).
			Load(classfilebuilder.RefLocal, 0).
			Load(classfilebuilder.RefLocal, 1).
			Load(classfilebuilder.IntLocal, 3).
			CP(classfilebuilder.INVOKESPECIAL, subSuper).
			Emit(classfilebuilder.RETURN)

		raw := mustBuildClass(t, sub)
		resolve := func(name string) ([]byte, bool) {
			if name == "p1/BaseMask" {
				return baseRaw, true
			}
			return nil, false
		}

		// Strict mode (RetainUnprovedSource=false) must keep rejecting unproved local prefix.
		strictRes, err := DecompileWithOptions(raw, DecompileOptions{
			Mode:                 Precision,
			Resolve:              resolve,
			RetainUnprovedSource: false,
		})
		if err == nil && len(strictRes.StubMethods) == 0 {
			t.Fatal("strict mode unexpectedly accepted escaping constructor prefix")
		}

		// RetainUnprovedSource=true folds local & ternary into super(...).
		res, err := DecompileWithOptions(raw, DecompileOptions{
			Mode:                 Compatibility,
			Resolve:              resolve,
			RetainUnprovedSource: true,
		})
		if err != nil {
			t.Fatalf("RetainUnprovedSource failed: %v", err)
		}
		if len(res.StubMethods) != 0 {
			t.Fatalf("expected 0 stub methods, got %v\n%s", res.StubMethods, res.Source)
		}
		if !strings.Contains(res.Source, "super(") || !strings.Contains(res.Source, `"default"`) || !strings.Contains(res.Source, ",7)") {
			t.Fatalf("expected folded ternary and constant in super(...):\n%s", res.Source)
		}
	})

	t.Run("escaping_param_mutated_before_super_and_used_after", func(t *testing.T) {
		base := classfilebuilder.NewClass("p1/BaseCtx", "java/lang/Object", 0x0021)
		bInit := base.AddMethod(0x0001, "<init>", "(Ljava/lang/String;)V")
		bSuper, _ := base.Pool().Methodref("java/lang/Object", "<init>", "()V")
		bInit.Code().Load(classfilebuilder.RefLocal, 0).CP(classfilebuilder.INVOKESPECIAL, bSuper).Emit(classfilebuilder.RETURN)
		baseRaw := mustBuildClass(t, base)

		sub := classfilebuilder.NewClass("p1/SubCtx", "p1/BaseCtx", 0x0021)
		sub.AddField(0x0001, "saved", "Ljava/lang/String;")
		pool := sub.Pool()
		fSaved, _ := pool.Fieldref("p1/SubCtx", "saved", "Ljava/lang/String;")
		subSuper, _ := pool.Methodref("p1/BaseCtx", "<init>", "(Ljava/lang/String;)V")
		trimRef, _ := pool.Methodref("java/lang/String", "trim", "()Ljava/lang/String;")
		ctor := sub.AddMethod(0x0001, "<init>", "(Ljava/lang/String;)V")
		// p1 = p1.trim(); super(p1); this.saved = p1;
		ctor.Code().
			Load(classfilebuilder.RefLocal, 1).
			CP(classfilebuilder.INVOKEVIRTUAL, trimRef).
			Store(classfilebuilder.RefLocal, 1).
			Load(classfilebuilder.RefLocal, 0).
			Load(classfilebuilder.RefLocal, 1).
			CP(classfilebuilder.INVOKESPECIAL, subSuper).
			Load(classfilebuilder.RefLocal, 0).
			Load(classfilebuilder.RefLocal, 1).
			CP(classfilebuilder.PUTFIELD, fSaved).
			Emit(classfilebuilder.RETURN)

		raw := mustBuildClass(t, sub)
		resolve := func(name string) ([]byte, bool) {
			if name == "p1/BaseCtx" {
				return baseRaw, true
			}
			return nil, false
		}
		res, err := DecompileWithOptions(raw, DecompileOptions{
			Mode:                 Compatibility,
			Resolve:              resolve,
			RetainUnprovedSource: true,
		})
		if err != nil {
			t.Fatalf("RetainUnprovedSource failed on escaping param: %v", err)
		}
		if len(res.StubMethods) != 0 {
			t.Fatalf("expected 0 stubs, got %v\n%s", res.StubMethods, res.Source)
		}
		if !strings.Contains(res.Source, "super(") || !strings.Contains(res.Source, ".trim()") || !strings.Contains(res.Source, "this.saved") {
			t.Fatalf("expected in-place parameter assignment in super(...) and post-super field assignment:\n%s", res.Source)
		}
	})
}

func TestAdversarialP1CheckedEscapeIncompleteHierarchy(t *testing.T) {
	c := classfilebuilder.NewClass("p1/MissingSuperActivity", "android/missing/BaseActivity", 0x0021)
	pool := c.Pool()
	initRef, _ := pool.Methodref("android/missing/BaseActivity", "<init>", "()V")
	ctor := c.AddMethod(0x0001, "<init>", "()V")
	ctor.Code().Load(classfilebuilder.RefLocal, 0).CP(classfilebuilder.INVOKESPECIAL, initRef).Emit(classfilebuilder.RETURN)

	// Checked exception IOException thrown inside body of a method not declaring throws IOException.
	ioCls, _ := pool.Class("java/io/IOException")
	ioInit, _ := pool.Methodref("java/io/IOException", "<init>", "()V")
	m := c.AddMethod(0x0001, "failChecked", "()V")
	m.Code().
		CP(classfilebuilder.NEW, ioCls).
		Emit(classfilebuilder.DUP).
		CP(classfilebuilder.INVOKESPECIAL, ioInit).
		Emit(classfilebuilder.ATHROW)

	raw := mustBuildClass(t, c)
	noResolve := func(string) ([]byte, bool) { return nil, false }

	// Strict mode must refuse generating a checked-escape helper without complete parent hierarchy.
	strictRes, err := DecompileWithOptions(raw, DecompileOptions{
		Mode:                 Precision,
		Resolve:              noResolve,
		RetainUnprovedSource: false,
	})
	if err == nil && len(strictRes.StubMethods) == 0 {
		t.Fatalf("strict mode should require complete inherited member names, got complete result")
	}
	if err == nil && (len(strictRes.Diagnostics) == 0 || !strings.Contains(strictRes.Diagnostics[0].Message, "complete inherited member names")) {
		t.Fatalf("unexpected strict diagnostics: %+v", strictRes.Diagnostics)
	}

	// RetainUnprovedSource=true allows checked-escape helper synthesis cleanly.
	res, err := DecompileWithOptions(raw, DecompileOptions{
		Mode:                 Compatibility,
		Resolve:              noResolve,
		RetainUnprovedSource: true,
	})
	if err != nil {
		t.Fatalf("RetainUnprovedSource failed on missing superclass checked escape: %v", err)
	}
	if len(res.StubMethods) != 0 {
		t.Fatalf("expected 0 stub methods, got %v\n%s", res.StubMethods, res.Source)
	}
	if !strings.Contains(res.Source, "jdec$rethrow$") {
		t.Fatalf("expected checked escape helper in source:\n%s", res.Source)
	}
}

func TestAdversarialP1ExceptionSentinelAndNilSlotSafe(t *testing.T) {
	in := strings.Join([]string{
		"}catch(Throwable var21){",
		"	Throwable var17 = Exception;",
		"	if ((var14) != (null)){",
		"		Throwable var18_1 = Exception;",
		"		var21.addSuppressed(var18_1);",
		"		var14.close();",
		"	}",
		"	throw var21;",
		"}",
	}, "\n")
	out := fixLeakedExceptionSentinelCode(in)
	if hasExceptionSentinel(out) {
		t.Fatalf("expected leaked sentinels recovered/cleaned, got:\n%s", out)
	}
	if !strings.Contains(out, "try{") || !strings.Contains(out, "var14.close();") || !strings.Contains(out, "var21.addSuppressed(var18_1);") {
		t.Fatalf("expected try/catch close suppression block, got:\n%s", out)
	}
	if strings.Contains(out, "var17") {
		t.Fatalf("expected dead var17 sentinel stripped, got:\n%s", out)
	}

	// Nil JavaRef and nil-inner SlotValue must never panic on String/Type.
	var nilRef *values.JavaRef
	if got := nilRef.String(nil); got == "" {
		t.Fatal("nil JavaRef String returned empty")
	}
	slot := &values.SlotValue{}
	if got := slot.String(nil); got == "" {
		t.Fatal("nil-val SlotValue String returned empty")
	}
}
