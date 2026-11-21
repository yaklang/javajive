package javaclassparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
)

func mustBuildClass(t *testing.T, c *classfilebuilder.Class) []byte {
	t.Helper()
	raw, err := c.Bytes()
	if err != nil {
		t.Fatalf("classfilebuilder.Bytes: %v", err)
	}
	return raw
}

func decompileShadowCompat(t *testing.T, raw []byte) string {
	t.Helper()
	res, err := DecompileWithOptions(raw, DecompileOptions{
		Mode:           Compatibility,
		EnableShadowIR: true,
	})
	if err != nil {
		t.Fatalf("DecompileWithOptions(ShadowIR): %v", err)
	}
	if res.Source == "" {
		t.Fatalf("empty decompiled source (status=%s)", res.Status)
	}
	return res.Source
}

func TestAdversarialT4bPhiConvergentAndLiveStore(t *testing.T) {
	c := classfilebuilder.NewClass("adv/T4bProbe", "java/lang/Object", 0x0021)
	pool := c.Pool()
	arrayListInit, _ := pool.Methodref("java/util/ArrayList", "<init>", "()V")
	arrayListCls, _ := pool.Class("java/util/ArrayList")
	listClear, _ := pool.InterfaceMethodref("java/util/List", "clear", "()V")
	unmodList, _ := pool.Methodref("java/util/Collections", "unmodifiableList", "(Ljava/util/List;)Ljava/util/List;")
	valueOf, _ := pool.Methodref("java/lang/String", "valueOf", "(Ljava/lang/Object;)Ljava/lang/String;")
	lineSep, _ := pool.Methodref("java/lang/System", "lineSeparator", "()Ljava/lang/String;")
	fallbackLit, _ := pool.String("fallback")

	// Method 1: LocaleUtils-style intermediate store split along a subtype branch:
	// static List buildList(List list, boolean flag)
	m1 := c.AddMethod(0x0009, "buildList", "(Ljava/util/List;Z)Ljava/util/List;")
	lUnmod := m1.Code().NewLabel()
	lRet1 := m1.Code().NewLabel()
	m1.Code().
		Load(classfilebuilder.RefLocal, 0).
		Branch(classfilebuilder.IFNONNULL, lRet1).
		CP(classfilebuilder.NEW, arrayListCls).
		Emit(classfilebuilder.DUP).
		CP(classfilebuilder.INVOKESPECIAL, arrayListInit).
		Store(classfilebuilder.RefLocal, 0).
		Load(classfilebuilder.IntLocal, 1).
		Branch(classfilebuilder.IFEQ, lUnmod).
		Load(classfilebuilder.RefLocal, 0).
		InvokeInterface(listClear).
		Mark(lUnmod).
		Load(classfilebuilder.RefLocal, 0).
		CP(classfilebuilder.INVOKESTATIC, unmodList).
		Store(classfilebuilder.RefLocal, 0).
		Mark(lRet1).
		Load(classfilebuilder.RefLocal, 0).
		Emit(classfilebuilder.ARETURN)

	// Method 2: Live-across-branch single-use store guard:
	// String res = String.valueOf(in); if (in == null) { System.lineSeparator(); res = "fallback"; } return res;
	m2 := c.AddMethod(0x0009, "fallback", "(Ljava/lang/Object;)Ljava/lang/String;")
	lRet2 := m2.Code().NewLabel()
	m2.Code().
		Load(classfilebuilder.RefLocal, 0).
		CP(classfilebuilder.INVOKESTATIC, valueOf).
		Store(classfilebuilder.RefLocal, 1).
		Load(classfilebuilder.RefLocal, 0).
		Branch(classfilebuilder.IFNONNULL, lRet2).
		CP(classfilebuilder.INVOKESTATIC, lineSep).
		Emit(classfilebuilder.POP).
		LDC(fallbackLit).
		Store(classfilebuilder.RefLocal, 1).
		Mark(lRet2).
		Load(classfilebuilder.RefLocal, 1).
		Emit(classfilebuilder.ARETURN)

	raw := mustBuildClass(t, c)
	src := decompileShadowCompat(t, raw)
	if !strings.Contains(src, "unmodifiableList") {
		t.Fatalf("missing unmodifiableList in buildList:\n%s", src)
	}
	if !strings.Contains(src, "String.valueOf") || !strings.Contains(src, "\"fallback\"") {
		t.Fatalf("live-across-branch single-use store was dropped:\n%s", src)
	}

	// Exercise kill switches cleanly without panic or failure.
	for _, env := range []string{"JDEC_T4B_PHI_MERGE_OFF", "JDEC_T4B_LIVE_STORE_FOLD_OFF"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv(env, "1")
			if out, err := Decompile(raw); err != nil || out == "" {
				t.Fatalf("Decompile with %s=1 failed: err=%v", env, err)
			}
		})
	}
}

func TestAdversarialThisAliasSuperCall(t *testing.T) {
	c := classfilebuilder.NewClass("adv/SubView", "adv/BaseView", 0x0021)
	pool := c.Pool()
	baseOnCreate, _ := pool.Methodref("adv/BaseView", "onCreate", "(Ljava/lang/Object;)V")
	helperMethod, _ := pool.Methodref("adv/SubView", "helper", "()V")

	c.AddMethod(0x0002, "helper", "()V").Code().Emit(classfilebuilder.RETURN)

	// Mimic d8 pattern: aload_0; astore_2; aload_2; aload_1; invokespecial Base.onCreate; aload_2; invokespecial Sub.helper; return
	m := c.AddMethod(0x0001, "onCreate", "(Ljava/lang/Object;)V")
	m.Code().
		Load(classfilebuilder.RefLocal, 0).
		Store(classfilebuilder.RefLocal, 2).
		Load(classfilebuilder.RefLocal, 2).
		Load(classfilebuilder.RefLocal, 1).
		CP(classfilebuilder.INVOKESPECIAL, baseOnCreate).
		Load(classfilebuilder.RefLocal, 2).
		CP(classfilebuilder.INVOKESPECIAL, helperMethod).
		Emit(classfilebuilder.RETURN)

	raw := mustBuildClass(t, c)
	src := decompileShadowCompat(t, raw)
	if !strings.Contains(src, "super.onCreate(") {
		t.Fatalf("expected super.onCreate(...), got:\n%s", src)
	}
	if strings.Contains(src, "var2.onCreate(") {
		t.Fatalf("emitted infinite self/alias call var2.onCreate instead of super.onCreate:\n%s", src)
	}

	t.Setenv("JDEC_THIS_ALIAS_SUPER_OFF", "1")
	offSrc, err := Decompile(raw)
	if err != nil {
		t.Fatalf("Decompile with JDEC_THIS_ALIAS_SUPER_OFF=1 failed: %v", err)
	}
	if strings.Contains(offSrc, "super.onCreate(") {
		t.Fatalf("JDEC_THIS_ALIAS_SUPER_OFF=1 did not disable super rewrite:\n%s", offSrc)
	}
}

func TestAdversarialConstructorIrreducibleDispatch(t *testing.T) {
	t.Run("post_super_irreducible_succeeds", func(t *testing.T) {
		c := classfilebuilder.NewClass("adv/PostSuperIrred", "java/lang/Object", 0x0021)
		objInit, _ := c.Pool().Methodref("java/lang/Object", "<init>", "()V")
		ctor := c.AddMethod(0x0001, "<init>", "(I)V")
		l1 := ctor.Code().NewLabel()
		l2 := ctor.Code().NewLabel()
		lRet := ctor.Code().NewLabel()
		ctor.Code().
			Load(classfilebuilder.RefLocal, 0).
			CP(classfilebuilder.INVOKESPECIAL, objInit).
			Load(classfilebuilder.IntLocal, 1).
			Branch(classfilebuilder.IFEQ, l2).
			Mark(l1).
			Load(classfilebuilder.IntLocal, 1).
			Branch(classfilebuilder.IFLE, lRet).
			IInc(1, -1).
			Mark(l2).
			Load(classfilebuilder.IntLocal, 1).
			Branch(classfilebuilder.IFGE, l1).
			Mark(lRet).
			Emit(classfilebuilder.RETURN)

		raw := mustBuildClass(t, c)
		src := decompileShadowCompat(t, raw)
		superIdx := strings.Index(src, "super();")
		switchIdx := strings.Index(src, "switch (")
		if switchIdx < 0 || strings.Contains(src, "undecompilable method body") {
			t.Fatalf("expected post-super irreducible dispatch recovery:\n%s", src)
		}
		if superIdx < 0 || superIdx > switchIdx {
			t.Fatalf("super() must precede irreducible switch:\n%s", src)
		}
	})

	t.Run("pre_super_irreducible_still_rejected", func(t *testing.T) {
		c := classfilebuilder.NewClass("adv/PreSuperIrred", "java/lang/Object", 0x0021)
		objInit, _ := c.Pool().Methodref("java/lang/Object", "<init>", "()V")
		ctor := c.AddMethod(0x0001, "<init>", "(I)V")
		l1 := ctor.Code().NewLabel()
		l2 := ctor.Code().NewLabel()
		lSuper := ctor.Code().NewLabel()
		// Irreducible cycle BEFORE super() (while local0 is still uninitializedThis!)
		ctor.Code().
			Load(classfilebuilder.IntLocal, 1).
			Branch(classfilebuilder.IFEQ, l2).
			Mark(l1).
			Load(classfilebuilder.IntLocal, 1).
			Branch(classfilebuilder.IFLE, lSuper).
			IInc(1, -1).
			Mark(l2).
			Load(classfilebuilder.IntLocal, 1).
			Branch(classfilebuilder.IFGE, l1).
			Mark(lSuper).
			Load(classfilebuilder.RefLocal, 0).
			CP(classfilebuilder.INVOKESPECIAL, objInit).
			Emit(classfilebuilder.RETURN)

		raw := mustBuildClass(t, c)
		res, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
		if err == nil && !strings.Contains(res.Source, "constructor state dispatch is unsupported") {
			t.Fatalf("expected pre-super irreducible refusal, got err=%v src=%s", err, res.Source)
		}
	})
}

func TestAdversarialLoopNormalizationWithConstPrefixAndAssign(t *testing.T) {
	c := classfilebuilder.NewClass("adv/LoopNorm", "java/lang/Object", 0x0021)
	m1 := c.AddMethod(0x0009, "sumStep", "(I)I")
	lCond1 := m1.Code().NewLabel()
	lBody1 := m1.Code().NewLabel()
	// sum=0(local1), i=0(local2); goto cond; body: sum += i; i = i + n; cond: if (i < 30) goto body; return sum
	m1.Code().
		Emit(classfilebuilder.ICONST_0).Store(classfilebuilder.IntLocal, 1).
		Emit(classfilebuilder.ICONST_0).Store(classfilebuilder.IntLocal, 2).
		Branch(classfilebuilder.GOTO, lCond1).
		Mark(lBody1).
		Load(classfilebuilder.IntLocal, 1).
		Load(classfilebuilder.IntLocal, 2).
		Emit(classfilebuilder.IADD).
		Store(classfilebuilder.IntLocal, 1).
		Load(classfilebuilder.IntLocal, 2).
		Load(classfilebuilder.IntLocal, 0).
		Emit(classfilebuilder.IADD).
		Store(classfilebuilder.IntLocal, 2).
		Mark(lCond1).
		Load(classfilebuilder.IntLocal, 2).
		Byte(classfilebuilder.BIPUSH, 30).
		Branch(classfilebuilder.IF_ICMPLT, lBody1).
		Load(classfilebuilder.IntLocal, 1).
		Emit(classfilebuilder.IRETURN)

	// Top-tested loop where limit = 30 is materialized before the condition and also read in body
	// so limit stays an un-inlined single-assignment constant prefix inside do { limit = 30; if (i < limit) ... }.
	m2 := c.AddMethod(0x0009, "sumWithConstPrefix", "(I)I")
	lHead2 := m2.Code().NewLabel()
	lExit2 := m2.Code().NewLabel()
	m2.Code().
		Emit(classfilebuilder.ICONST_0).Store(classfilebuilder.IntLocal, 1).
		Emit(classfilebuilder.ICONST_0).Store(classfilebuilder.IntLocal, 2).
		Mark(lHead2).
		Byte(classfilebuilder.BIPUSH, 30).
		Store(classfilebuilder.IntLocal, 3).
		Load(classfilebuilder.IntLocal, 2).
		Load(classfilebuilder.IntLocal, 3).
		Branch(classfilebuilder.IF_ICMPGE, lExit2).
		Load(classfilebuilder.IntLocal, 1).
		Load(classfilebuilder.IntLocal, 3).
		Emit(classfilebuilder.IADD).
		Store(classfilebuilder.IntLocal, 1).
		Load(classfilebuilder.IntLocal, 2).
		Load(classfilebuilder.IntLocal, 0).
		Emit(classfilebuilder.IADD).
		Store(classfilebuilder.IntLocal, 2).
		Branch(classfilebuilder.GOTO, lHead2).
		Mark(lExit2).
		Load(classfilebuilder.IntLocal, 1).
		Emit(classfilebuilder.IRETURN)

	raw := mustBuildClass(t, c)
	src := decompileShadowCompat(t, raw)
	if !strings.Contains(src, "for(") && !strings.Contains(src, "while (") && !strings.Contains(src, "while(") {
		t.Fatalf("expected normalized for/while loop:\n%s", src)
	}
	if strings.Contains(src, "while (true)") || strings.Contains(src, "do {") || strings.Contains(src, "do{") {
		t.Fatalf("loop remained un-normalized do/while(true):\n%s", src)
	}
}

func TestAdversarialR8RetainUnprovedSourceFallbacks(t *testing.T) {
	t.Run("obscured_static_owner", func(t *testing.T) {
		c := classfilebuilder.NewClass("p/p", "java/lang/Object", 0x0021)
		c.SetClassSignature("<Other:Ljava/lang/Object;>Ljava/lang/Object;")
		fieldRef, _ := c.Pool().Fieldref("p/Other", "VAL", "I")
		m := c.AddMethod(0x0009, "read", "()I")
		m.Code().CP(classfilebuilder.GETSTATIC, fieldRef).Emit(classfilebuilder.IRETURN)

		raw := mustBuildClass(t, c)
		if _, err := Decompile(raw); err == nil || !strings.Contains(err.Error(), "obscured") {
			t.Fatalf("strict mode should fail on obscured static owner, got %v", err)
		}
		res, err := DecompileWithOptions(raw, DecompileOptions{Mode: Compatibility, RetainUnprovedSource: true})
		if err != nil {
			t.Fatalf("RetainUnprovedSource failed: %v", err)
		}
		if !strings.Contains(res.Source, "p.Other.VAL") {
			t.Fatalf("expected p.Other.VAL in retained source:\n%s", res.Source)
		}
	})

	t.Run("enum_without_values_backing_array", func(t *testing.T) {
		c := classfilebuilder.NewClass("r8/MiniEnum", "java/lang/Enum", 0x4031)
		enumInit, _ := c.Pool().Methodref("java/lang/Enum", "<init>", "(Ljava/lang/String;I)V")
		selfCls, _ := c.Pool().Class("r8/MiniEnum")
		aField, _ := c.Pool().Fieldref("r8/MiniEnum", "A", "Lr8/MiniEnum;")
		aName, _ := c.Pool().String("A")
		c.AddField(0x4019, "A", "Lr8/MiniEnum;")
		clinit := c.AddMethod(0x0008, "<clinit>", "()V")
		clinit.Code().
			CP(classfilebuilder.NEW, selfCls).
			Emit(classfilebuilder.DUP).
			LDC(aName).
			Emit(classfilebuilder.ICONST_0).
			CP(classfilebuilder.INVOKESPECIAL, enumInit).
			CP(classfilebuilder.PUTSTATIC, aField).
			Emit(classfilebuilder.RETURN)

		raw := mustBuildClass(t, c)
		if _, err := Decompile(raw); err == nil || !strings.Contains(err.Error(), "enum regeneration") {
			t.Fatalf("strict mode should fail on enum without $VALUES, got %v", err)
		}
		res, err := DecompileWithOptions(raw, DecompileOptions{Mode: Compatibility, RetainUnprovedSource: true})
		if err != nil {
			t.Fatalf("RetainUnprovedSource failed on R8 enum: %v", err)
		}
		if !strings.Contains(res.Source, "class MiniEnum") {
			t.Fatalf("expected class fallback rendering for unproved enum:\n%s", res.Source)
		}
	})

	t.Run("unproved_enclosing_method_type_scope", func(t *testing.T) {
		c := classfilebuilder.NewClass("r8/Encl$1", "java/lang/Object", 0x0021)
		c.SetEnclosingMethod(classfilebuilder.EnclosingMethodSpec{
			Class: "Lr8/Encl;", Method: "make", Proto: "()V",
		})
		f := c.AddField(0x0001, "val", "Ljava/lang/Object;")
		c.SetFieldSignature(f, "TT;")

		raw := mustBuildClass(t, c)
		noResolve := func(string) ([]byte, bool) { return nil, false }
		if _, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, Resolve: noResolve}); err == nil || !strings.Contains(err.Error(), "unproven original enclosing method type scope") {
			t.Fatalf("strict mode should reject unproven enclosing method scope, got %v", err)
		}
		res, err := DecompileWithOptions(raw, DecompileOptions{
			Mode:                 Compatibility,
			Resolve:              noResolve,
			RetainUnprovedSource: true,
		})
		if err != nil {
			t.Fatalf("RetainUnprovedSource failed on unproved enclosing method scope: %v", err)
		}
		if !strings.Contains(res.Source, "class Encl$1") {
			t.Fatalf("unexpected retained source:\n%s", res.Source)
		}
	})
}
