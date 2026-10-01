package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Printed slot names are provisional. A store to an enclosing local cannot
// make a different, already covered declaration in either arm escape its scope.
func TestLocalCoveragePreservesSiblingDeclarationIdentities(t *testing.T) {
	for _, sameType := range []bool{false, true} {
		makeRef := func(typ string) *values.JavaRef {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(typ))
			ref.Id.SetName("var7")
			return ref
		}
		outer := makeRef("Lookup")
		if sameType {
			outer = makeRef("java.lang.String")
		}
		left, right := makeRef("java.lang.String"), makeRef("java.lang.String")
		ld := statements.NewAssignStatement(left, values.JavaNull, true)
		rd := statements.NewAssignStatement(right, values.JavaNull, true)
		condition := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
		branch := &statements.IfStatement{
			Condition: condition,
			IfBody: []statements.Statement{
				statements.NewAssignStatement(outer, values.JavaNull, false),
				&statements.IfStatement{Condition: condition, IfBody: []statements.Statement{ld, statements.NewReturnStatement(left)}},
				&statements.IfStatement{Condition: condition, IfBody: []statements.Statement{rd, statements.NewReturnStatement(right)}},
				statements.NewReturnStatement(outer),
			},
		}
		declaration := statements.NewDeclareStatement(outer)
		root := []statements.Statement{declaration, branch}
		coverUndeclaredGeneratedLocals(&root)
		if len(root) != 2 || root[0] != declaration || root[1] != branch || !ld.IsFirst || !rd.IsFirst ||
			len(branch.IfBody) != 4 || len(branch.ElseBody) != 0 {
			t.Fatalf("sameType=%v: same spelling demoted distinct covered declarations", sameType)
		}
		if left.Id == right.Id || left.Id == outer.Id || right.Id == outer.Id {
			t.Fatal("coverage merged distinct declaration identities")
		}
	}
}

func TestLocalCoverageMovesOnlyTheEscapingIdentity(t *testing.T) {
	makeRef := func() *values.JavaRef {
		r := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Payload"))
		r.Id.SetName("var7")
		return r
	}
	local, escaping := makeRef(), makeRef()
	localDecl := statements.NewAssignStatement(local, values.JavaNull, true)
	escapingDecl := statements.NewAssignStatement(escaping, values.JavaNull, true)
	branch := &statements.IfStatement{
		Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)),
		IfBody:    []statements.Statement{localDecl, statements.NewReturnStatement(local)},
		ElseBody:  []statements.Statement{escapingDecl},
	}
	root := []statements.Statement{branch, statements.NewReturnStatement(escaping)}
	coverUndeclaredGeneratedLocals(&root)
	if len(root) != 3 || !localDecl.IsFirst || escapingDecl.IsFirst || root[1] != branch {
		t.Fatal("coverage moved a covered sibling or did not cover the actual escaping identity")
	}
	hoisted, ok := root[0].(*statements.AssignStatement)
	if !ok || !hoisted.IsDeclare || hoisted.LeftValue.(*values.JavaRef).Id != escaping.Id {
		t.Fatal("a declaration of the same printed name was substituted for the escaping identity")
	}
	before := root[0].String(&class_context.ClassContext{})
	coverUndeclaredGeneratedLocals(&root)
	if len(root) != 3 || before != root[0].String(&class_context.ClassContext{}) || !localDecl.IsFirst {
		t.Fatal("identity placement is not idempotent")
	}
}
