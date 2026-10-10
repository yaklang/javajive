package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestDeclarationIdentityKeepsDefinitionsAndLexicalScope(t *testing.T) {
	for _, nested := range []bool{false, true} {
		ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
		ref.Id.SetName("var7")
		init := values.NewJavaLiteral(3, ref.Type())
		store := statements.NewAssignStatement(ref, init, true)
		foreign := values.NewJavaRef(utils.NewRootVariableId(), nil, ref.Type())
		foreign.Id.SetName("var7")
		other := statements.NewAssignStatement(foreign, init, true)
		bare := statements.NewDeclareStatement(ref)
		root := []statements.Statement{bare, store, other}
		if nested {
			root = []statements.Statement{bare, &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{store, other}}}
		}
		dropDuplicateDeclarations(&root)
		var actual *statements.AssignStatement
		if nested {
			actual = root[1].(*statements.IfStatement).IfBody[0].(*statements.AssignStatement)
		} else {
			actual = root[1].(*statements.AssignStatement)
		}
		if actual.IsFirst || actual.IsDeclare || actual.JavaValue != init || !other.IsFirst {
			t.Fatalf("nested=%v duplicate declaration retained or definition/foreign identity changed", nested)
		}
	}
}

func TestSolvedWebSurvivesNamingAcrossSiblingStores(t *testing.T) {
	typ := types.NewJavaPrimer(types.JavaDouble)
	identity := utils.NewRootVariableId()
	a := values.NewJavaRef(identity, nil, typ)
	a.SolvedWebIdentity = identity
	b := *a
	first := statements.NewAssignStatement(a, values.NewJavaLiteral(2.0, typ), true)
	second := statements.NewAssignStatement(&b, values.NewJavaLiteral(3.0, typ), false)
	foreign := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
	foreign.Id.SetName(identity.String())
	other := statements.NewAssignStatement(foreign, values.NewJavaLiteral(4.0, typ), true)
	root := []statements.Statement{&statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{first}}, second, other}
	RewriteVar(&root, 0, nil)
	if first.LeftValue.(*values.JavaRef).Id != second.LeftValue.(*values.JavaRef).Id {
		t.Fatal("proved web split by lexical naming")
	}
	if other.LeftValue.(*values.JavaRef).Id == second.LeftValue.(*values.JavaRef).Id {
		t.Fatal("same spelling merged an unproved identity")
	}
	if first.JavaValue == nil || second.JavaValue == nil {
		t.Fatal("definition effects dropped")
	}
}

func TestDeclarationFlagsAreLocalToSharedStatementOccurrence(t *testing.T) {
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
	shared := statements.NewAssignStatement(ref, values.NewJavaLiteral(7, ref.Type()), true)
	branch := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{statements.NewDeclareStatement(ref), shared}, ElseBody: []statements.Statement{shared}}
	root := []statements.Statement{branch}
	dropDuplicateDeclarations(&root)
	if branch.IfBody[1].(*statements.AssignStatement).IsFirst || !branch.ElseBody[0].(*statements.AssignStatement).IsFirst {
		t.Fatal("demoting one occurrence changed a sibling declaration")
	}
}
