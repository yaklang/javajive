package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestDeclarationPlacementKeepsDistinctScopedIdentities(t *testing.T) {
	for _, pair := range [][2]string{{"java.nio.ByteBuffer", "java.nio.file.Path"}, {"java.lang.String", "java.lang.String"}} {
		t.Run(pair[0]+"/"+pair[1], func(t *testing.T) {
			makeArm := func(typ string) (*values.JavaRef, *statements.AssignStatement, *statements.IfStatement) {
				ref := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, types.NewJavaClass(typ))
				ref.Id.SetName("var7")
				definition := statements.NewAssignStatement(ref, values.NewJavaLiteral(nil, ref.Type()), true)
				arm := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{definition, statements.NewReturnStatement(ref)}}
				return ref, definition, arm
			}
			first, firstDecl, left := makeArm(pair[0])
			second, secondDecl, right := makeArm(pair[1])
			root := []statements.Statement{left, right}
			placeCrossScopeDeclarations(&root, nil, true)
			if len(root) != 2 || root[0] != left || root[1] != right || !firstDecl.IsFirst || !secondDecl.IsFirst {
				t.Fatal("already dominating declarations were hoisted because unrelated ids share a printed name")
			}
			if first.Id == second.Id || first.Id.String() != "var7" || second.Id.String() != "var7" {
				t.Fatal("distinct identities or temporary probe names leaked")
			}
		})
	}
}

func TestDeclarationPlacementStillCoversEscapingIdentity(t *testing.T) {
	ref := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, types.NewJavaClass("java.lang.String"))
	definition := statements.NewAssignStatement(ref, values.NewJavaLiteral(nil, ref.Type()), true)
	branch := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{definition}}
	root := []statements.Statement{branch, statements.NewReturnStatement(ref)}
	placeCrossScopeDeclarations(&root, nil, true)
	if len(root) != 3 || definition.IsFirst || root[0].(*statements.AssignStatement).LeftValue.(*values.JavaRef).Id != ref.Id {
		t.Fatal("actual undominated identity was not given one enclosing declaration")
	}
}

func TestDeclarationDominanceIncludesControlHeads(t *testing.T) {
	for _, shape := range []string{"if", "while"} {
		t.Run(shape, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, types.NewJavaPrimer(types.JavaBoolean))
			body := []statements.Statement{statements.NewAssignStatement(ref, values.NewJavaLiteral(false, ref.Type()), true)}
			var container statements.Statement = &statements.IfStatement{Condition: ref, IfBody: body}
			if shape == "while" {
				container = &statements.WhileStatement{ConditionValue: ref, Body: body}
			}
			if topLevelDeclDominatesAllUses([]statements.Statement{container}, ref.Id) {
				t.Fatal("a body declaration cannot cover the condition evaluated before it")
			}
		})
	}
}

func TestDeclarationDominanceBoundsImplicitScopes(t *testing.T) {
	for _, shape := range []string{"for initializer", "catch entry"} {
		t.Run(shape, func(t *testing.T) {
			var typ types.JavaType = types.NewJavaClass("java.lang.Exception")
			if shape == "for initializer" {
				typ = types.NewJavaPrimer(types.JavaInteger)
			}
			ref := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, typ)
			use := statements.NewReturnStatement(ref)
			var container statements.Statement = &statements.TryCatchStatement{Exception: []*values.JavaRef{ref}, CatchBodies: [][]statements.Statement{{use}}}
			if shape == "for initializer" {
				container = &statements.ForStatement{InitVar: statements.NewAssignStatement(ref, values.NewJavaLiteral(0, ref.Type()), true), Condition: &statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))}, EndExp: statements.NewAssignStatement(ref, values.NewJavaLiteral(1, ref.Type()), false), SubStatements: []statements.Statement{use}}
			}
			if !topLevelDeclDominatesAllUses([]statements.Statement{container}, ref.Id) {
				t.Fatal("entry definition did not cover its own child scope")
			}
			if topLevelDeclDominatesAllUses([]statements.Statement{container, use}, ref.Id) {
				t.Fatal("child entry definition escaped into a following sibling")
			}
		})
	}
}
