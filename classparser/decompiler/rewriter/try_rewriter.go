package rewriter

import (
	"fmt"
	"slices"

	"github.com/samber/lo"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/utils"
)

// extractCatchException pulls the caught-exception variable out of a structured catch handler body
// and returns the remaining handler statements. Three handler shapes occur in real bytecode:
//
//  1. astore (javac normal form): body[0] is `<ref> = <exception placeholder>`. The ref is reused
//     as the catch variable and stripped from the body.
//  2. pop (the ECJ empty-catch idiom): body[0] merely discards the exception placeholder. There is
//     no named variable, so synthesize one (taking the concrete catch type from the placeholder)
//     and drop the discard.
//  3. fully elided: neither of the above. Synthesize a catch variable and keep the whole body so no
//     code is lost.
func (manager *RewriteManager) extractCatchException(body []statements.Statement) (*values.JavaRef, []statements.Statement) {
	if len(body) > 0 {
		if assign, ok := body[0].(*statements.AssignStatement); ok {
			if ref, ok := assign.LeftValue.(*values.JavaRef); ok {
				if cv, ok := core.UnpackSoltValue(assign.JavaValue).(*values.CustomValue); ok && cv.Flag == "exception" {
					return ref, body[1:]
				}
			}
		}
	}
	excType := types.JavaType(types.NewJavaClass("Throwable"))
	rest := body
	if len(body) > 0 {
		if expr, ok := body[0].(*statements.ExpressionStatement); ok {
			if cv, ok := core.UnpackSoltValue(expr.Expression).(*values.CustomValue); ok && cv.Flag == "exception" {
				if t := cv.Type(); t != nil {
					excType = t
				}
				rest = body[1:]
			}
		}
	}
	manager.syntheticCatchVarCounter++
	name := fmt.Sprintf("ex%d", manager.syntheticCatchVarCounter)
	ref := values.NewJavaRef(nil, nil, excType)
	ref.CustomValue = values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
		return name
	}, func() types.JavaType {
		return excType
	})
	return ref, rest
}

// isCatchHandlerBody reports whether a structured body begins with the synthetic
// caught-exception store (`<var> = <exception placeholder>`) that every catch handler
// opens with. The placeholder is the CustomValue tagged Flag=="exception" pushed onto
// the stack at the handler PC (see CalcOpcodeStackInfo). Detecting the handler this way
// instead of by successor position is load-bearing: by the time TryRewriter runs the
// later CFG passes (RemoveGotoStatement, loop/if structuring, node-id regeneration) may
// have reordered the try node's successor list, so node.Next[0] is NOT guaranteed to be
// the try body. When it was the catch handler instead, the old positional assumption fed
// the real try body into the catch slot, where it failed the exception-store check, got
// dropped, and left a try with zero catch handlers -> malformed stub.
func isCatchHandlerBody(body []statements.Statement) bool {
	if len(body) == 0 {
		return false
	}
	assign, ok := body[0].(*statements.AssignStatement)
	if !ok {
		return false
	}
	if _, ok := assign.LeftValue.(*values.JavaRef); !ok {
		return false
	}
	cv, ok := core.UnpackSoltValue(assign.JavaValue).(*values.CustomValue)
	return ok && cv.Flag == "exception"
}

func TryRewriter(manager *RewriteManager, node *core.Node) error {
	next := make([]*core.Node, len(node.Next))
	copy(next, node.Next)
	tryCatchSt := statements.NewTryCatchStatement(nil, nil)
	tryNode := manager.NewNode(tryCatchSt)
	node.Replace(tryNode)
	tryNode.RemoveAllNext()
	var endNodes []*core.Node
	visitedSet := utils.NewSet[*core.Node]()
	getBody := func(startNode, stopAt *core.Node) ([]statements.Statement, error) {
		var sts []statements.Statement
		err := core.WalkGraph[*core.Node](startNode, func(node *core.Node) ([]*core.Node, error) {
			if node == stopAt {
				endNodes = append(endNodes, node)
				return nil, nil
			}
			visitedSet.Add(node)
			err := manager.CheckVisitedNode(node)
			if err != nil {
				// Node was already visited (shared merge point). Skip instead of failing.
				return nil, nil
			}
			sts = append(sts, node.Statement)
			var next []*core.Node
			for _, n := range node.Next {
				if slices.Contains(manager.DominatorMap[node], n) {
					next = append(next, n)
				} else {
					if !visitedSet.Has(n) {
						endNodes = append(endNodes, n)
					}
				}
			}
			return next, nil
		})
		if err != nil {
			return nil, err
		}
		return sts, nil
	}

	// Structure every successor's body first, then classify each as the try body or a catch
	// handler by content (handler bodies open with the caught-exception store). The walk order
	// is preserved (it determines which path claims shared merge nodes), so only the labelling
	// changes versus the old positional scheme.
	// Throwable cleanup handlers (including try-with-resources) must not absorb
	// subsequent resources into the protected body. Typed user catches still
	// use the existing region builder, whose folded expression boundaries differ.
	cleanupRegion := false
	handlerCount := 0
	for _, start := range next {
		if !start.IsCatchStart {
			continue
		}
		handlerCount++
		if as, ok := start.Statement.(*statements.AssignStatement); ok {
			if cv, ok := core.UnpackSoltValue(as.JavaValue).(*values.CustomValue); ok && cv.Flag == "exception" {
				typ := cv.Type().String(&class_context.ClassContext{})
				if typ == "Throwable" || typ == "java.lang.Throwable" {
					cleanupRegion = true
				}
			}
		}
	}
	// Mixed typed catches plus a finally handler share the normal cleanup tail;
	// stopping only the try arm would detach that tail from the normal path.
	cleanupRegion = cleanupRegion && handlerCount == 1
	bodies := make([][]statements.Statement, len(next))
	for i := range next {
		var stopAt *core.Node
		if cleanupRegion && !next[i].IsCatchStart {
			stopAt = node.ProtectedEnd
			for seen := map[*core.Node]bool{}; stopAt != nil && !seen[stopAt]; {
				seen[stopAt] = true
				if _, jump := stopAt.Statement.(*statements.GOTOStatement); !jump || len(stopAt.Next) != 1 {
					break
				}
				stopAt = stopAt.Next[0]
			}
			// javac excludes the return instruction from its protected range,
			// but the returned expression may contain a protected invocation.
			// Expression folding similarly moves a whole protected expression
			// onto the first emitted statement at the exclusive boundary.
			if stopAt == next[i] {
				stopAt = nil
			}
			if stopAt != nil {
				if _, ret := stopAt.Statement.(*statements.ReturnStatement); ret {
					stopAt = nil
				} else if middle, ok := stopAt.Statement.(*statements.MiddleStatement); ok && middle.Flag == "monitor_exit" {
					// Kotlin's protected range ends before the normal monitorexit.
					// The synchronized structurer needs that marker in the body.
					stopAt = nil
				}
			}
		}
		body, err := getBody(next[i], stopAt)
		if err != nil {
			return err
		}
		bodies[i] = body
	}

	var tryBody []statements.Statement
	catchBodies := [][]statements.Statement{}

	// Classify each successor as the try body or a catch handler. Prefer the structural marker
	// set when the try node was built from the exception table (IsCatchStart): it survives
	// successor reordering AND handlers whose body has no leading exception store (empty/pop
	// catches that discard the unused exception). Fall back to the body-content heuristic when
	// no successor carries the marker (e.g. the handler entry node was replaced by an earlier
	// structuring pass).
	haveMarker := false
	for i := range next {
		if next[i].IsCatchStart {
			haveMarker = true
			break
		}
	}
	if haveMarker {
		tryIdx := -1
		for i := range bodies {
			if next[i].IsCatchStart {
				catchBodies = append(catchBodies, bodies[i])
			} else if tryIdx == -1 {
				tryIdx = i
				tryBody = bodies[i]
			} else {
				// More than one non-handler successor: keep the first as the try body and treat
				// the rest as catches so no code is silently dropped.
				catchBodies = append(catchBodies, bodies[i])
			}
		}
		if tryIdx == -1 && len(bodies) > 0 {
			tryBody = bodies[0]
		}
		// Each marker-classified successor is a genuine handler, so always materialize a catch
		// variable (synthesizing one for empty/pop handlers). This is what eliminates the
		// "try without catch handler" malformed-try stub.
		for i, body := range catchBodies {
			ref, rest := manager.extractCatchException(body)
			tryCatchSt.Exception = append(tryCatchSt.Exception, ref)
			catchBodies[i] = rest
		}
	} else {
		tryIdx := -1
		for i, body := range bodies {
			if isCatchHandlerBody(body) {
				catchBodies = append(catchBodies, body)
			} else if tryIdx == -1 {
				tryIdx = i
				tryBody = body
			} else {
				// More than one non-handler successor is ambiguous (the normal shape is one try
				// body plus N handlers). Fall back to the original positional interpretation to
				// avoid mis-structuring: treat next[0] as the try body and the rest as catches.
				tryIdx = -2
				break
			}
		}
		if tryIdx == -2 || tryIdx == -1 {
			tryBody = bodies[0]
			catchBodies = catchBodies[:0]
			for i := 1; i < len(bodies); i++ {
				catchBodies = append(catchBodies, bodies[i])
			}
		}
		for i, body := range catchBodies {
			var foundException bool
			if len(body) > 0 {
				if v, ok := body[0].(*statements.AssignStatement); ok {
					if v1, ok := v.LeftValue.(*values.JavaRef); ok {
						tryCatchSt.Exception = append(tryCatchSt.Exception, v1)
						catchBodies[i] = body[1:]
						foundException = true
					}
				}
			}
			if !foundException {
				catchBodies[i] = nil
			}
		}
		catchBodies = lo.Filter(catchBodies, func(item []statements.Statement, index int) bool {
			return item != nil
		})
	}
	tryCatchSt.TryBody = append(tryCatchSt.TryBody, tryBody...)
	tryCatchSt.CatchBodies = append(tryCatchSt.CatchBodies, catchBodies...)
	endNodes = lo.Filter(endNodes, func(item *core.Node, index int) bool {
		return item != tryNode && !IsEndNode(item)
	})
	for _, c := range NodeDeduplication(endNodes) {
		tryNode.AddNext(c)
	}
	return nil
}
