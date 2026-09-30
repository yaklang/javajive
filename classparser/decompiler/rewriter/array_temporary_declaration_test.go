package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestRestoreSingleDefinitionArrayDeclaration(t *testing.T) {
	for _, kind := range []string{"fresh", "same-name parameter", "parameter", "inline declared", "bare declared", "multiple definitions", "wrong component", "wrong dimension", "non-array local", "indirect value", "named local", "this", "array member", "for initializer", "for update", "catch identity"} {
		t.Run(kind, func(t *testing.T) {
			typ := types.NewJavaArrayType(types.NewJavaPrimer(types.JavaLong))
			ref := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, typ)
			array := values.NewNewArrayExpression(typ, values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))
			assign := statements.NewAssignStatement(ref, array, false)
			root := []statements.Statement{assign}
			var params []*values.JavaRef
			switch kind {
			case "same-name parameter":
				params = []*values.JavaRef{values.NewJavaRef(utils.NewRootVariableId().Next(), nil, typ)}
			case "parameter":
				params = []*values.JavaRef{ref}
			case "inline declared":
				assign.IsFirst = true
			case "bare declared":
				root = append([]statements.Statement{statements.NewDeclareStatement(ref)}, root...)
			case "multiple definitions":
				root = append(root, statements.NewAssignStatement(ref, array, false))
			case "wrong component":
				ref.ResetVarType(types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger)))
			case "wrong dimension":
				ref.ResetVarType(types.NewJavaArrayType(typ))
			case "non-array local":
				ref.ResetVarType(types.NewJavaClass("java.lang.Object"))
			case "indirect value":
				assign.JavaValue = values.NewJavaRef(utils.NewRootVariableId().Next(), array, typ)
			case "named local":
				ref.Id.SetName("items")
			case "this":
				ref.IsThis = true
			case "array member":
				assign.ArrayMember = values.NewJavaArrayMember(ref, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)))
			}
			if kind == "for initializer" {
				root = []statements.Statement{&statements.ForStatement{InitVar: statements.NewAssignStatement(ref, array, true), SubStatements: []statements.Statement{assign}}}
			} else if kind == "for update" {
				root = []statements.Statement{&statements.ForStatement{EndExp: statements.NewAssignStatement(ref, array, false), SubStatements: []statements.Statement{assign}}}
			} else if kind == "catch identity" {
				root = []statements.Statement{&statements.TryCatchStatement{Exception: []*values.JavaRef{ref}, CatchBodies: [][]statements.Statement{{assign}}}}
			}
			restoreSingleDefinitionArrayDeclarations(&root, params)
			want := kind == "fresh" || kind == "same-name parameter" || kind == "inline declared"
			if assign.IsFirst != want {
				t.Fatalf("IsFirst=%v want=%v", assign.IsFirst, want)
			}
			if assign.JavaValue != array && kind != "indirect value" {
				t.Fatal("allocation value changed")
			}
			if root[len(root)-1] != assign && kind != "multiple definitions" && kind != "for initializer" && kind != "for update" && kind != "catch identity" {
				t.Fatal("allocation moved")
			}
			restoreSingleDefinitionArrayDeclarations(&root, params)
			if assign.IsFirst != want {
				t.Fatal("declaration repair is not idempotent")
			}
		})
	}
	restoreSingleDefinitionArrayDeclarations(nil, nil)
}

func TestArrayTemporaryTypeUsesJvmIdentity(t *testing.T) {
	array := func(name string) types.JavaType { return types.NewJavaArrayType(types.NewJavaClass(name)) }
	if !sameArrayTemporaryType(array("one/Value"), array("one.Value")) || sameArrayTemporaryType(array("one.Value"), array("two.Value")) || sameArrayTemporaryType(nil, array("one.Value")) {
		t.Fatal("array type compared display names")
	}
}
