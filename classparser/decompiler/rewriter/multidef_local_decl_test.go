package rewriter

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestMultiDefUndeclaredGeneratedLocalHoistedToLCA(t *testing.T) {
	ctx := &class_context.ClassContext{}
	anchorType := types.NewJavaClass("Anchor")
	intType := types.NewJavaPrimer(types.JavaInteger)
	boolType := types.NewJavaPrimer(types.JavaBoolean)
	render := func(sts []statements.Statement) string {
		var b strings.Builder
		for _, s := range sts {
			b.WriteString(s.String(ctx))
			b.WriteString("\n")
		}
		return b.String()
	}

	t.Run("if_else_multi_def_hoists_to_enclosing_lca_before_if", func(t *testing.T) {
		outerGuard := &statements.IfStatement{
			Condition: values.NewJavaLiteral(true, boolType),
		}
		v := values.NewJavaRef(utils.NewRootVariableId(), nil, anchorType)
		v.Id.SetName("var21")
		branch := &statements.IfStatement{
			Condition: values.NewJavaLiteral(true, boolType),
			IfBody:    []statements.Statement{statements.NewAssignStatement(v, values.JavaNull, false)},
			ElseBody:  []statements.Statement{statements.NewAssignStatement(v, values.JavaNull, false)},
		}
		prelude := statements.NewExpressionStatement(values.NewJavaLiteral("prelude", types.NewJavaClass("java.lang.String")))
		use := statements.NewReturnStatement(v)
		outerGuard.IfBody = []statements.Statement{prelude, branch, use}
		root := []statements.Statement{outerGuard}

		declareUncoveredMultiDefGeneratedLocals(&root)
		out := render(root)
		if strings.Count(out, "Anchor var21 = null") != 1 {
			t.Fatalf("expected exactly 1 hoisted Anchor var21 = null declaration, got:\n%s", out)
		}
		if len(root) != 1 || len(outerGuard.IfBody) != 4 {
			t.Fatalf("expected hoisting into enclosing IfBody at LCA (before branch), got root=%d outerGuard.IfBody=%d:\n%s", len(root), len(outerGuard.IfBody), out)
		}
		if outerGuard.IfBody[0] != prelude {
			t.Fatalf("expected prelude before declaration at firstIdx=1")
		}
		// Idempotent on second pass.
		declareUncoveredMultiDefGeneratedLocals(&root)
		if out2 := render(root); out2 != out {
			t.Fatalf("expected idempotent output:\n%s\n---\n%s", out, out2)
		}
	})

	t.Run("do_while_multi_def_and_escape_hoisted_before_loop", func(t *testing.T) {
		v := values.NewJavaRef(utils.NewRootVariableId(), nil, anchorType)
		v.Id.SetName("var19")
		loop := &statements.DoWhileStatement{
			ConditionValue: values.NewJavaLiteral(false, boolType),
			Body: []statements.Statement{
				&statements.IfStatement{
					Condition: values.NewJavaLiteral(true, boolType),
					IfBody:    []statements.Statement{statements.NewAssignStatement(v, values.JavaNull, false)},
				},
				statements.NewAssignStatement(v, values.JavaNull, false),
			},
		}
		root := []statements.Statement{loop, statements.NewReturnStatement(v)}

		declareUncoveredMultiDefGeneratedLocals(&root)
		out := render(root)
		if strings.Count(out, "Anchor var19 = null") != 1 || len(root) != 3 {
			t.Fatalf("expected hoisted declaration before do/while loop, got len=%d:\n%s", len(root), out)
		}
	})

	t.Run("same_block_sequential_multi_def_promoted_inline", func(t *testing.T) {
		v := values.NewJavaRef(utils.NewRootVariableId(), nil, intType)
		v.Id.SetName("var9_1")
		s1 := statements.NewAssignStatement(v, values.NewJavaLiteral(1, intType), false)
		s2 := statements.NewAssignStatement(v, values.NewJavaLiteral(2, intType), false)
		root := []statements.Statement{s1, s2, statements.NewReturnStatement(v)}

		declareUncoveredMultiDefGeneratedLocals(&root)
		if len(root) != 3 || !s1.IsFirst || s2.IsFirst {
			t.Fatalf("expected inline IsFirst promotion without extra statement, len=%d s1.IsFirst=%v s2.IsFirst=%v", len(root), s1.IsFirst, s2.IsFirst)
		}
		out := render(root)
		if strings.Count(out, "int var9_1") != 1 {
			t.Fatalf("expected single int var9_1 declaration, got:\n%s", out)
		}
	})

	t.Run("conflicting_types_and_named_locals_skipped", func(t *testing.T) {
		conflictID := utils.NewRootVariableId()
		conflictID.SetName("var3")
		namedID := utils.NewRootVariableId()
		namedID.SetName("localName")
		namedRef := values.NewJavaRef(namedID, nil, intType)
		root := []statements.Statement{
			statements.NewAssignStatement(values.NewJavaRef(conflictID, nil, intType), values.NewJavaLiteral(1, intType), false),
			statements.NewAssignStatement(values.NewJavaRef(conflictID, nil, anchorType), values.JavaNull, false),
			statements.NewAssignStatement(namedRef, values.NewJavaLiteral(5, intType), false),
		}
		declareUncoveredMultiDefGeneratedLocals(&root)
		out := render(root)
		if strings.Contains(out, "int var3") || strings.Contains(out, "Anchor var3") || strings.Contains(out, "int localName") {
			t.Fatalf("expected conflicting-type and non-generated locals to be skipped, got:\n%s", out)
		}
	})
}
