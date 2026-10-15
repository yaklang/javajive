package rewriter

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestDeclarationInsertionBarriersUseTypedDeclarationsWithoutRenderingValues(t *testing.T) {
	for _, shape := range []string{"root", "bare", "nested", "siblings", "for initializer", "catch", "context name", "nondeclaration", "array store", "field store", "opaque reference", "shared cycle"} {
		t.Run(shape, func(t *testing.T) {
			typ := types.NewJavaArrayType(types.NewJavaClass("p.外部$取值"))
			id := utils.NewRootVariableId().Next()
			id.SetName("var0")
			ref := values.NewJavaRef(id, nil, typ)
			array := values.NewNewArrayExpression(typ, values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))
			assign := statements.NewAssignStatement(ref, array, true)
			root := []statements.Statement{assign}
			ctx := &class_context.ClassContext{}
			renderCalls := 0
			switch shape {
			case "bare":
				assign.JavaValue = nil
			case "nested":
				root = []statements.Statement{&statements.IfStatement{IfBody: root}}
			case "siblings":
				other := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, typ)
				other.Id.SetName("var0")
				root = []statements.Statement{&statements.IfStatement{IfBody: root, ElseBody: []statements.Statement{statements.NewAssignStatement(other, array, true)}}}
			case "for initializer":
				root = []statements.Statement{&statements.ForStatement{InitVar: assign}}
			case "catch":
				root = []statements.Statement{&statements.TryCatchStatement{Exception: []*values.JavaRef{ref}}}
			case "context name":
				ctx.LocalNames = map[*utils.VariableId]string{id: "actualLocal"}
			case "nondeclaration":
				assign.IsFirst = false
				assign.IsDeclare = false
			case "array store":
				assign.ArrayMember = values.NewJavaArrayMember(ref, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)))
			case "field store":
				assign.LeftValue = &values.RefMember{Object: ref, Member: "field"}
			case "opaque reference":
				ref.CustomValue = values.NewCustomValue(func(*class_context.ClassContext) string { renderCalls++; return "var0" }, func() types.JavaType { return typ })
			case "shared cycle":
				block := &statements.IfStatement{IfBody: root}
				block.ElseBody = []statements.Statement{block}
				root = []statements.Statement{block}
			}
			first, declare := assign.IsFirst, assign.IsDeclare
			originalValue := assign.JavaValue
			names, err := DeclarationInsertionBarriers(root, ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			want := shape != "nondeclaration" && shape != "array store" && shape != "field store" && shape != "opaque reference"
			name := "var0"
			if shape == "context name" {
				name = "actualLocal"
			}
			if names[name] != want || len(names) > 1 {
				t.Fatalf("declaration barriers=%v want %q=%v", names, name, want)
			}
			if renderCalls != 0 || assign.IsFirst != first || assign.IsDeclare != declare || assign.JavaValue != originalValue || ref.Id != id {
				t.Fatal("a negative declaration barrier rendered an opaque operand or changed an original identity/value/placement")
			}
		})
	}
}

func TestDeclarationInsertionBarriersPropagateResourceRefusals(t *testing.T) {
	ref := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, types.NewJavaPrimer(types.JavaInteger))
	ref.Id.SetName("var0")
	root := []statements.Statement{statements.NewAssignStatement(ref, values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger)), true)}
	for _, kind := range []string{"work", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			var work *workbudget.Budget
			switch kind {
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if names, err := DeclarationInsertionBarriers(root, nil, work); err == nil || names != nil {
				t.Fatal("partial declaration evidence escaped a resource refusal")
			}
		})
	}
}
