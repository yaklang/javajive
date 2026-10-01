package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestCoverageRestoresDeclarationOfExplicitLocalDefinition(t *testing.T) {
	for _, kind := range []string{"primitive", "reference", "multiple arms", "sibling names", "parameter", "catch entry", "existing declaration", "read only", "array element", "field", "escaping read", "self read", "earlier read"} {
		t.Run(kind, func(t *testing.T) {
			var typ types.JavaType = types.NewJavaPrimer(types.JavaDouble)
			if kind == "reference" {
				typ = types.NewJavaClass("java.lang.Object")
			}
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			ref.Id.SetName("var7")
			value := values.NewJavaLiteral(2.5, typ)
			if kind == "reference" {
				value = values.NewJavaLiteral(nil, typ)
			}
			store := statements.NewAssignStatement(ref, value, false)
			use := statements.NewReturnStatement(ref)
			arm := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{store, use}}
			root := []statements.Statement{arm}
			switch kind {
			case "multiple arms":
				arm.ElseBody = []statements.Statement{statements.NewAssignStatement(ref, value, false)}
			case "sibling names":
				other := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
				other.Id.SetName("var7")
				arm.ElseBody = []statements.Statement{statements.NewAssignStatement(other, value, true), statements.NewReturnStatement(other)}
			case "parameter":
				ref.IsParam = true
			case "catch entry":
				root = []statements.Statement{&statements.TryCatchStatement{Exception: []*values.JavaRef{ref}, CatchBodies: [][]statements.Statement{{store, use}}}}
			case "existing declaration":
				root = append([]statements.Statement{statements.NewDeclareStatement(ref)}, root...)
			case "read only":
				root = []statements.Statement{use}
			case "array element":
				store.ArrayMember = values.NewJavaArrayMember(ref, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)))
			case "escaping read":
				root = append(root, use)
			case "self read":
				store.JavaValue = ref
			case "earlier read":
				arm.IfBody = []statements.Statement{use, store}
			case "field":
				store.LeftValue = values.NewRefMember(ref, "field", typ)
			}
			before := len(root)
			coverUndeclaredGeneratedLocals(&root)
			want := kind == "primitive" || kind == "reference" || kind == "sibling names"
			if len(root) != before || store.IsFirst != want {
				t.Fatalf("inline declaration restored=%v want=%v", store.IsFirst, want)
			}
			if want {
				if arm.IfBody[0] != store || store.JavaValue != value || store.IsDeclare {
					t.Fatal("coverage changed the actual definition or inserted an initializer")
				}
				if !topLevelDeclDominatesAllUses(root, ref.Id) {
					t.Fatal("recovered declaration does not cover the exact identity")
				}
			}
			count := len(root)
			coverUndeclaredGeneratedLocals(&root)
			if len(root) != count {
				t.Fatal("declaration recovery is not idempotent")
			}
		})
	}
}
