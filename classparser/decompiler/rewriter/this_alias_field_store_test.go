package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func newCtorTestRef(name string, typ types.JavaType, isThis bool) *values.JavaRef {
	id := utils.NewRootVariableId()
	id.SetName(name)
	ref := values.NewJavaRef(id, nil, typ)
	ref.IsThis = isThis
	return ref
}

func TestRewriteThisAliasFieldStoresInConstructor(t *testing.T) {
	selfTyp := types.NewJavaClass("pkg.Box")
	otherTyp := types.NewJavaClass("pkg.Other")
	superTyp := types.NewJavaClass("pkg.Base")
	intTyp := types.NewJavaPrimer(types.JavaInteger)
	ctx := &class_context.ClassContext{
		PackageName:  "pkg",
		ClassName:    "pkg.Box",
		FunctionName: "<init>",
	}

	t.Run("rewrites_direct_and_chained_alias_stores_including_nested_try_and_if", func(t *testing.T) {
		thisRef := newCtorTestRef("this", selfTyp, true)
		var4 := newCtorTestRef("var4", selfTyp, false)
		var5 := newCtorTestRef("var5", selfTyp, false)

		aliasDecl := statements.NewAssignStatement(var4, thisRef, true)
		chainedDecl := statements.NewAssignStatement(var5, var4, true)

		store1 := statements.NewAssignStatement(
			values.NewRefMember(var4, "mFinal", intTyp),
			values.NewJavaLiteral(1, intTyp),
			false,
		)
		store2 := statements.NewAssignStatement(
			values.NewRefMember(var5, "mNested", intTyp),
			values.NewJavaLiteral(2, intTyp),
			false,
		)
		callUse := statements.NewExpressionStatement(&values.FunctionCallExpression{
			ClassName:    "pkg.Box",
			FunctionName: "helper",
			Object:       var4,
		})

		tryStmt := &statements.TryCatchStatement{
			TryBody: []statements.Statement{
				&statements.IfStatement{
					Condition: values.NewJavaLiteral(1, intTyp),
					IfBody:    []statements.Statement{store2},
				},
			},
		}
		sts := []statements.Statement{aliasDecl, chainedDecl, store1, tryStmt, callUse}
		RewriteThisAliasFieldStores(&sts, ctx)

		if got := store1.String(ctx); got != "this.mFinal = 1" {
			t.Fatalf("store1 = %q, want `this.mFinal = 1`", got)
		}
		if got := store2.String(ctx); got != "this.mNested = 2" {
			t.Fatalf("store2 = %q, want `this.mNested = 2`", got)
		}
		// Alias declaration and non-store argument/receiver uses must remain intact.
		if got := aliasDecl.String(ctx); got != "Box var4 = this" {
			t.Fatalf("aliasDecl mutated unexpectedly: %q", got)
		}
		if got := callUse.String(ctx); got != "var4.helper()" {
			t.Fatalf("callUse mutated unexpectedly: %q", got)
		}
	})

	t.Run("skips_reassigned_local", func(t *testing.T) {
		thisRef := newCtorTestRef("this", selfTyp, true)
		otherRef := newCtorTestRef("other", selfTyp, false)
		var4 := newCtorTestRef("var4", selfTyp, false)

		aliasDecl := statements.NewAssignStatement(var4, thisRef, true)
		reassign := statements.NewAssignStatement(var4, otherRef, false)
		store := statements.NewAssignStatement(
			values.NewRefMember(var4, "mField", intTyp),
			values.NewJavaLiteral(3, intTyp),
			false,
		)
		sts := []statements.Statement{aliasDecl, reassign, store}
		RewriteThisAliasFieldStores(&sts, ctx)

		if got := store.String(ctx); got != "var4.mField = 3" {
			t.Fatalf("reassigned local store should not be rewritten, got %q", got)
		}
	})

	t.Run("skips_superclass_or_other_typed_alias_and_non_init_methods", func(t *testing.T) {
		thisRef := newCtorTestRef("this", selfTyp, true)
		superVar := newCtorTestRef("varBase", superTyp, false)
		otherVar := newCtorTestRef("varOther", otherTyp, false)

		superDecl := statements.NewAssignStatement(superVar, thisRef, true)
		otherDecl := statements.NewAssignStatement(otherVar, thisRef, true)
		superStore := statements.NewAssignStatement(
			values.NewRefMember(superVar, "mBase", intTyp),
			values.NewJavaLiteral(4, intTyp),
			false,
		)
		otherStore := statements.NewAssignStatement(
			values.NewRefMember(otherVar, "mOther", intTyp),
			values.NewJavaLiteral(5, intTyp),
			false,
		)
		sts := []statements.Statement{superDecl, otherDecl, superStore, otherStore}
		RewriteThisAliasFieldStores(&sts, ctx)
		if got := superStore.String(ctx); got != "varBase.mBase = 4" {
			t.Fatalf("super-typed alias store rewritten unexpectedly: %q", got)
		}
		if got := otherStore.String(ctx); got != "varOther.mOther = 5" {
			t.Fatalf("other-typed alias store rewritten unexpectedly: %q", got)
		}

		// Non-<init> method with same-class local alias must not be rewritten.
		nonInitCtx := &class_context.ClassContext{PackageName: "pkg", ClassName: "pkg.Box", FunctionName: "reset"}
		var4 := newCtorTestRef("var4", selfTyp, false)
		d := statements.NewAssignStatement(var4, thisRef, true)
		s := statements.NewAssignStatement(values.NewRefMember(var4, "x", intTyp), values.NewJavaLiteral(6, intTyp), false)
		nonInitSts := []statements.Statement{d, s}
		RewriteThisAliasFieldStores(&nonInitSts, nonInitCtx)
		if got := s.String(nonInitCtx); got != "var4.x = 6" {
			t.Fatalf("non-<init> method store rewritten unexpectedly: %q", got)
		}
	})

	t.Run("kill_switch_disables_rewrite", func(t *testing.T) {
		t.Setenv("JDEC_THIS_ALIAS_FIELD_STORE_OFF", "1")
		thisRef := newCtorTestRef("this", selfTyp, true)
		var4 := newCtorTestRef("var4", selfTyp, false)
		d := statements.NewAssignStatement(var4, thisRef, true)
		s := statements.NewAssignStatement(values.NewRefMember(var4, "x", intTyp), values.NewJavaLiteral(7, intTyp), false)
		sts := []statements.Statement{d, s}
		RewriteThisAliasFieldStores(&sts, ctx)
		if got := s.String(ctx); got != "var4.x = 7" {
			t.Fatalf("kill-switch did not preserve original store: %q", got)
		}
	})
}
