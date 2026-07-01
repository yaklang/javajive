package rewriter

import (
	"os"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"golang.org/x/exp/slices"
)

// removeSunkMonitorExit removes the first (normal-exit) monitor_exit MiddleStatement found by a
// depth-first walk that descends into the TRY body of nested try/catch statements. When the body of
// a synchronized region is itself a try/catch (e.g. `synchronized(lock){ try { return ...; } catch
// (...) { ... } }`), javac's structured form sinks the synthetic normal-path monitorexit into that
// nested try's body (right before the return), so it no longer appears at the top level of the
// synchronized wrapper's TryBody. In that situation the top-level scan in SynchronizeRewriter finds
// no monitor_exit and would otherwise emit an EMPTY synchronized body, dropping the whole try/catch
// (root cause of gson JsonStreamParser.hasNext `missing return statement`). This helper strips just
// the sunk monitorexit in place; the surrounding try/catch (with its return) stays as the body.
// Only the try arm is descended (the exceptional-path monitorexit lives in the wrapper's `any` catch,
// which is discarded), so exactly the normal-path monitorexit is removed.
func removeSunkMonitorExit(sts []statements.Statement) ([]statements.Statement, bool) {
	for i := 0; i < len(sts); i++ {
		if mv, ok := sts[i].(*statements.MiddleStatement); ok && mv.Flag == "monitor_exit" {
			out := append(append([]statements.Statement{}, sts[:i]...), sts[i+1:]...)
			return out, true
		}
		if tc, ok := sts[i].(*statements.TryCatchStatement); ok {
			if nb, done := removeSunkMonitorExit(tc.TryBody); done {
				tc.TryBody = nb
				return sts, true
			}
		}
	}
	return sts, false
}

func SynchronizeRewriter(manager *RewriteManager, node *core.Node) error {
	val := node.Statement.(*statements.MiddleStatement).Data.(values.JavaValue)
	// Find the TryCatchStatement following the monitor_enter. In rare cases the
	// monitor_enter may have multiple Next nodes (from CFG restructuring); search
	// all of them for a try-catch node.
	var tryNode *core.Node
	var trySt *statements.TryCatchStatement
	for _, n := range node.Next {
		if tc, ok := n.Statement.(*statements.TryCatchStatement); ok {
			trySt = tc
			tryNode = n
			break
		}
	}
	if trySt == nil {
		// No try-catch found — the synchronized pattern is non-standard. Emit a
		// synchronized block with an empty body and continue, rather than failing.
		synNode := manager.NewNode(statements.NewSynchronizedStatement(val, nil))
		for _, s := range node.Source {
			s.ReplaceNext(node, synNode)
		}
		for _, n := range node.Next {
			synNode.AddNext(n)
		}
		return nil
	}
	currentNode := tryNode
	var bodySts, otherBody []statements.Statement
	foundTop := false
	for i := 0; i < len(trySt.TryBody); i++ {
		if v, ok := trySt.TryBody[i].(*statements.MiddleStatement); ok && v.Flag == "monitor_exit" {
			bodySts = trySt.TryBody[:i]
			otherBody = trySt.TryBody[i+1:]
			foundTop = true
			break
		}
	}
	if !foundTop && os.Getenv("JDEC_SYNC_NESTED_MONITOREXIT_OFF") == "" {
		// monitor_exit was sunk into a nested try body (synchronized body is itself a try/catch).
		// Strip it in place and keep the entire TryBody as the synchronized body; there is no
		// post-synchronized continuation to hoist out in this shape.
		if nb, done := removeSunkMonitorExit(trySt.TryBody); done {
			bodySts = nb
			otherBody = nil
		}
	}
	next := slices.Clone(currentNode.Next)
	source := slices.Clone(node.Source)
	synNode := manager.NewNode(statements.NewSynchronizedStatement(val, bodySts))
	currentN := synNode
	for _, statement := range otherBody {
		n := manager.NewNode(statement)
		currentN.AddNext(n)
		currentN = n
	}
	nextNode := currentN
	for _, n := range next {
		n.RemoveSource(currentNode)
	}
	for _, n := range next {
		n.AddSource(nextNode)
	}
	for _, n := range source {
		n.ReplaceNext(node, synNode)
	}
	return nil
}
