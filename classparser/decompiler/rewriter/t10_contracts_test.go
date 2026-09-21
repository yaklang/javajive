package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func t10ExceptionAssign(name string) statements.Statement {
	excType := types.JavaType(types.NewJavaClass("Throwable"))
	ref := values.NewJavaRef(nil, nil, excType)
	ref.CustomValue = values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
		return name
	}, func() types.JavaType { return excType })
	placeholder := values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
		return "Exception"
	}, func() types.JavaType { return excType })
	placeholder.Flag = "exception"
	return statements.NewAssignStatement(ref, placeholder, true)
}

func TestT10_C04_TryRewriterKeepsBothHandlers(t *testing.T) {
	t.Run("T10-C04", testT10C04TryRewriter)
}
func testT10C04TryRewriter(t *testing.T) {
	tryBody := core.NewNode(statements.NewExpressionStatement(values.NewJavaLiteral("try", types.NewJavaClass("String"))))
	catch1 := core.NewNode(t10ExceptionAssign("e1"))
	catch2 := core.NewNode(t10ExceptionAssign("e2"))
	tail := core.NewNode(statements.NewReturnStatement(nil))
	try := core.NewNode(statements.NewMiddleStatement(statements.MiddleTryStart, nil))
	try.Id = 1
	tryBody.Id = 2
	catch1.Id = 3
	catch2.Id = 4
	tail.Id = 5
	catch1.IsCatchStart = true
	catch2.IsCatchStart = true
	entry := core.NewNode(statements.NewMiddleStatement("start", nil))
	entry.Id = 0
	entry.AddNext(try)
	try.AddNext(tryBody)
	try.AddNext(catch1)
	try.AddNext(catch2)
	tryBody.AddNext(tail)
	catch1.AddNext(tail)
	catch2.AddNext(tail)
	mgr := NewRootStatementManager(entry)
	mgr.DominatorMap = GenerateDominatorTree(try)
	if err := TryRewriter(mgr, try); err != nil {
		t.Fatal(err)
	}
	var tc *statements.TryCatchStatement
	core.WalkGraph[*core.Node](entry, func(n *core.Node) ([]*core.Node, error) {
		if st, ok := n.Statement.(*statements.TryCatchStatement); ok {
			tc = st
		}
		return n.Next, nil
	})
	if tc == nil {
		t.Fatal("try rewriter dropped the try")
	}
	if len(tc.CatchBodies) != 2 {
		t.Fatalf("shared-handler catch bodies dropped: got %d", len(tc.CatchBodies))
	}
}

func TestT10_C05_MonitorRewriterDoesNotInventExit(t *testing.T) {
	t.Run("T10-C05", testT10C05NoInventedExit)
}
func testT10C05NoInventedExit(t *testing.T) {
	body := []statements.Statement{
		statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))),
	}
	out, found := removeSunkMonitorExit(body)
	if found {
		t.Fatal("invented monitorexit for a body that never acquired a lock")
	}
	if len(out) != 1 {
		t.Fatalf("body rewritten unexpectedly: %d", len(out))
	}
	locked := []statements.Statement{
		statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))),
		statements.NewMiddleStatement("monitor_exit", nil),
	}
	stripped, found := removeSunkMonitorExit(locked)
	if !found || len(stripped) != 1 {
		t.Fatalf("failed to strip a real sunk monitorexit: found=%v n=%d", found, len(stripped))
	}
}

func TestT10_C02_ThrowOnlySynchronizedKeepsBody(t *testing.T) {
	t.Run("T10-C02", testT10C02ThrowOnlySync)
}
func testT10C02ThrowOnlySync(t *testing.T) {
	lock := values.NewJavaLiteral("LOCK", types.NewJavaClass("Object"))
	entry := core.NewNode(statements.NewMiddleStatement("start", nil))
	enter := core.NewNode(statements.NewMiddleStatement("monitor_enter", lock))
	throwSt := statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
		return "throw new IllegalStateException(\"lock\")"
	}, func(oldId, newId *utils.VariableId) {})
	exitSt := statements.NewMiddleStatement("monitor_exit", nil)
	trySt := statements.NewTryCatchStatement([]statements.Statement{throwSt}, [][]statements.Statement{{exitSt}})
	tryNode := core.NewNode(trySt)
	entry.AddNext(enter)
	enter.AddNext(tryNode)
	end := core.NewNode(statements.NewMiddleStatement("end", nil))
	tryNode.AddNext(end)
	mgr := NewRootStatementManager(entry)
	if err := SynchronizeRewriter(mgr, enter); err != nil {
		t.Fatal(err)
	}
	var syn *statements.SynchronizedStatement
	core.WalkGraph[*core.Node](entry, func(n *core.Node) ([]*core.Node, error) {
		if st, ok := n.Statement.(*statements.SynchronizedStatement); ok {
			syn = st
		}
		return n.Next, nil
	})
	if syn == nil {
		t.Fatal("synchronize rewriter dropped the region")
	}
	if len(syn.Body) == 0 {
		t.Fatal("throw-only synchronized body was emptied")
	}
}
