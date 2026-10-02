package rewriter

import (
	"github.com/yaklang/javajive/internal/jdecenv"

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
	if !foundTop {
		// The normal exits of a waiting loop can each release the same
		// monitor. Prove every break/return releases it; then the loop's
		// continuation belongs outside the synchronized block.
		for i, st := range trySt.TryBody {
			loop, ok := st.(*statements.DoWhileStatement)
			if !ok || loop == nil || loop.Label != "" || !isUnconditionalMonitorLoop(loop) {
				continue
			}
			rewritten, releases, ok := stripReleasedLoopExits(loop.Body, false, 0)
			if !ok || releases == 0 {
				continue
			}
			clone := *loop
			clone.Body = rewritten
			bodySts = append(append([]statements.Statement{}, trySt.TryBody[:i]...), &clone)
			otherBody = trySt.TryBody[i+1:]
			foundTop = true
			break
		}
	}
	if !foundTop && jdecenv.Get("JDEC_SYNC_NESTED_MONITOREXIT_OFF") == "" {
		// monitor_exit was sunk into a nested try body (synchronized body is itself a try/catch).
		// Strip it in place and keep the entire TryBody as the synchronized body; there is no
		// post-synchronized continuation to hoist out in this shape.
		if nb, done := removeSunkMonitorExit(trySt.TryBody); done {
			bodySts = nb
			otherBody = nil
			foundTop = true
		}
	}
	if !foundTop {
		// Throw-only (or otherwise non-completing) synchronized bodies have no normal-path
		// monitorexit; javac puts the only monitorexit in the synthetic catch-all.
		bodySts = trySt.TryBody
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

// A transfer is accepted from decoded monitor_exit plus the loop structurer's
// explicit unlabeled transfer identity. Opaque leaves, nested monitors/loops,
// or any normal loop exit without a release fail closed; no rendered text is
// used to infer monitor ownership.
func stripReleasedLoopExits(body []statements.Statement, released bool, depth int) ([]statements.Statement, int, bool) {
	if depth > 32 || len(body) > 256 {
		return nil, 0, false
	}
	var out []statements.Statement
	count := 0
	for i, st := range body {
		if released {
			switch st.(type) {
			case *statements.ReturnStatement, *statements.CustomStatement:
			default:
				return nil, 0, false
			}
		}
		switch x := st.(type) {
		case *statements.MiddleStatement:
			if x == nil || x.Flag != "monitor_exit" || released {
				return nil, 0, false
			}
			released = true
			count++
			continue
		case *statements.CustomStatement:
			if x == nil {
				return nil, 0, false
			}
			if x.Name == "break" {
				if !released || i != len(body)-1 {
					return nil, 0, false
				}
				released = false
			} else if x.Name == "continue" {
				if released || i != len(body)-1 {
					return nil, 0, false
				}
			} else {
				return nil, 0, false
			}
		case *statements.ReturnStatement:
			if x == nil || !released || i != len(body)-1 {
				return nil, 0, false
			}
			released = false
		case *statements.IfStatement:
			if x == nil || released {
				return nil, 0, false
			}
			clone := *x
			var n int
			var ok bool
			clone.IfBody, n, ok = stripReleasedLoopExits(x.IfBody, false, depth+1)
			if !ok {
				return nil, 0, false
			}
			count += n
			clone.ElseBody, n, ok = stripReleasedLoopExits(x.ElseBody, false, depth+1)
			if !ok {
				return nil, 0, false
			}
			count += n
			st = &clone
		case *statements.AssignStatement, *statements.ExpressionStatement:
		default:
			return nil, 0, false
		}
		out = append(out, st)
	}
	if released {
		return nil, 0, false
	} // a release must be followed by its transfer
	return out, count, true
}

// Only a literal true loop can leave through the proved transfers. A normal
// condition-false edge has no monitor-release evidence in the loop body.
func isUnconditionalMonitorLoop(loop *statements.DoWhileStatement) bool {
	if loop == nil {
		return false
	}
	literal, ok := values.UnpackSoltValue(loop.ConditionValue).(*values.JavaLiteral)
	if !ok || literal == nil || literal.Type() == nil {
		return false
	}
	value, ok := literal.Data.(bool)
	return ok && value
}
