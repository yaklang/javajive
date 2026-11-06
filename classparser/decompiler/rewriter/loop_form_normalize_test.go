package rewriter

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func loopTestBreak() *statements.CustomStatement {
	return statements.NewSourceTransferStatement("break", "")
}

func TestNormalizeLoopFormsDoWhileGuardToWhile(t *testing.T) {
	condition := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
	body := statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))
	guard := statements.NewIfStatement(condition,
		[]statements.Statement{body},
		[]statements.Statement{loopTestBreak()})
	loop := statements.NewDoWhileStatement(
		values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)),
		[]statements.Statement{guard})
	out := NormalizeLoopForms([]statements.Statement{loop})
	if len(out) != 1 {
		t.Fatalf("statements = %d", len(out))
	}
	while, ok := out[0].(*statements.WhileStatement)
	if !ok {
		t.Fatalf("loop = %T, want WhileStatement", out[0])
	}
	if len(while.Body) != 1 {
		t.Fatalf("while body = %d", len(while.Body))
	}
}

func TestNormalizeLoopFormsDoesNotTouchLabeledDoWhile(t *testing.T) {
	loop := statements.NewDoWhileStatement(
		values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)),
		[]statements.Statement{statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))})
	loop.Label = "OUTER"
	out := NormalizeLoopForms([]statements.Statement{loop})
	if _, ok := out[0].(*statements.DoWhileStatement); !ok {
		t.Fatalf("labeled loop changed to %T", out[0])
	}
}

func TestNormalizeLoopFormsRecoversForLoop(t *testing.T) {
	intType := types.NewJavaPrimer(types.JavaInteger)
	ref := useTestRef("var30", "int")
	ref.ResetVarType(intType)
	init := statements.NewAssignStatement(ref, values.NewJavaLiteral(0, intType), true)
	condition := values.NewBinaryExpression(ref, values.NewJavaLiteral(10, intType), values.LT,
		types.NewJavaPrimer(types.JavaBoolean))
	update := statements.NewExpressionStatement(values.NewUnaryExpression(ref, values.INC, intType))
	body := statements.NewExpressionStatement(values.NewJavaLiteral(1, intType))
	while := statements.NewWhileStatement(condition, []statements.Statement{body, update})
	out := NormalizeLoopForms([]statements.Statement{init, while})
	if len(out) != 1 {
		t.Fatalf("statements = %d, want one for statement", len(out))
	}
	forSt, ok := out[0].(*statements.ForStatement)
	if !ok {
		t.Fatalf("loop = %T, want ForStatement", out[0])
	}
	if len(forSt.SubStatements) != 1 || forSt.InitVar == nil || forSt.EndExp == nil {
		t.Fatalf("malformed for: %#v", forSt)
	}
	text := forSt.String(&class_context.ClassContext{})
	if !strings.HasPrefix(text, "for(") || strings.Contains(text, "if (") {
		t.Fatalf("for rendering = %s", text)
	}
}

func TestNormalizeLoopFormsKeepsContinueOrEscapingDeclaration(t *testing.T) {
	intType := types.NewJavaPrimer(types.JavaInteger)
	ref := useTestRef("var31", "int")
	ref.ResetVarType(intType)
	init := statements.NewAssignStatement(ref, values.NewJavaLiteral(0, intType), true)
	condition := values.NewBinaryExpression(ref, values.NewJavaLiteral(10, intType), values.LT,
		types.NewJavaPrimer(types.JavaBoolean))
	update := statements.NewExpressionStatement(values.NewUnaryExpression(ref, values.INC, intType))
	continueStmt := statements.NewSourceTransferStatement("continue", "")
	while := statements.NewWhileStatement(condition, []statements.Statement{continueStmt, update})
	out := NormalizeLoopForms([]statements.Statement{init, while})
	if len(out) != 2 {
		t.Fatalf("continue loop was converted to for")
	}

	use := statements.NewExpressionStatement(ref)
	out = NormalizeLoopForms([]statements.Statement{init, while, use})
	if len(out) != 3 {
		t.Fatalf("escaping declaration was converted to for")
	}
}
