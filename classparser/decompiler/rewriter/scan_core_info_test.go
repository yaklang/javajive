package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestScanCoreInfoDoesNotTreatDiamondMergeAsLoop(t *testing.T) {
	entry := core.NewNode(statements.NewMiddleStatement("start", nil))
	branch := core.NewNode(statements.NewMiddleStatement("fork", nil))
	left := core.NewNode(statements.NewMiddleStatement("left", nil))
	right := core.NewNode(statements.NewMiddleStatement("right", nil))
	merge := core.NewNode(statements.NewMiddleStatement("merge", nil))
	condition := core.NewNode(&statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))})
	ret := core.NewNode(statements.NewReturnStatement(nil))
	otherRet := core.NewNode(statements.NewReturnStatement(nil))
	end := core.NewNode(statements.NewMiddleStatement("end", nil))
	entry.AddNext(branch)
	branch.AddNext(left)
	branch.AddNext(right)
	left.AddNext(merge)
	right.AddNext(merge)
	merge.AddNext(condition)
	condition.AddNext(ret)
	condition.AddNext(otherRet)
	ret.AddNext(end)
	otherRet.AddNext(end)
	for id, node := range []*core.Node{entry, branch, left, right, merge, condition, ret, otherRet, end} {
		node.Id = id
	}

	mgr := NewRootStatementManager(entry)
	if err := mgr.ScanCoreInfo(); err != nil {
		t.Fatal(err)
	}
	for _, loopEntry := range mgr.CircleEntryPoint {
		if loopEntry == merge {
			t.Fatal("diamond merge was incorrectly classified as a loop entry")
		}
	}
}

func TestScanCoreInfoKeepsRealLoopEntry(t *testing.T) {
	entry := core.NewNode(statements.NewMiddleStatement("start", nil))
	condition := core.NewNode(&statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))})
	body := core.NewNode(statements.NewMiddleStatement("body", nil))
	exit := core.NewNode(statements.NewReturnStatement(nil))
	end := core.NewNode(statements.NewMiddleStatement("end", nil))
	entry.AddNext(condition)
	condition.AddNext(body)
	condition.AddNext(exit)
	body.AddNext(condition)
	exit.AddNext(end)

	mgr := NewRootStatementManager(entry)
	if err := mgr.ScanCoreInfo(); err != nil {
		t.Fatal(err)
	}
	for _, loopEntry := range mgr.CircleEntryPoint {
		if loopEntry == condition {
			return
		}
	}
	t.Fatal("real loop header was discarded")
}
