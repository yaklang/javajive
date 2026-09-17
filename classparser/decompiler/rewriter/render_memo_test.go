package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"testing"
)

func TestDeclarationProbeMemoEpoch(t *testing.T) {
	text, calls := "var2 + var20", 0
	st := statements.NewCustomStatement(func(*class_context.ClassContext) string { calls++; return text }, func(*utils.VariableId, *utils.VariableId) {})
	list := []statements.Statement{st}
	memo := stmtRenderMemo{}
	for i := 0; i < 100; i++ {
		if !statementsReferenceName(list, "var2", memo) || statementsReferenceName(list, "var3", memo) {
			t.Fatal("token identity lost")
		}
	}
	if calls != 1 {
		t.Fatalf("same subtree rendered %d times", calls)
	}
	text = "var3"
	memo.invalidate()
	if statementsReferenceName(list, "var2", memo) || !statementsReferenceName(list, "var3", memo) || calls != 2 {
		t.Fatal("tree mutation reused stale render")
	}
}
