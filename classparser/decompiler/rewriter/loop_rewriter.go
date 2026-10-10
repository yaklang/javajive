package rewriter

import (
	"github.com/yaklang/javajive/internal/jdecenv"

	"github.com/samber/lo"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/classparser/decompiler/utils"
	utils2 "github.com/yaklang/javajive/internal/utils"
	"golang.org/x/exp/slices"
)

type LoopStatement struct {
	Condition values.JavaValue
	BodyStart *core.Node
}

// Executing a loop also dominates its normal continuation. That does not make
// the continuation part of its body. When choosing which loop must materialize
// jumps before an if/try is consumed, use body membership as well as dominance;
// otherwise an already processed inner loop hides the pending enclosing loop.
func loopOwnsRewriteNode(manager *RewriteManager, loop, node *core.Node) bool {
	if loop == node {
		return true
	}
	if len(loop.Next) == 0 || !utils.IsDominate(manager.DominatorMap, loop, node) {
		return false
	}
	if !manager.LoopRegionReducible || circleElementSet(loop, loop.Next[0], manager.DominatorMap, true).Has(node) {
		return true
	}
	// A successful break arm need not reach any backedge. Its private exit
	// prefix still belongs to this loop until the normal boundary. Structure
	// those transfers before an if on that prefix becomes an opaque container.
	boundary := searchCircleEndNode(loop, loop.Next[0], manager.DominatorMap, true)
	if boundary == nil || boundary == node {
		return false
	}
	seen := map[*core.Node]bool{}
	pending := []*core.Node{loop.Next[0]}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if current == nil || current == boundary || current == loop || seen[current] {
			continue
		}
		if len(seen) >= 512 {
			return false
		}
		seen[current] = true
		if current == node {
			return true
		}
		pending = append(pending, current.Next...)
	}
	return false
}

// A bounded retry loop has a condition between its wrapper and try body.
// Structure its successful break before TryRewriter consumes that body; doing
// so afterwards loses the normal exit and incorrectly retries on success.
// Require the other header edge to be the proved loop exit, not another body
// branch. Shared catch entries remain excluded by the caller.
func loopHeaderGuardsTry(manager *RewriteManager, loop, tr *core.Node) bool {
	if manager == nil || loop == nil || tr == nil || len(loop.Next) != 1 {
		return false
	}
	header := loop.Next[0]
	if _, ok := header.Statement.(*statements.ConditionStatement); !ok || len(header.Next) != 2 {
		return false
	}
	var other *core.Node
	if header.Next[0] == tr {
		other = header.Next[1]
	} else if header.Next[1] == tr {
		other = header.Next[0]
	} else {
		return false
	}
	return other != tr && other == searchCircleEndNode(loop, header, manager.DominatorMap, manager.LoopRegionReducible)
}

func RebuildLoopNode(manager *RewriteManager) error {
	for _, node := range manager.CircleEntryPoint {
		doWhileSt := statements.NewDoWhileStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), nil)
		doWhileNode := manager.NewNode(doWhileSt)
		// A try entry can coincide with a natural loop header. If its catches
		// never retry, the protected region encloses the loop and its normal
		// continuation; wrapping the try itself would move that continuation
		// outside the exception table.
		if normal, backedges := protectedTryLoopHeader(manager, node); normal != nil {
			for _, source := range backedges {
				replaceNextInPlace(source, node, doWhileNode)
			}
			replaceNextInPlace(node, normal, doWhileNode)
			doWhileNode.AddNext(normal)
			manager.WhileNode = append(manager.WhileNode, doWhileNode)
			continue
		}
		// Redirect every edge `source -> circleNode` to `source -> doWhileNode` while preserving the
		// edge's index in source.Next. The previous remove-all-source + AddSource approach appended
		// the redirected edge to the end of source.Next; for a bottom-tested loop the loop-condition
		// node is itself a back-edge source, so its branch to circleNode was shoved to index 1,
		// swapping the if's two successors and inverting the reconstructed do-while condition (break
		// and continue landed on the wrong branch). In-place replacement keeps branch polarity.
		for _, n := range slices.Clone(node.Source) {
			replaceNextInPlace(n, node, doWhileNode)
		}
		doWhileNode.AddNext(node)
		manager.WhileNode = append(manager.WhileNode, doWhileNode)
	}
	return nil
}

// replaceNextInPlace rewires the edge node->oldNext to node->newNext while keeping the edge at
// its original index inside node.Next. Position matters: a ConditionStatement's TrueNode()/
// FalseNode() are bound to fixed Next indices computed during graph construction, so appending a
// freshly created break/continue node (the old RemoveNext + AddNext pair) would shift it to the
// other branch and silently invert the loop condition - body and exit swap, producing
// "if (i < n) break; else { body }" instead of "if (i < n) { body } else break;". Replacing in
// place preserves the branch polarity so the reconstructed loop keeps its original semantics.
func replaceNextInPlace(node, oldNext, newNext *core.Node) {
	node.ReplaceSwitchTarget(oldNext, newNext)
	idx := slices.Index(node.Next, oldNext)
	node.RemoveNext(oldNext)
	if idx < 0 || idx > len(node.Next) {
		node.AddNext(newNext)
		return
	}
	node.Next = slices.Insert(node.Next, idx, newNext)
	// newNext is already spliced into node.Next; AddNext only fixes the reverse Source link here.
	node.AddNext(newNext)
}

// outermostEnclosingLoopWithExit returns the outermost ENCLOSING loop (a while-node that dominates
// circleNode, excluding circleNode itself) whose own loop-exit node equals exitNode, or nil if no
// enclosing loop shares that exit. "Outermost" = the one that dominates the others, so a break that
// escapes several stacked loops with the same exit target jumps all the way out. Used to turn a bare
// `break` (which would only leave the innermost loop) into a labeled `break LOOP_n` when the exit
// actually lies outside an enclosing loop. Setting JDEC_NO_LOOP_BREAK_LABEL_FIX is the kill-switch.
func outermostEnclosingLoopWithExit(manager *RewriteManager, preWhileNodes []*core.Node, preWhileNodeEnds map[*core.Node]*core.Node, circleNode, exitNode *core.Node) *core.Node {
	if jdecenv.Get("JDEC_NO_LOOP_BREAK_LABEL_FIX") != "" {
		return nil
	}
	var best *core.Node
	for _, m := range preWhileNodes {
		if m == circleNode {
			continue
		}
		if preWhileNodeEnds[m] != exitNode {
			continue
		}
		if best == nil || utils.IsDominate(manager.DominatorMap, m, best) {
			best = m
		}
	}
	return best
}

// asLatchIncExpr reports whether a node is a bare loop-increment latch statement of the form `i++`
// or `i--` (a single int local incremented/decremented by ±1, the iinc javac emits for a for-loop
// step), returning the underlying JavaExpression. Compound steps (`i += k`, AssignStatement) and any
// other statement shape are rejected so only the clean for-step latch is eligible for split-continue
// reconstruction.
func asLatchIncExpr(n *core.Node) (*values.JavaExpression, bool) {
	if n == nil {
		return nil, false
	}
	e, ok := n.Statement.(*values.JavaExpression)
	if !ok {
		return nil, false
	}
	if e.Op != values.INC && e.Op != values.DEC {
		return nil, false
	}
	if len(e.Values) < 2 {
		return nil, false
	}
	return e, true
}

// convertSplitContinueToLatch repairs for-loop `continue` statements that javac compiled as a `goto`
// to the loop's increment/step node (the latch `i++`) instead of to the loop header. When a loop body
// has TWO OR MORE early-continue branches that jump to the single bottom-of-body increment (e.g. gson
// JsonWriter.string / the Repro fixture: `if(c<128){r=arr[c]; if(r==null) continue;} else if(...) ...
// else continue; <write r>; i++;`), the increment node is the if/else region's post-dominator. Those
// `goto increment` edges are NOT back-edges to the header, so RebuildLoopNode never redirected them and
// LoopJmpRewriter's `next == circleNode` continue rule never fires; the edges survive as ordinary
// fall-through into the increment. IfRewriter then structures the branches as EMPTY (the continue body
// was just the dropped goto) and the post-if write runs unconditionally over a local that the
// continue paths never assigned ("variable r might not have been initialized"). The single trailing
// increment cannot serve every continue path, and a bare `continue` would skip the step and spin.
//
// Fix: for each early-continue predecessor P of the latch, splice a PRIVATE copy of the increment plus
// an explicit `continue` (P -> i++' -> continue -> header), so each path steps then re-tests exactly
// like the source for-loop, while the latch's own fall-through path is left untouched. circleNode here
// is the RebuildLoopNode do-while wrapper, so wiring the new continue to it matches the existing
// `next == circleNode` continue mechanism. Gated to: a single ±1 latch whose ONLY successor is the
// loop header, with >=2 conditional/jump predecessors AND >=1 fall-through predecessor — so the clean
// single-continue loop (handled by IfRewriter branch inversion) and loops without a separable step
// latch are byte-for-byte unchanged. Kill-switch: JDEC_SPLIT_CONTINUE_LATCH_OFF=1.
func convertSplitContinueToLatch(manager *RewriteManager, circleNode *core.Node) {
	if jdecenv.Get("JDEC_SPLIT_CONTINUE_LATCH_OFF") != "" {
		return
	}
	for _, latch := range slices.Clone(circleNode.Source) {
		if latch == circleNode {
			continue
		}
		incExpr, ok := asLatchIncExpr(latch)
		if !ok {
			continue
		}
		// The latch must be a genuine bottom-of-body step: its sole successor is the loop header and it
		// lives inside the loop body.
		if len(latch.Next) != 1 || latch.Next[0] != circleNode {
			continue
		}
		if !utils.IsDominate(manager.DominatorMap, circleNode, latch) {
			continue
		}
		var jumpPreds, fallPreds []*core.Node
		for _, p := range slices.Clone(latch.Source) {
			if p == circleNode {
				continue
			}
			// A predecessor outside the loop body cannot be a body-level continue; keep it intact.
			if !utils.IsDominate(manager.DominatorMap, circleNode, p) {
				fallPreds = append(fallPreds, p)
				continue
			}
			if p.IsJmp || len(p.Next) >= 2 {
				jumpPreds = append(jumpPreds, p)
			} else {
				fallPreds = append(fallPreds, p)
			}
		}
		if len(jumpPreds) < 2 || len(fallPreds) < 1 {
			continue
		}
		for _, p := range jumpPreds {
			// Build a FRESH increment expression reusing the latch's variable ref (so RewriteVar renames
			// it consistently) but not the latch node itself, then an explicit continue to the header.
			incrNode := manager.NewNode(values.NewBinaryExpression(incExpr.Values[0], incExpr.Values[1], incExpr.Op, incExpr.Typ))
			contNode := manager.NewNode(statements.NewSourceTransferStatement("continue", ""))
			contNode.IsJmp = true
			replaceNextInPlace(p, latch, incrNode)
			incrNode.AddNext(contNode)
			contNode.AddNext(circleNode)
		}
	}
}

func LoopJmpRewriter(manager *RewriteManager, circleNode *core.Node) error {
	convertSplitContinueToLatch(manager, circleNode)
	loopEnd := searchCircleEndNode(circleNode, circleNode.Next[0], manager.DominatorMap, manager.LoopRegionReducible)

	preWhileNodes := utils.NodeFilter(manager.WhileNode, func(node *core.Node) bool {
		return utils.IsDominate(manager.DominatorMap, node, circleNode)
	})
	preWhileNodeEnds := map[*core.Node]*core.Node{}
	for _, n := range preWhileNodes {
		preWhileNodeEnds[n] = searchCircleEndNode(n, n.Next[0], manager.DominatorMap, manager.LoopRegionReducible)
	}
	checkNode := func(node *core.Node) ([]*core.Node, error) {
		if node.IsJmp {
			manager.qualifyLoopTransfer(node, circleNode)
			return nil, nil
		}
		if _, ok := node.Statement.(*statements.IfStatement); ok {
			return nil, nil
		}
		if !utils.IsDominate(manager.DominatorMap, circleNode, node) {
			return nil, nil
		}
		nextList := []*core.Node{}
		allNext := slices.Clone(node.Next)
		for _, next := range allNext {
			// An exception-handler entry (catch / finally-desugar / try-with-resources suppress handler)
			// is reached ONLY via the exception edge, never by normal loop control flow. LoopJmpRewriter
			// must not rewrite that edge into a break/continue: doing so deletes the handler from the
			// enclosing try node's successor list, so the later TryRewriter can no longer wrap the loop
			// body in try/catch - the handler is emitted as dangling post-loop code and the
			// caught-exception placeholder leaks as a bare `Exception` token. Keep that entry edge;
			// traverse the handler's normal flow only when this loop dominates its entry.
			// Kill-switch: JDEC_LOOP_KEEP_CATCH_EDGE_OFF=1.
			if next.IsCatchStart && jdecenv.Get("JDEC_LOOP_KEEP_CATCH_EDGE_OFF") == "" {
				// Keep the exceptional entry edge intact, but a handler entered
				// only from inside this loop still owns normal break/continue
				// edges. Materialize those before a catch/if consumes its body.
				if utils.IsDominate(manager.DominatorMap, circleNode, next) {
					nextList = append(nextList, next)
				}
				continue
			}
			if next == circleNode {
				continueNode := manager.NewNode(statements.NewSourceTransferStatement("continue", ""))
				continueNode.IsJmp = true
				continueNode.Statement.(*statements.CustomStatement).Name = "continue"
				manager.recordLoopTransfer(continueNode, circleNode, "continue", node)
				replaceNextInPlace(node, next, continueNode)
				continueNode.AddNext(next)
				continue
			}

			if false && !utils.IsDominate(manager.DominatorMap, node, next) && node != circleNode {
				if node != circleNode {
					breakNode := manager.NewNode(statements.NewSourceTransferStatement("break", ""))
					breakNode.HideNext = next
					breakNode.IsJmp = true
					node.RemoveNext(next)
					node.AddNext(breakNode)
					breakNode.AddNext(circleNode)
					circleNode.AddNext(next)
					continue
				}
				breakNode := manager.NewNode(statements.NewSourceTransferStatement("break", ""))
				breakNode.HideNext = next
				breakNode.IsJmp = true

				matched := utils.NodeFilter(manager.WhileNode, func(node *core.Node) bool {
					return node == next
				})
				if len(matched) > 0 {
					if utils.IsDominate(manager.DominatorMap, matched[0], circleNode) {
						loopNode := matched[0].Statement.(*statements.DoWhileStatement)
						if loopNode.Label == "" {
							label := manager.NewLoopLabel()
							loopNode.Label = label
						}
						breakNode.Statement = statements.NewSourceTransferStatement("continue", loopNode.Label)
					}
					//} else {
					//	return nil, errors.New("loop end node conflict")
					//}
				} else {
					//var ok bool
					for _, n := range manager.WhileNode {
						if loopEnd == next && utils.IsDominate(manager.DominatorMap, n, circleNode) {
							loopNode := n.Statement.(*statements.DoWhileStatement)
							if loopNode.Label == "" {
								label := manager.NewLoopLabel()
								loopNode.Label = label
							}
							breakNode.Statement = statements.NewSourceTransferStatement("break", loopNode.Label)
							//ok = true
							break
						}
					}
					//if !ok {
					//	return nil, errors.New("loop end node conflict")
					//}
				}

				node.RemoveNext(next)
				node.AddNext(breakNode)
				breakNode.AddNext(next)
				continue
			}
			// When the tight loop-exit search returns an ENCLOSING loop's header (a while-node that
			// dominates circleNode), the edge node->next is a back edge to that outer loop, i.e. a
			// `continue LOOP_n`, never a break. This only became reachable once the reducible-method exit
			// search (excludePreHeader) started reporting the true tight exit: for a nested loop whose inner
			// body falls straight back to the outer header (see TestNestedLoop), loopEnd is now the outer
			// do-while node itself. Skip the plain-break branch here so the labeled-continue handling below
			// (the matched-while-node path) runs and emits `continue LOOP_n` instead of a bare `break`.
			nextIsEnclosingLoopHeader := slices.Contains(manager.WhileNode, next) &&
				utils.IsDominate(manager.DominatorMap, next, circleNode)
			if loopEnd != nil && (next == loopEnd && node != circleNode) && !nextIsEnclosingLoopHeader {
				// Shared-exit nested loops: this loop's computed exit (loopEnd) can coincide with an
				// ENCLOSING loop's exit. javac compiles `do { ... while(inner) ... } while(outer)` where the
				// post-test of an inner while is absorbed into the inner do-while(true) body; the only way
				// out of the inner loop then targets a node that is ALSO outside the outer loop. A bare
				// `break` exits only the inner do-while and falls through to the bottom of the outer body,
				// so the outer do-while(true) loops forever and the post-loop code is unreachable (javac:
				// "unreachable statement"). When the exit escapes an enclosing loop, emit a labeled
				// `break LOOP_n` targeting the OUTERMOST loop whose exit is this same node instead.
				if enclosing := outermostEnclosingLoopWithExit(manager, preWhileNodes, preWhileNodeEnds, circleNode, next); enclosing != nil {
					loopNode := enclosing.Statement.(*statements.DoWhileStatement)
					if loopNode.Label == "" {
						loopNode.Label = manager.NewLoopLabel()
					}
					breakNode := manager.NewNode(statements.NewSourceTransferStatement("break", loopNode.Label))
					breakNode.IsJmp = true
					// Mirror the plain-break wiring but hand the exit edge to the ENCLOSING loop: the break
					// leaf flows to the outer loop node, and the outer loop node owns the edge to the shared
					// exit. This lets the outer LoopRewriter pick `next` up as its exit (endNode) so the
					// post-loop code stays reachable, instead of being dropped as "incomplete control flow".
					replaceNextInPlace(node, next, breakNode)
					breakNode.AddNext(enclosing)
					enclosing.AddNext(next)
					continue
				}
				// A switch captures an unlabeled break. Preserve the actual loop target
				// when the exiting edge belongs to a switch nested in this loop.
				breakLabel := ""
				for _, sw := range manager.SwitchNode {
					if utils.IsDominate(manager.DominatorMap, circleNode, sw) &&
						(sw == node || utils.IsDominate(manager.DominatorMap, sw, node)) {
						loop := circleNode.Statement.(*statements.DoWhileStatement)
						if loop.Label == "" {
							loop.Label = manager.NewLoopLabel()
						}
						breakLabel = loop.Label
						break
					}
				}
				breakNode := manager.NewNode(statements.NewSourceTransferStatement("break", breakLabel))
				replaceNextInPlace(node, next, breakNode)
				breakNode.AddNext(circleNode)
				circleNode.AddNext(next)
				breakNode.IsJmp = true
				if breakLabel == "" {
					breakNode.Statement.(*statements.CustomStatement).Name = "break"
					manager.recordLoopTransfer(breakNode, circleNode, "break", node)
				}
				continue
			}
			if node != circleNode {
				matched := utils.NodeFilter(manager.WhileNode, func(node *core.Node) bool {
					return node == next
				})
				if len(matched) > 0 {
					if utils.IsDominate(manager.DominatorMap, matched[0], circleNode) {
						loopNode := matched[0].Statement.(*statements.DoWhileStatement)
						if loopNode.Label == "" {
							label := manager.NewLoopLabel()
							loopNode.Label = label
						}
						breakNode := manager.NewNode(statements.NewSourceTransferStatement("break", ""))
						breakNode.Statement = statements.NewSourceTransferStatement("continue", loopNode.Label)
						breakNode.IsJmp = true
						replaceNextInPlace(node, next, breakNode)
						breakNode.AddNext(matched[0])
					}
					//} else {
					//	return nil, errors.New("loop end node conflict")
					//}
				} else {
					//var ok bool
					for _, n := range manager.WhileNode {
						// Bug O: a labeled `break outer` jumps from inside this (inner) loop to an enclosing
						// loop's exit. Reverse-topological processing structures the inner loop FIRST, so the
						// enclosing do-while still carries only its single wrapper edge (len(Next)==1) at this
						// point; the legacy guard skipped it and the break-outer (plus the assignment right
						// before it) was dropped, leaving the inner loop spinning forever. Relax the guard for
						// reducible methods (preWhileNodeEnds is only populated for genuine enclosing loops, so
						// non-enclosing while-nodes still cannot match). Irreducible methods keep the guard.
						relaxLabelGuard := manager.LoopRegionReducible && jdecenv.Get("JDEC_NO_LOOP_BREAK_LABEL_FIX") == ""
						if len(n.Next) < 2 && !relaxLabelGuard {
							continue
						}
						endNode := preWhileNodeEnds[n]
						if endNode == next {
							loopNode := n.Statement.(*statements.DoWhileStatement)
							if loopNode.Label == "" {
								label := manager.NewLoopLabel()
								loopNode.Label = label
							}
							breakNode := manager.NewNode(statements.NewSourceTransferStatement("break", ""))
							breakNode.Statement = statements.NewSourceTransferStatement("break", loopNode.Label)
							breakNode.IsJmp = true
							replaceNextInPlace(node, next, breakNode)
							// The enclosing loop owns its continuation; a jump leaf
							// alone is consumed by the inner branch collector. Keep
							// the original exit as analysis metadata, without adding
							// a synthetic back edge that would absorb the outer body.
							breakNode.HideNext = endNode
							n.AddNext(endNode)
							break
						}
					}
					//if !ok {
					//	return nil, errors.New("loop end node conflict")
					//}
				}
			}
			nextList = append(nextList, next)
		}
		return nextList, nil
	}
	times := 0
	err := core.WalkGraph[*core.Node](circleNode.Next[0], func(node *core.Node) ([]*core.Node, error) {
		times++
		//return node.Next, nil
		return checkNode(node)
	})
	if err != nil {
		return err
	}
	_, err = checkNode(circleNode)
	if err != nil {
		return err
	}
	return nil
}
func LoopRewriter(manager *RewriteManager, node *core.Node) error {
	circleNode := node
	loopStart := circleNode.Next[0]
	circleNode.RemoveNext(loopStart)

	body := []statements.Statement{}
	bodyNodes := []*core.Node{}
	endNodes := []*core.Node{}
	circleSet := getCircleElementSet(circleNode, loopStart, manager.DominatorMap)
	err := core.WalkGraph[*core.Node](loopStart, func(node *core.Node) ([]*core.Node, error) {
		if !circleSet.Has(node) {
			endNodes = append(endNodes, node)
			return nil, nil
		}
		err := manager.CheckVisitedNode(node)
		if err != nil {
			// Node was already visited (shared merge point). Skip instead of failing.
			return nil, nil
		}
		body = append(body, node.Statement)
		bodyNodes = append(bodyNodes, node)
		var next []*core.Node
		for _, n := range node.Next {
			if slices.Contains(manager.DominatorMap[node], n) {
				next = append(next, n)
			} else {
				if n != circleNode {
					endNodes = append(endNodes, n)
				}
			}
		}
		return next, nil
	})
	if err != nil {
		return err
	}
	doWhileSt := statements.NewDoWhileStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), nil)
	doWhileSt.Label = circleNode.Statement.(*statements.DoWhileStatement).Label
	doWhileSt.Body = append(doWhileSt.Body, body...)
	//allSource := slices.Clone(node.Source)
	//node.RemoveAllSource()
	//for _, n := range allSource {
	//	n.AddNext(manager.NewNode(doWhileSt))
	//}
	loopNode := manager.NewNode(doWhileSt)
	circleNode.Replace(loopNode)
	endNodes = lo.Filter(endNodes, func(item *core.Node, index int) bool {
		return !IsEndNode(item)
	})
	for _, c := range NodeDeduplication(endNodes) {
		loopNode.AddNext(c)
	}
	// An encoded transfer to an enclosing loop remains abrupt after wrapping
	// this loop. Its own break, however, completes the wrapper normally: the
	// old circle node carries that ordinary continuation and must participate
	// in the classification too.
	markEncodedJumps(loopNode, append(bodyNodes, circleNode))
	return nil
}
func getCircleElementSet(circleNode *core.Node, loopStart *core.Node, domTree map[*core.Node][]*core.Node) *utils2.Set[*core.Node] {
	return circleElementSet(circleNode, loopStart, domTree, false)
}

// circleElementSet computes the loop body for the loop whose header is circleNode (its original header
// retained as loopStart). It reverse-BFS's from the back-edge sources (nodes whose Next includes
// circleNode), stopping at circleNode, so post-loop code is excluded.
//
// excludePreHeader controls SEED selection. The default (false) seeds from EVERY predecessor of
// circleNode — the historical behavior, kept verbatim for the loop BODY because real-world IRREDUCIBLE
// loops (e.g. ant CBZip2OutputStream.hbMakeCodeLengths) depend on that over-approximation and collapse
// ("has circle") if any seed is dropped. When excludePreHeader is true (used ONLY by the exit/loopEnd
// search of a REDUCIBLE method, see searchCircleEndNode) it drops genuine pre-header FORWARD entry edges
// so the exit of a reducible nested loop is computed from the tight body instead of leaking into the
// enclosing loop.
func circleElementSet(circleNode *core.Node, loopStart *core.Node, domTree map[*core.Node][]*core.Node, excludePreHeader bool) *utils2.Set[*core.Node] {
	finalSet := utils2.NewSet[*core.Node]()
	// Step 1: collect all nodes reachable from loopStart and build reverse adjacency.
	reverseAdj := map[*core.Node][]*core.Node{}
	allNodes := utils2.NewSet[*core.Node]()
	{
		stk := []*core.Node{loopStart}
		for len(stk) > 0 {
			n := stk[len(stk)-1]
			stk = stk[:len(stk)-1]
			if allNodes.Has(n) {
				continue
			}
			allNodes.Add(n)
			for _, next := range loopAnalysisSuccessors(n) {
				reverseAdj[next] = append(reverseAdj[next], n)
				if !allNodes.Has(next) {
					stk = append(stk, next)
				}
			}
		}
	}
	// Step 2: find back-edge sources — nodes whose Next includes circleNode. Iterate in stable id order
	// (Set.List() is map-ordered) so allSources, and the reverse-BFS latch set derived from it, are
	// deterministic.
	var allSources []*core.Node
	for _, n := range sortNodesByID(allNodes.List()) {
		if slices.Contains(loopAnalysisSuccessors(n), circleNode) {
			allSources = append(allSources, n)
		}
	}
	sources := allSources
	if excludePreHeader && jdecenv.Get("JDEC_NO_LOOP_BACKEDGE_DOM_FILTER") == "" {
		// Drop forward pre-header entry edges. The caller only sets excludePreHeader for a method whose
		// ORIGINAL CFG is reducible (see RewriteManager.LoopRegionReducible), so a forward, non-dominated
		// predecessor is a genuine pre-header rather than an alternate entry of an irreducible tangle.
		// Node IDs are allocation order, not bytecode order. A preceding loop
		// wrapper can have a larger ID than this header without being a back
		// edge. Require dominance even for synthetic predecessors; otherwise
		// reverse reachability leaks through the pre-header into an outer loop.
		var kept []*core.Node
		for _, n := range allSources {
			if utils.IsDominate(domTree, circleNode, n) {
				kept = append(kept, n)
			}
		}
		sources = kept
	}
	finalSet = reverseBFSStopAt(sources, reverseAdj, circleNode)

	// A retry may return to the header only through its catch. The synthetic
	// try node represents exceptional edges at region entry, so reverse reachability
	// alone omits the successful protected path. Include that path up to the
	// exception table's exclusive end; otherwise the first protected store is
	// mistaken for the loop exit and moved outside the handler.
	for _, tr := range finalSet.List() {

		end := tr.ProtectedEnd
		if end == nil {
			continue
		}
		for seen := map[*core.Node]bool{}; end != nil && !seen[end]; {
			seen[end] = true
			if _, jump := end.Statement.(*statements.GOTOStatement); !jump || len(end.Next) != 1 {
				break
			}
			end = end.Next[0]
		}
		if !hasPrivateRetryHandler(tr, finalSet) {
			continue
		}
		if path, boundary := protectedRetryBranchPrefix(tr, circleNode); boundary != nil {
			tr.ProtectedEnd = boundary
			for _, n := range path {
				finalSet.Add(n)
			}
			continue
		}
		var path []*core.Node
		reachedEnd := false
		seen := map[*core.Node]bool{}
		var visit func(*core.Node)
		visit = func(n *core.Node) {
			if n == end {
				// The return PC is outside the protected interval, but folded
				// operand calls can still belong entirely to this private try.
				if protectedRetryFoldedReturn(tr, n) {
					path = append(path, n)
				}
				reachedEnd = true
				return
			}
			if n == nil || n == circleNode || n.IsCatchStart || seen[n] {
				return
			}
			seen[n] = true
			path = append(path, n)
			for _, next := range loopAnalysisSuccessors(n) {
				visit(next)
			}
		}
		for _, next := range tr.Next {
			if !next.IsCatchStart {
				visit(next)
			}
		}
		if !reachedEnd {
			// Boolean/if reduction can remove the old exclusive-end node.
			// Re-establish only a private straight-line protected prefix from
			// original PC witnesses, then retain its current boundary for later
			// break/continue materialization. IDs and dangling pointers are not
			// evidence that a throwing store lies outside the retry's try.
			if prefix, boundary := protectedRetryPrefix(tr, circleNode); boundary != nil {
				tr.ProtectedEnd = boundary
				path, reachedEnd = prefix, true
			}
		}
		if reachedEnd {
			for _, n := range path {
				finalSet.Add(n)
			}
		}
	}
	return finalSet
}

func protectedRetryPrefix(tr, header *core.Node) ([]*core.Node, *core.Node) {
	if tr == nil || !tr.HasProtectedRange || tr.ProtectedStartPC < 0 || tr.ProtectedEndPC <= tr.ProtectedStartPC {
		return nil, nil
	}
	var first *core.Node
	for _, next := range tr.Next {
		if next != nil && !next.IsCatchStart {
			if first != nil {
				return nil, nil
			}
			first = next
		}
	}
	var prefix []*core.Node
	previous, pc := tr, tr.ProtectedStartPC-1
	for current := first; len(prefix) < 32; {
		if current == nil || current == header || current.IsCatchStart || current.IsJmp || !current.HasOriginPC || current.OriginPC <= pc {
			return nil, nil
		}
		if current.OriginPC >= tr.ProtectedEndPC {
			if len(prefix) != 0 {
				return prefix, current
			}
			return nil, nil
		}
		if current.OriginPC < tr.ProtectedStartPC || len(current.Source) != 1 || current.Source[0] != previous || len(current.Next) != 1 {
			return nil, nil
		}
		switch current.Statement.(type) {
		case *statements.AssignStatement, *statements.ExpressionStatement:
		default:
			return nil, nil
		}
		prefix = append(prefix, current)
		previous, pc, current = current, current.OriginPC, current.Next[0]
	}
	return nil, nil
}

// A private retry can leave the protected range from several conditional
// branches. Reverse reachability from its catch misses those successful paths.
// Prove the entire acyclic protected subgraph by original PCs, including each
// branch and its operands' statement, and require every normal exit to reach
// the same unprotected continuation. No foreign entry, nested container,
// unwitnessed statement, back edge or unresolved exit is admitted.
func protectedRetryBranchPrefix(tr, header *core.Node) ([]*core.Node, *core.Node) {
	if tr == nil || !tr.HasProtectedRange || tr.ProtectedStartPC < 0 || tr.ProtectedEndPC <= tr.ProtectedStartPC {
		return nil, nil
	}
	ranges := tr.SharedProtectedRanges
	endPC := tr.ProtectedEndPC
	if tr.SharedProtectedHandler {
		if len(ranges) < 2 || len(ranges) > 32 {
			return nil, nil
		}
		handler, catchType := ranges[0].HandlerPc, ranges[0].CatchType
		previousEnd := tr.ProtectedStartPC
		for _, row := range ranges {
			if row.HandlerPc != handler || row.CatchType != catchType || int(row.StartPc) < previousEnd || row.EndPc <= row.StartPc {
				return nil, nil
			}
			previousEnd = int(row.EndPc)
		}
		if int(ranges[0].StartPc) != tr.ProtectedStartPC {
			return nil, nil
		}
		endPC = previousEnd
	}
	covered := func(pc int) bool {
		if !tr.SharedProtectedHandler {
			return pc >= tr.ProtectedStartPC && pc < endPC
		}
		for _, row := range ranges {
			if pc >= int(row.StartPc) && pc < int(row.EndPc) {
				return true
			}
		}
		return false
	}
	var first *core.Node
	for _, next := range tr.Next {
		if next != nil && !next.IsCatchStart {
			if first != nil {
				return nil, nil
			}
			first = next
		}
	}

	state := map[*core.Node]int{}
	var path []*core.Node
	var boundary *core.Node
	var visit func(*core.Node) bool
	visit = func(n *core.Node) bool {
		if n == nil || n == header || n.IsCatchStart || n.IsJmp || !n.HasOriginPC {
			return false
		}
		if n.OriginPC >= endPC {
			seen := map[*core.Node]bool{}
			for {
				if _, jump := n.Statement.(*statements.GOTOStatement); !jump {
					break
				}
				if seen[n] || len(n.Next) != 1 {
					return false
				}
				seen[n] = true
				n = n.Next[0]
				if n == nil || n == header || n.IsCatchStart || !n.HasOriginPC || n.OriginPC < endPC {
					return false
				}
			}
			if boundary != nil && boundary != n {
				return false
			}
			boundary = n
			return true
		}
		if n.OriginPC < tr.ProtectedStartPC || state[n] == 1 || len(state) >= 32 {
			return false
		}
		if state[n] == 2 {
			return true
		}
		if !covered(n.OriginPC) {
			if _, transfer := n.Statement.(*statements.GOTOStatement); !transfer {
				return false
			}
		}
		switch n.Statement.(type) {
		case *statements.ConditionStatement:
			if len(n.Next) != 2 {
				return false
			}
		case *statements.AssignStatement, *statements.ExpressionStatement, *statements.GOTOStatement:
			if len(n.Next) != 1 {
				return false
			}
		default:
			return false
		}
		state[n] = 1
		path = append(path, n)
		for _, next := range n.Next {
			if !visit(next) {
				return false
			}
		}
		state[n] = 2
		return true
	}
	if !visit(first) || boundary == nil || len(path) == 0 {
		return nil, nil
	}
	for _, n := range path {
		for _, source := range n.Source {
			if source != tr && state[source] != 2 {
				return nil, nil
			}
		}
	}

	return path, boundary
}

func hasPrivateRetryHandler(tr *core.Node, body *utils2.Set[*core.Node]) bool {
	for _, next := range tr.Next {
		if next != nil && next.IsCatchStart && body.Has(next) {
			// A shared finally cleanup may coexist with a private retry catch;
			// a retry handler with alternate entries cannot prove this region.
			if !hasSharedCatchEntry(tr) || (!next.SharedProtectedHandler &&
				len(next.Source) == 1 && next.Source[0] == tr) {
				return true
			}
			if len(next.Source) == 1 && next.Source[0] == tr {
				if _, boundary := protectedRetryBranchPrefix(tr, nil); boundary != nil {
					return true
				}
			}
		}
	}
	return false
}

func protectedRetryContinuations(body *utils2.Set[*core.Node], header *core.Node) map[*core.Node]bool {
	exits := map[*core.Node]bool{}
	for _, tr := range body.List() {
		if !hasPrivateRetryHandler(tr, body) {
			continue
		}
		prefix, boundary := protectedRetryPrefix(tr, header)
		if branch, exit := protectedRetryBranchPrefix(tr, header); exit != nil {
			prefix, boundary = branch, exit
		}
		if boundary == nil || body.Has(boundary) {
			continue
		}
		complete := true
		for _, node := range prefix {
			complete = complete && body.Has(node)
		}
		if complete {
			exits[boundary] = true
		}
	}
	return exits
}

func loopAnalysisSuccessors(node *core.Node) []*core.Node {
	if node.HideNext != nil && len(node.Next) == 0 {
		return []*core.Node{node.HideNext}
	}
	return node.Next
}

// isReducibleCFG reports whether the (pristine) CFG rooted at root is a reducible flow graph, using the
// Dragon-book characterization: partition edges into back edges and forward edges, where a back edge is
// any m->s whose head s DOMINATES its tail m; the graph is reducible iff the remaining forward edges
// form a DAG (removing the back edges leaves no cycle).
//
// We must NOT approximate "back edge" by bytecode/Id order: node Id is a per-node ordinal that does not
// honour control-flow topology — control-flow SINKS like the synthetic `end` node and early `return`
// nodes get LOW Ids even though every edge into them is a forward edge to a terminal. An Id-based
// "retreating edge" test therefore flags e.g. `return var -> end` as retreating and, because `end` does
// not dominate the return, misreports a perfectly reducible nested loop with an early return (Sieve of
// Eratosthenes guarded by `if (n < 2) return 0;`) as irreducible. Dominance-based back-edge detection
// avoids that entirely.
//
// Must be called BEFORE RebuildLoopNode so every node is an original node and the dominator relation is
// not yet perturbed by do-while wrappers or break/continue edges.
func isReducibleCFG(root *core.Node, domTree map[*core.Node][]*core.Node) bool {
	allNodes := []*core.Node{}
	seen := map[*core.Node]bool{}
	core.WalkGraph[*core.Node](root, func(n *core.Node) ([]*core.Node, error) {
		if !seen[n] {
			seen[n] = true
			allNodes = append(allNodes, n)
		}
		return n.Next, nil
	})
	// forward = every edge except back edges (head dominates tail; this also covers self loops because a
	// node always dominates itself).
	forward := map[*core.Node][]*core.Node{}
	for _, m := range allNodes {
		for _, s := range m.Next {
			if utils.IsDominate(domTree, s, m) {
				continue
			}
			forward[m] = append(forward[m], s)
		}
	}
	// Iterative three-colour DFS cycle detection over the forward subgraph (iterative to avoid blowing
	// the stack on large methods). A grey-target edge means a remaining cycle => irreducible.
	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := map[*core.Node]int{}
	type frame struct {
		node *core.Node
		idx  int
	}
	for _, start := range allNodes {
		if color[start] != white {
			continue
		}
		color[start] = grey
		stack := []frame{{start, 0}}
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			succ := forward[top.node]
			if top.idx < len(succ) {
				s := succ[top.idx]
				top.idx++
				switch color[s] {
				case grey:
					return false
				case white:
					color[s] = grey
					stack = append(stack, frame{s, 0})
				}
			} else {
				color[top.node] = black
				stack = stack[:len(stack)-1]
			}
		}
	}
	return true
}

// reverseBFSStopAt returns {circleNode} ∪ all nodes that can reach a seed via reverse edges without
// passing through circleNode.
func reverseBFSStopAt(seeds []*core.Node, reverseAdj map[*core.Node][]*core.Node, circleNode *core.Node) *utils2.Set[*core.Node] {
	set := utils2.NewSet[*core.Node]()
	set.Add(circleNode)
	queue := []*core.Node{}
	for _, n := range seeds {
		if !set.Has(n) {
			set.Add(n)
			queue = append(queue, n)
		}
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, pred := range reverseAdj[n] {
			if pred != circleNode && !set.Has(pred) {
				set.Add(pred)
				queue = append(queue, pred)
			}
		}
	}
	return set
}

func searchCircleEndNode(circleNode *core.Node, loopStart *core.Node, domTree map[*core.Node][]*core.Node, reducible bool) *core.Node {
	// For a reducible method, use the pre-header-excluding body so a nested loop's exit is computed from
	// its tight body (Bug Q): with the legacy over-approximated body, the reverse-BFS leaks through this
	// loop's exit into the enclosing loop, making the only out-node the OUTER exit, so the inner loop
	// never gets its `else{break}` and becomes a non-terminating do-while(true). Irreducible methods keep
	// the legacy body to avoid collapsing the loop. The LOOP BODY itself always stays on the legacy path
	// (getCircleElementSet); only this exit search prunes pre-headers.
	elementSet := circleElementSet(circleNode, loopStart, domTree, reducible)
	outNodes := []*core.Node{}
	elementSet.ForEach(func(node *core.Node) {
		for _, n := range loopAnalysisSuccessors(node) {
			if !elementSet.Has(n) {
				// An exception-handler entry (catch / finally-desugar / try-with-resources suppress
				// handler) is reached ONLY via the exception edge, never as a normal loop exit. Counting
				// it as an out-edge fabricates a spurious second exit for a loop whose body contains a
				// try-start: the generic multi-exit merge below then collapses the real fall-out exit and
				// the handler into their shared post-dominator (typically the method's `return`), so
				// loopEnd is wrong, the normal exit edge never becomes a `break`, and the loop degrades to
				// a non-terminating do-while(true) with the post-loop continuation absorbed into the body
				// (Bug U second form: try-with-resources + finally whose body is a loop). Exclude handler
				// edges so the tight fall-out exit is found. Kill-switch: JDEC_LOOP_KEEP_CATCH_EDGE_OFF=1.
				if n.IsCatchStart && jdecenv.Get("JDEC_LOOP_KEEP_CATCH_EDGE_OFF") == "" {
					continue
				}
				outNodes = append(outNodes, n)
			}
		}
	})
	outNodes = NodeDeduplication(outNodes)
	if len(outNodes) == 0 {
		return nil
	}
	if len(outNodes) == 1 {
		// A private original throw is an abrupt arm, even when it is the
		// only boundary of an otherwise nonterminating loop. It has no
		// normal continuation that can be expressed by a generated break.
		if reducible && originalLoopThrowLeaf(outNodes[0]) && exclusiveTerminalLoopBranch(outNodes[0], circleNode, domTree) {
			return nil
		}
		return outNodes[0]
	}
	// Bug O — multi-exit loop: a nested loop that carries a labeled break/continue to an enclosing loop
	// has more than one out-edge (its own fall-out exit PLUS the secondary jump that escapes outward).
	// The generic merge below collapses those exits into their common post-dominator, which for
	// `for{ for{ if(..) continue outer; if(..){found=..; break outer;} } }` is the method's return, not
	// the inner loop's real exit — so the inner loop loses its fall-out target and the break-outer's
	// preceding assignment is dropped. Prefer the loop HEADER's own exit edge: the unique successor of
	// the header (loopStart) that lies OUTSIDE the tight loop body is the canonical while-style fall-out
	// exit, and the remaining out-edges are then correctly classified as labeled break/continue by
	// LoopJmpRewriter. Gated on a reducible method (the header is well-defined) and only when there is
	// genuine multi-exit ambiguity, so single-exit loops are byte-for-byte unchanged.
	if reducible && jdecenv.Get("JDEC_NO_LOOP_HEADER_EXIT") == "" {
		var headerOut []*core.Node
		for _, n := range loopStart.Next {
			if n.IsCatchStart && jdecenv.Get("JDEC_LOOP_KEEP_CATCH_EDGE_OFF") == "" {
				continue
			}
			if !elementSet.Has(n) {
				headerOut = append(headerOut, n)
			}
		}
		// A successful nested search may return directly at its header's
		// false edge, while mismatch continues an outer loop through its
		// step. Keep that terminal arm inline; the step is the actual normal
		// continuation. Picking the return loses the outer-continue edge.
		if len(NodeDeduplication(headerOut)) == 1 {
			// A throwing header guard is an inline terminal arm when a
			// successful body exits to a shared normal continuation. Choosing
			// the throw as the loop boundary lifts the remaining candidate
			// checks (and their back edges) outside the loop.
			normalAlternative := false
			throwingHeader := terminalRegionOnlyThrows(headerOut[0])
			for _, out := range outNodes {
				if throwingHeader && out != headerOut[0] && !IsEndNode(out) && !exclusiveTerminalLoopBranch(out, circleNode, domTree) && loopExitCannotResumeOwner(out, circleNode) {
					normalAlternative = true
					break
				}
			}
			privateTerminal := exclusiveTerminalLoopBranch(headerOut[0], circleNode, domTree)
			if !privateTerminal || (!originalLoopThrowLeaf(headerOut[0]) && !normalAlternative && !hasEnclosingLoopContinuation(outNodes, circleNode, domTree)) {
				return headerOut[0]
			}
		}
	}
	// Early returns and terminal switch bodies are inline alternatives to a
	// loop continuation. Requiring a continuation to post-dominate them loses
	// ordinary breaks whenever a loop can also return early. Conversely, a
	// terminal case shared by fall-through labels must remain inside switch,
	// rather than becoming an enclosing-loop break that skips its effects.
	inlineCases := map[*core.Node]bool{}
	for _, sw := range elementSet.List() {
		middle, ok := sw.Statement.(*statements.MiddleStatement)
		if !ok || middle.Flag != statements.MiddleSwitch {
			continue
		}
		data, ok := middle.Data.([]any)
		if !ok {
			continue
		}
		cases, err := switchCaseNodes(data, sw)
		if err != nil {
			continue
		}
		for _, target := range cases.Values() {
			if target == sw.MergeNode && (sw.SwitchEmptyCaseMerge || sw.SwitchEmptyDefaultMerge) {
				continue
			}
			if !switchCaseHasOnlyJumpEntries(sw, target, cases) {
				inlineCases[target] = true
			}
		}
	}
	var continuations []*core.Node
	protectedExits := map[*core.Node]bool{}
	if reducible {
		protectedExits = protectedRetryContinuations(elementSet, circleNode)
	}
	for _, out := range outNodes {
		if inlineCases[out] || IsEndNode(out) {
			continue
		}
		// A return shared by distinct branches can itself be the normal
		// continuation. A single-path return is already an inline terminal.
		// A retry catch's back edge is exceptional; the successful protected
		// prefix falls out normally at the exception table's exclusive end.
		// Its sole return may contain a downstream call outside that handler.
		// Keeping it as an inline early return absorbs that call into the try
		// and retries downstream exceptions. Only the private PC-proved path
		// may override the ordinary terminal-arm rule.
		if exclusiveTerminalLoopBranch(out, circleNode, domTree) && !protectedExits[out] {
			continue
		}
		continuations = append(continuations, out)
	}
	if len(continuations) == 0 {
		return nil
	}
	if len(continuations) == 1 {
		return continuations[0]
	}
	return commonLoopExit(continuations)
}

// Shared handler entries describe split protected intervals, not one lexical
// try body. Leave those regions to the existing shared-handler structurer.
func hasSharedCatchEntry(node *core.Node) bool {
	if node.SharedProtectedHandler {
		return true
	}
	for _, next := range node.Next {
		if next.IsCatchStart && len(next.Source) > 1 {
			return true
		}
	}
	return false
}

func loopHasProvedProtectedRetry(manager *RewriteManager, loop *core.Node) bool {
	if len(loop.Next) == 0 {
		return false
	}
	body := circleElementSet(loop, loop.Next[0], manager.DominatorMap, true)
	for _, tr := range body.List() {
		if hasPrivateRetryHandler(tr, body) {
			if _, boundary := protectedRetryBranchPrefix(tr, loop); boundary != nil {
				return true
			}
		}
	}
	return false
}

// A private retry's successful continuation can be structured before its try.
// Materialize that loop's exit before an if at this boundary consumes it; the
// continuation is outside the cyclic body but still owns the incoming exit.
func protectedRetryContinuationOwnsRewriteNode(manager *RewriteManager, loop, node *core.Node) bool {
	if loop == nil || node == nil || len(loop.Next) != 1 {
		return false
	}
	body := circleElementSet(loop, loop.Next[0], manager.DominatorMap, true)
	return protectedRetryContinuations(body, loop)[node]
}

func protectedTryLoopHeader(manager *RewriteManager, node *core.Node) (*core.Node, []*core.Node) {
	if manager == nil || node == nil || !manager.LoopRegionReducible || !node.HasProtectedRange || node.ProtectedEndPC <= node.ProtectedStartPC {
		return nil, nil
	}
	middle, ok := node.Statement.(*statements.MiddleStatement)
	if !ok || middle.Flag != statements.MiddleTryStart {
		return nil, nil
	}
	var normal *core.Node
	handlers := []*core.Node{}
	for _, next := range node.Next {
		if next.IsCatchStart {
			handlers = append(handlers, next)
		} else if normal == nil {
			normal = next
		} else {
			return nil, nil
		}
	}
	if normal == nil || len(handlers) == 0 {
		return nil, nil
	}
	// A bounded walk proves that no handler reaches the loop header. Retrying
	// catches require the existing per-iteration try layout instead.
	seen := map[*core.Node]bool{}
	queue := append([]*core.Node{}, handlers...)
	for len(queue) > 0 {
		n := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if n == node {
			return nil, nil
		}
		if n == nil || seen[n] {
			continue
		}
		seen[n] = true
		if len(seen) > 512 {
			return nil, nil
		}
		queue = append(queue, n.Next...)
	}
	var backedges []*core.Node
	for _, source := range node.Source {
		if utils.IsDominate(manager.DominatorMap, node, source) {
			backedges = append(backedges, source)
		}
	}
	if len(backedges) == 0 {
		return nil, nil
	}
	return normal, backedges
}

// Loop passes can materialize an outer transfer before discovering its lexical
// placement inside an inner loop. Preserve the target identity at creation;
// a later inner pass qualifies that same transfer instead of interpreting its
// printed break/continue as an exit of the innermost loop.
type loopTransfer struct {
	target *core.Node
	kind   string
}

func (manager *RewriteManager) recordLoopTransfer(node, target *core.Node, kind string, sources ...*core.Node) {
	if manager.loopTransfers == nil {
		manager.loopTransfers = map[*core.Node]loopTransfer{}
	}
	manager.loopTransfers[node] = loopTransfer{target, kind}
	if len(sources) == 1 {
		// Qualify before a container captures the statement pointer. An
		// enclosing-header transfer keeps that exact destination even when
		// a preceding inner loop also dominates its continuation.
		for _, inner := range manager.WhileNode {
			if inner != target && utils.IsDominate(manager.DominatorMap, target, inner) && utils.IsDominate(manager.DominatorMap, inner, sources[0]) {
				manager.qualifyTargetTransfer(node, target, kind)
				break
			}
		}
	}

}
func (manager *RewriteManager) qualifyLoopTransfer(node, current *core.Node) {
	transfer, ok := manager.loopTransfers[node]
	if !ok || transfer.target == current || !utils.IsDominate(manager.DominatorMap, transfer.target, current) || !utils.IsDominate(manager.DominatorMap, current, node) {
		return
	}
	manager.qualifyTargetTransfer(node, transfer.target, transfer.kind)
}
func (manager *RewriteManager) qualifyTargetTransfer(node, target *core.Node, kind string) {
	loop, ok := target.Statement.(*statements.DoWhileStatement)
	if !ok || loop == nil {
		return
	}
	original, ok := node.Statement.(*statements.CustomStatement)
	if !ok || original == nil || original.ThrownValue != nil {
		return
	}
	if loop.Label == "" {
		loop.Label = manager.NewLoopLabel()
	}
	copy := *original
	copy.Name = ""
	copy.LoopTransferKind, copy.LoopTargetLabel = kind, loop.Label
	copy.StringFunc = func(*class_context.ClassContext) string { return kind + " " + loop.Label }
	copy.RetargetSourceTransfer(kind, loop.Label)
	node.Statement = &copy
}
