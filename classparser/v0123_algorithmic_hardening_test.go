package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAnnotationWildcardBoundErasedOnlyWhenRetained(t *testing.T) {
	c := classfilebuilder.NewClass("adv/GenericBoundAnno", "java/lang/Object",
		classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_INTERFACE|classfilebuilder.ACC_ABSTRACT|classfilebuilder.ACC_ANNOTATION)
	m := c.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_ABSTRACT, "value", "()Ljava/lang/Class;")
	c.SetMethodSignature(m, "()Ljava/lang/Class<+Ladv/Box<Ljava/lang/String;>;>;")
	c.SetMethodAnnotationDefault(m, classfilebuilder.AnnotationValue{
		Kind: classfilebuilder.AnnotationClass,
		Text: "Ladv/StringBox;",
	})
	raw := mustBuildClass(t, c)

	strictRes, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
	if err != nil {
		t.Fatalf("strict DecompileWithOptions failed: %v", err)
	}
	if !strings.Contains(strictRes.Source, "Box<String>") {
		t.Fatalf("expected strict mode to keep raw nested wildcard bound Box<String>, got:\n%s", strictRes.Source)
	}

	retainedRes, err := DecompileWithOptions(raw, DecompileOptions{
		Mode:                 Compatibility,
		RetainUnprovedSource: true,
	})
	if err != nil {
		t.Fatalf("retained DecompileWithOptions failed: %v", err)
	}
	if len(retainedRes.StubMethods) != 0 || strings.Contains(retainedRes.Source, DecompileStubMarker) {
		t.Fatalf("expected zero stubs in retained mode, got stubs=%v:\n%s", retainedRes.StubMethods, retainedRes.Source)
	}
	if strings.Contains(retainedRes.Source, "Box<String>") || !strings.Contains(retainedRes.Source, "Class<? extends Box>") {
		t.Fatalf("expected erased wildcard bound `Class<? extends Box>` without `<String>`, got:\n%s", retainedRes.Source)
	}
	if !strings.Contains(retainedRes.Source, "default StringBox.class") {
		t.Fatalf("expected preserved annotation default, got:\n%s", retainedRes.Source)
	}
}

func TestMultiSiteInertObjectInitAllowedOnlyWhenRetained(t *testing.T) {
	c := classfilebuilder.NewClass("adv/MultiSiteObjectInit", "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_SUPER)
	c.AddField(classfilebuilder.ACC_FINAL|classfilebuilder.ACC_PUBLIC, "v", "I")
	objInit, err := c.Pool().Methodref("java/lang/Object", "<init>", "()V")
	if err != nil {
		t.Fatal(err)
	}
	vField, err := c.Pool().Fieldref("adv/MultiSiteObjectInit", "v", "I")
	if err != nil {
		t.Fatal(err)
	}
	code := c.AddMethod(classfilebuilder.ACC_PUBLIC, "<init>", "(I)V").Code()
	elseLbl := code.NewLabel()
	code.
		Load(classfilebuilder.IntLocal, 1).
		Branch(classfilebuilder.IFLE, elseLbl).
		Load(classfilebuilder.RefLocal, 0).
		CP(classfilebuilder.INVOKESPECIAL, objInit).
		Load(classfilebuilder.RefLocal, 0).
		Load(classfilebuilder.IntLocal, 1).
		CP(classfilebuilder.PUTFIELD, vField).
		Emit(classfilebuilder.RETURN).
		Mark(elseLbl).
		Load(classfilebuilder.RefLocal, 0).
		CP(classfilebuilder.INVOKESPECIAL, objInit).
		Load(classfilebuilder.RefLocal, 0).
		Emit(classfilebuilder.ICONST_0).
		CP(classfilebuilder.PUTFIELD, vField).
		Emit(classfilebuilder.RETURN)
	raw := mustBuildClass(t, c)

	strictRes, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
	if err != nil {
		t.Fatalf("strict DecompileWithOptions failed: %v", err)
	}
	if len(strictRes.StubMethods) == 0 || !strings.Contains(strictRes.Source, DecompileStubMarker) {
		t.Fatalf("expected strict mode to fail closed on multi-site Object.<init>(), got:\n%s", strictRes.Source)
	}

	retainedRes, err := DecompileWithOptions(raw, DecompileOptions{
		Mode:                 Compatibility,
		RetainUnprovedSource: true,
	})
	if err != nil {
		t.Fatalf("retained DecompileWithOptions failed: %v", err)
	}
	if len(retainedRes.StubMethods) != 0 || strings.Contains(retainedRes.Source, DecompileStubMarker) {
		t.Fatalf("expected zero stubs in retained mode for inert Object.<init>(), got stubs=%v:\n%s", retainedRes.StubMethods, retainedRes.Source)
	}
	if !strings.Contains(retainedRes.Source, "this.v =") {
		t.Fatalf("expected restored field assignments in retained mode, got:\n%s", retainedRes.Source)
	}
}

func TestPackCarrierStaticFieldArgAndPostSuperSwitch(t *testing.T) {
	t.Run("static_field_arg_and_strict_fail_closed", func(t *testing.T) {
		c := classfilebuilder.NewClass("adv/StaticArgPackCtor", "adv/BaseStaticArgCtor", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_SUPER)
		c.AddField(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_STATIC|classfilebuilder.ACC_FINAL, "CONST_TAG", "Ljava/lang/String;")
		constTagRef, err := c.Pool().Fieldref("adv/StaticArgPackCtor", "CONST_TAG", "Ljava/lang/String;")
		if err != nil {
			t.Fatal(err)
		}
		requireNonNullRef, err := c.Pool().Methodref("java/util/Objects", "requireNonNull", "(Ljava/lang/Object;)Ljava/lang/Object;")
		if err != nil {
			t.Fatal(err)
		}
		clsLit, err := c.Pool().Class("java/lang/String")
		if err != nil {
			t.Fatal(err)
		}
		superRef, err := c.Pool().Methodref("adv/BaseStaticArgCtor", "<init>", "(Ljava/lang/String;Ljava/lang/Object;Ljava/lang/Class;)V")
		if err != nil {
			t.Fatal(err)
		}
		c.AddMethod(classfilebuilder.ACC_PUBLIC, "<init>", "(Ljava/lang/String;Ljava/lang/Object;)V").Code().
			Load(classfilebuilder.RefLocal, 1).
			CP(classfilebuilder.INVOKESTATIC, requireNonNullRef).
			Emit(classfilebuilder.POP).
			Load(classfilebuilder.RefLocal, 2).
			CP(classfilebuilder.INVOKESTATIC, requireNonNullRef).
			Store(classfilebuilder.RefLocal, 3).
			Load(classfilebuilder.RefLocal, 0).
			CP(classfilebuilder.GETSTATIC, constTagRef).
			Load(classfilebuilder.RefLocal, 3).
			LDC(clsLit).
			CP(classfilebuilder.INVOKESPECIAL, superRef).
			Emit(classfilebuilder.RETURN)
		raw := mustBuildClass(t, c)

		strictRes, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
		if err != nil {
			t.Fatalf("strict DecompileWithOptions failed: %v", err)
		}
		if len(strictRes.StubMethods) == 0 || !strings.Contains(strictRes.Source, DecompileStubMarker) {
			t.Fatalf("expected strict mode to fail closed, got:\n%s", strictRes.Source)
		}

		retainedRes, err := DecompileWithOptions(raw, DecompileOptions{
			Mode:                 Compatibility,
			RetainUnprovedSource: true,
		})
		if err != nil {
			t.Fatalf("retained DecompileWithOptions failed: %v", err)
		}
		if len(retainedRes.StubMethods) != 0 || strings.Contains(retainedRes.Source, DecompileStubMarker) {
			t.Fatalf("expected zero stubs for pack carrier with static field arg, got stubs=%v:\n%s", retainedRes.StubMethods, retainedRes.Source)
		}
		if !strings.Contains(retainedRes.Source, "$pack$") || !strings.Contains(retainedRes.Source, "CONST_TAG") {
			t.Fatalf("expected packed carrier helper preserving CONST_TAG static field, got:\n%s", retainedRes.Source)
		}
	})

	t.Run("switch_post_super_field_write_with_this_alias", func(t *testing.T) {
		for _, distinct := range []bool{false, true} {
			name := "uniform"
			if distinct {
				name = "distinct"
			}
			t.Run(name, func(t *testing.T) {
				c := classfilebuilder.NewClass("adv/MergedSwitchCtor_"+name, "adv/BaseMergedCtor", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_SUPER)
				c.AddField(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_FINAL, "tag", "B")
				c.AddField(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_FINAL, "extra", "I")
				tagRef, err := c.Pool().Fieldref("adv/MergedSwitchCtor_"+name, "tag", "B")
				if err != nil {
					t.Fatal(err)
				}
				extraRef, err := c.Pool().Fieldref("adv/MergedSwitchCtor_"+name, "extra", "I")
				if err != nil {
					t.Fatal(err)
				}
				superRef, err := c.Pool().Methodref("adv/BaseMergedCtor", "<init>", "(ILjava/lang/String;)V")
				if err != nil {
					t.Fatal(err)
				}
				s0, _ := c.Pool().String("zero")
				s1, _ := c.Pool().String("one")
				code := c.AddMethod(classfilebuilder.ACC_PUBLIC, "<init>", "(B)V").Code()
				defLbl := code.NewLabel()
				case0Lbl := code.NewLabel()
				case1Lbl := code.NewLabel()
				code.
					Load(classfilebuilder.IntLocal, 1).
					Store(classfilebuilder.IntLocal, 2).
					Load(classfilebuilder.IntLocal, 1).
					TableSwitch(0, defLbl, case0Lbl, case1Lbl).
					Mark(case0Lbl).
					LDC(s0).
					Store(classfilebuilder.RefLocal, 3).
					Load(classfilebuilder.RefLocal, 0).
					Store(classfilebuilder.RefLocal, 4).
					Load(classfilebuilder.RefLocal, 4).
					Emit(classfilebuilder.ICONST_1).
					Load(classfilebuilder.RefLocal, 3).
					CP(classfilebuilder.INVOKESPECIAL, superRef).
					Load(classfilebuilder.RefLocal, 0).
					Load(classfilebuilder.IntLocal, 2).
					CP(classfilebuilder.PUTFIELD, tagRef)
				if distinct {
					code.Load(classfilebuilder.RefLocal, 0).Byte(classfilebuilder.BIPUSH, 11).CP(classfilebuilder.PUTFIELD, extraRef)
				}
				code.Emit(classfilebuilder.RETURN).
					Mark(case1Lbl).
					LDC(s1).
					Store(classfilebuilder.RefLocal, 3).
					Load(classfilebuilder.RefLocal, 0).
					Emit(classfilebuilder.ICONST_2).
					Load(classfilebuilder.RefLocal, 3).
					CP(classfilebuilder.INVOKESPECIAL, superRef).
					Load(classfilebuilder.RefLocal, 0).
					Load(classfilebuilder.IntLocal, 2).
					CP(classfilebuilder.PUTFIELD, tagRef)
				if distinct {
					code.Load(classfilebuilder.RefLocal, 0).Byte(classfilebuilder.BIPUSH, 22).CP(classfilebuilder.PUTFIELD, extraRef)
				}
				code.Emit(classfilebuilder.RETURN).
					Mark(defLbl).
					LDC(s1).
					Store(classfilebuilder.RefLocal, 3).
					Load(classfilebuilder.RefLocal, 0).
					Emit(classfilebuilder.ICONST_3).
					Load(classfilebuilder.RefLocal, 3).
					CP(classfilebuilder.INVOKESPECIAL, superRef).
					Load(classfilebuilder.RefLocal, 0).
					Load(classfilebuilder.IntLocal, 2).
					CP(classfilebuilder.PUTFIELD, tagRef)
				if distinct {
					code.Load(classfilebuilder.RefLocal, 0).Byte(classfilebuilder.BIPUSH, 33).CP(classfilebuilder.PUTFIELD, extraRef)
				}
				code.Emit(classfilebuilder.RETURN)

				raw := mustBuildClass(t, c)
				retainedRes, err := DecompileWithOptions(raw, DecompileOptions{
					Mode:                 Compatibility,
					RetainUnprovedSource: true,
				})
				if err != nil {
					t.Fatalf("retained DecompileWithOptions failed: %v", err)
				}
				if len(retainedRes.StubMethods) != 0 || strings.Contains(retainedRes.Source, DecompileStubMarker) {
					t.Fatalf("expected zero stubs for switch ctor with post-super field write, got stubs=%v:\n%s", retainedRes.StubMethods, retainedRes.Source)
				}
				if !strings.Contains(retainedRes.Source, "$pack$") || !strings.Contains(retainedRes.Source, "this.tag =") {
					t.Fatalf("expected pack carrier and post-super this.tag write, got:\n%s", retainedRes.Source)
				}
				if distinct && !strings.Contains(retainedRes.Source, "this.extra =") {
					t.Fatalf("expected per-arm this.extra write, got:\n%s", retainedRes.Source)
				}
			})
		}
	})

	t.Run("canceled_work_budget_fails_closed", func(t *testing.T) {
		c := classfilebuilder.NewClass("adv/CanceledPackCtor", "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_SUPER)
		initRef, _ := c.Pool().Methodref("java/lang/Object", "<init>", "()V")
		c.AddMethod(classfilebuilder.ACC_PUBLIC, "<init>", "()V").Code().
			Load(classfilebuilder.RefLocal, 0).
			CP(classfilebuilder.INVOKESPECIAL, initRef).
			Emit(classfilebuilder.RETURN)
		raw := mustBuildClass(t, c)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		work := workbudget.New(ctx, workbudget.Limits{})
		if _, err := DecompileWithOptions(raw, DecompileOptions{
			Mode:                 Compatibility,
			RetainUnprovedSource: true,
			Work:                 work,
		}); err == nil {
			t.Fatal("expected canceled Work budget to fail closed")
		}
	})
}
