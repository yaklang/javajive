package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestDeclarationPlacementCoversSingleContainerHead(t *testing.T) {
	for _, shape := range []string{"if", "while", "do while", "switch", "monitor", "for condition", "for update"} {
		t.Run(shape, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			ref.Id.SetName("var7")
			assignment := values.NewAssignmentExpression(ref, values.NewJavaLiteral(11, ref.Type()), 23, nil)
			declaration := statements.NewDeclareStatement(ref)
			store := statements.NewAssignStatement(ref, values.NewJavaLiteral(17, ref.Type()), false)
			body := []statements.Statement{declaration, store}
			var container statements.Statement = &statements.IfStatement{Condition: assignment, IfBody: body}
			switch shape {
			case "while":
				container = &statements.WhileStatement{ConditionValue: assignment, Body: body}
			case "do while":
				container = &statements.DoWhileStatement{ConditionValue: assignment, Body: body}
			case "switch":
				container = &statements.SwitchStatement{Value: assignment, Cases: []*statements.CaseItem{{Body: body}}}
			case "monitor":
				container = &statements.SynchronizedStatement{Argument: assignment, Body: body}
			case "for condition":
				container = &statements.ForStatement{Condition: &statements.ConditionStatement{Condition: assignment}, SubStatements: body}
			case "for update":
				container = &statements.ForStatement{EndExp: statements.NewExpressionStatement(assignment), SubStatements: body}
			}
			root := []statements.Statement{container}
			placeCrossScopeDeclarations(&root, nil, true)
			if len(root) != 2 || root[1] != container || !declaresIdentity(root[0], ref.Id) {
				t.Fatal("a single container's head requires a declaration outside its body")
			}
			children := childStatementLists(container)
			if len(children) == 0 || len(*children[0]) != 1 || (*children[0])[0] != store || store.IsFirst || store.IsDeclare {
				t.Fatal("declaration relocation changed the actual store or kept a shadowing declaration")
			}
			if assignment.Target != ref || assignment.OriginPC != 23 || assignment.Value.(*values.JavaLiteral).Data != 11 {
				t.Fatal("declaration placement changed the head's assignment")
			}
			placeCrossScopeDeclarations(&root, nil, true)
			if len(root) != 2 || !topLevelDeclDominatesAllUses(root, ref.Id) {
				t.Fatal("head declaration placement is not idempotent or does not cover the identity")
			}
		})
	}
}

func TestDeclarationPlacementHeadDoesNotBorrowSiblingIdentity(t *testing.T) {
	makeRef := func() *values.JavaRef {
		ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
		ref.Id.SetName("var7")
		return ref
	}
	outer, inner := makeRef(), makeRef()
	store := statements.NewAssignStatement(inner, values.NewJavaLiteral(17, inner.Type()), true)
	branch := &statements.IfStatement{Condition: outer, IfBody: []statements.Statement{store, statements.NewReturnStatement(inner)}}
	outerDecl := statements.NewDeclareStatement(outer)
	root := []statements.Statement{outerDecl, branch}
	placeCrossScopeDeclarations(&root, nil, true)
	if len(root) != 2 || root[0] != outerDecl || root[1] != branch || !store.IsFirst || len(branch.IfBody) != 2 {
		t.Fatal("same spelling in the control head moved a different, already covered identity")
	}
}
