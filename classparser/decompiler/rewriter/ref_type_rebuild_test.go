package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestRebuildRefDeclarationTypesUsesCommonSupertype(t *testing.T) {
	objectType := types.NewJavaClass("java.lang.Object")
	arrayListType := types.NewJavaClass("java.util.ArrayList")
	collectionType := types.NewJavaClass("java.util.Collection")
	ref := useTestRef("var20", "java.util.ArrayList")
	sts := []statements.Statement{
		statements.NewAssignStatement(ref, values.NewJavaLiteral(nil, arrayListType), true),
		statements.NewAssignStatement(ref, values.NewJavaLiteral(nil, collectionType), false),
		&statements.ReturnStatement{JavaValue: ref},
	}
	RebuildRefDeclarationTypes(&sts, useTestContext(objectType))
	name, ok := types.ClassFQNOf(ref.Type())
	if !ok || name != "java.util.Collection" {
		t.Fatalf("declaration = %v, want Collection", ref.Type())
	}
}

func TestRebuildRefDeclarationTypesRejectsNarrowMemberUse(t *testing.T) {
	objectType := types.NewJavaClass("java.lang.Object")
	arrayListType := types.NewJavaClass("java.util.ArrayList")
	collectionType := types.NewJavaClass("java.util.Collection")
	ref := useTestRef("var21", "java.util.ArrayList")
	call := &values.FunctionCallExpression{
		Object:       ref,
		ClassName:    "java.util.ArrayList",
		FunctionName: "size",
		FuncType: types.NewJavaFuncType("", nil,
			types.NewJavaPrimer(types.JavaInteger)),
	}
	sts := []statements.Statement{
		statements.NewAssignStatement(ref, values.NewJavaLiteral(nil, arrayListType), true),
		statements.NewAssignStatement(ref, values.NewJavaLiteral(nil, collectionType), false),
		statements.NewExpressionStatement(call),
	}
	RebuildRefDeclarationTypes(&sts, useTestContext(objectType))
	name, _ := types.ClassFQNOf(ref.Type())
	if name != "java.util.ArrayList" {
		t.Fatalf("declaration changed despite narrow member use: %v", ref.Type())
	}
}

func TestRebuildRefDeclarationTypesComputesSiblingLUB(t *testing.T) {
	objectType := types.NewJavaClass("java.lang.Object")
	stringType := types.NewJavaClass("java.lang.String")
	integerType := types.NewJavaClass("java.lang.Integer")
	ref := useTestRef("var22", "java.lang.String")
	sts := []statements.Statement{
		statements.NewAssignStatement(ref, values.NewJavaLiteral(nil, stringType), true),
		statements.NewAssignStatement(ref, values.NewJavaLiteral(nil, integerType), false),
		statements.NewExpressionStatement(values.NewOriginalInstanceOf(ref, stringType, 2)),
		&statements.ReturnStatement{JavaValue: ref},
	}
	RebuildRefDeclarationTypes(&sts, useTestContext(objectType))
	name, ok := types.ClassFQNOf(ref.Type())
	if !ok || name == "java.lang.String" || name == "java.lang.Integer" {
		t.Fatalf("sibling definitions did not acquire a common supertype: %v", ref.Type())
	}
}

func TestRebuildRefDeclarationTypesSkipsSolvedWeb(t *testing.T) {
	objectType := types.NewJavaClass("java.lang.Object")
	arrayListType := types.NewJavaClass("java.util.ArrayList")
	collectionType := types.NewJavaClass("java.util.Collection")
	ref := useTestRef("var23", "java.util.ArrayList")
	ref.WebDeclType = arrayListType.Copy()
	sts := []statements.Statement{
		statements.NewAssignStatement(ref, values.NewJavaLiteral(nil, arrayListType), true),
		statements.NewAssignStatement(ref, values.NewJavaLiteral(nil, collectionType), false),
		&statements.ReturnStatement{JavaValue: ref},
	}
	RebuildRefDeclarationTypes(&sts, useTestContext(objectType))
	name, _ := types.ClassFQNOf(ref.Type())
	if name != "java.util.ArrayList" {
		t.Fatalf("solved web was retyped: %v", ref.Type())
	}
}
