package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestLocalTypeFallbackDoesNotBorrowStoresFromAnotherIdentity(t *testing.T) {
	for _, narrowing := range []bool{false, true} {
		makeRef := func(name string) *values.JavaRef {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(name))
			ref.Id.SetName("var7")
			return ref
		}
		concrete, object := makeRef("java.lang.String"), makeRef("java.lang.Object")
		concreteDecl := statements.NewAssignStatement(concrete, values.NewJavaLiteral("initial", concrete.Type()), true)
		objectDecl := statements.NewAssignStatement(object, values.JavaNull, true)
		root := []statements.Statement{concreteDecl, objectDecl}
		if narrowing {
			root = append(root, statements.NewAssignStatement(concrete, values.NewJavaLiteral("later", concrete.Type()), false))
			narrowNullInitObjectDecl(&root)
		} else {
			other := makeRef("java.lang.Object")
			other.Id.SetName("input")
			root = append(root, statements.NewAssignStatement(object, other, false))
			widenConcreteDeclToObject(&root)
		}
		if object.Type().RawType().(*types.JavaClass).Name != "java.lang.Object" || concrete.Type().RawType().(*types.JavaClass).Name != "java.lang.String" {
			t.Fatalf("narrowing=%v: a same-spelled local borrowed another identity's reaching stores", narrowing)
		}
		if len(root) != 3 || root[0] != concreteDecl || root[1] != objectDecl {
			t.Fatal("type recovery changed definition placement")
		}
	}
}

func TestLocalTypeFallbackStillUsesStoresOfItsOwnIdentity(t *testing.T) {
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.String"))
	ref.Id.SetName("var7")
	input := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	input.Id.SetName("input")
	root := []statements.Statement{statements.NewDeclareStatement(ref), statements.NewAssignStatement(ref, input, false)}
	widenConcreteDeclToObject(&root)
	if ref.Type().RawType().(*types.JavaClass).Name != "java.lang.Object" || len(root) != 2 {
		t.Fatal("exact-identity stores no longer recover their declaration type")
	}
}

func TestLocalTypeFallbackDoesNotBorrowAnotherIdentitysDeclaration(t *testing.T) {
	other := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.String"))
	orphan := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	other.Id.SetName("var7")
	orphan.Id.SetName("var7")
	foreignDecl := statements.NewAssignStatement(other, values.NewJavaLiteral("first", other.Type()), true)
	store := statements.NewAssignStatement(orphan, values.NewJavaLiteral("second", other.Type()), false)
	root := []statements.Statement{foreignDecl, store}
	narrowNullInitObjectDecl(&root)
	if len(root) != 3 || root[1] != foreignDecl || root[2] != store || !foreignDecl.IsFirst ||
		orphan.Type().RawType().(*types.JavaClass).Name != "java.lang.String" {
		t.Fatal("a distinct same-spelled declaration prevented recovery of the orphan's own definition")
	}
	declaration, ok := root[0].(*statements.AssignStatement)
	if !ok || !declaration.IsDeclare || declaration.LeftValue.(*values.JavaRef).Id != orphan.Id || orphan.Id == other.Id {
		t.Fatal("type recovery used the foreign declaration identity")
	}
}
