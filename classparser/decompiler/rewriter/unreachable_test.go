package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"testing"
)

func TestNestedSwitchAbruptCompletion(t *testing.T) {
	ret := statements.NewReturnStatement(nil)
	branch := statements.NewIfStatement(values.JavaNull, []statements.Statement{ret}, []statements.Statement{ret})
	sw := statements.NewSwitchStatement(values.JavaNull, []*statements.CaseItem{{IntValue: 1}, {IsDefault: true, Body: []statements.Statement{branch}}})
	if switchCompletesNormally(sw) {
		t.Fatal("grouped labels and two returning branches do not fall through")
	}
	if got := PruneUnreachableStatements([]statements.Statement{sw, ret}); len(got) != 1 {
		t.Fatal("unreachable post-switch statement retained")
	}
	sw.Cases[0].Body = []statements.Statement{statements.NewCustomStatement(func(*class_context.ClassContext) string { return "touch()" }, nil)}
	if switchCompletesNormally(sw) {
		t.Fatal("an effectful fall-through arm reaches the returning default")
	}
	breakStmt := statements.NewCustomStatement(func(*class_context.ClassContext) string { return "break" }, nil)
	sw.Cases[1].Body = []statements.Statement{statements.NewIfStatement(values.JavaNull, []statements.Statement{breakStmt}, nil), ret}
	if !switchCompletesNormally(sw) {
		t.Fatal("conditional break must keep the continuation reachable")
	}
	if got := PruneUnreachableStatements([]statements.Statement{sw, ret}); len(got) != 2 {
		t.Fatal("reachable continuation removed")
	}
	sw.Cases = append(sw.Cases, &statements.CaseItem{IntValue: 2})
	if !switchCompletesNormally(sw) {
		t.Fatal("trailing empty label must fall through")
	}
}
