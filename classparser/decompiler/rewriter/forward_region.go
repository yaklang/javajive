package rewriter

import (
	"fmt"
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	utils2 "github.com/yaklang/javajive/classparser/decompiler/utils"
	"github.com/yaklang/javajive/internal/jdecenv"
)

type forwardRegion struct {
	exit            *core.Node
	label           string
	protectedEnd    *core.Node
	protectedRanges [][2]int
}

type forwardEdge struct{ from, to *core.Node }

func (manager *RewriteManager) primaryForwardRegion(owner *core.Node) *forwardRegion {
	if manager == nil || owner == nil {
		return nil
	}
	regions := manager.forwardRegions[owner]
	if len(regions) == 0 {
		return nil
	}
	return regions[0]
}

func (manager *RewriteManager) protectedForwardRegion(owner *core.Node) *forwardRegion {
	if manager == nil || owner == nil {
		return nil
	}
	for _, r := range manager.forwardRegions[owner] {
		if r != nil && (r.protectedEnd != nil || len(r.protectedRanges) != 0) {
			return r
		}
	}
	return nil
}

func (manager *RewriteManager) whileNodeSet() map[*core.Node]bool {
	if manager == nil || len(manager.WhileNode) == 0 {
		return nil
	}
	set := make(map[*core.Node]bool, len(manager.WhileNode))
	for _, n := range manager.WhileNode {
		if n != nil {
			set[n] = true
		}
	}
	return set
}

func forwardEnclosingBarriers(manager *RewriteManager, owner, liveStart *core.Node) map[*core.Node]bool {
	barriers := map[*core.Node]bool{}
	if owner != nil {
		barriers[owner] = true
	}
	if liveStart != nil {
		barriers[liveStart] = true
	}
	if manager == nil || len(manager.DominatorMap) == 0 {
		return barriers
	}
	parent := map[*core.Node]*core.Node{}
	work := 65536
	for ancestor, children := range manager.DominatorMap {
		for _, child := range children {
			work--
			if work < 0 {
				break
			}
			if child != nil && child != ancestor && parent[child] == nil {
				parent[child] = ancestor
			}
		}
		if work < 0 {
			break
		}
	}
	addAncestors := func(start *core.Node) {
		seen := map[*core.Node]bool{}
		for cur, depth := parent[start], 0; cur != nil && depth < 1024 && !seen[cur]; cur, depth = parent[cur], depth+1 {
			seen[cur] = true
			barriers[cur] = true
		}
	}
	if owner != nil {
		addAncestors(owner)
	}
	if liveStart != nil && liveStart != owner {
		addAncestors(liveStart)
	}
	return barriers
}

func forwardIsLoopBackTransfer(n, next *core.Node, whileSet map[*core.Node]bool) bool {
	if n == nil || next == nil {
		return false
	}
	if cs, ok := n.Statement.(*statements.CustomStatement); ok && cs != nil {
		if kind, _, ok := cs.SourceTransferProtocol(); ok && kind == "continue" {
			return true
		}
	}
	return encodedJumpTo(n, next) && (whileSet[next] || next.IsCircle)
}

func pendingForwardRegionExits(manager *RewriteManager, exclude *core.Node) map[*core.Node]bool {
	if manager == nil || len(manager.forwardRegions) == 0 {
		return nil
	}
	var stops map[*core.Node]bool
	for _, list := range manager.forwardRegions {
		for _, r := range list {
			if r == nil || r.exit == nil || r.exit == exclude {
				continue
			}
			if stops == nil {
				stops = map[*core.Node]bool{}
			}
			stops[r.exit] = true
		}
	}
	return stops
}

// forwardExitReaches reports whether goal is reachable from from along normal
// or forward-break CFG edges without crossing barriers, loop-back transfers, or END.
func forwardExitReaches(from, goal *core.Node, barriers, whileSet map[*core.Node]bool) bool {
	if from == nil || goal == nil || barriers[from] || barriers[goal] {
		return false
	}
	if from == goal {
		return true
	}
	seen := map[*core.Node]bool{}
	queue := []*core.Node{from}
	for len(queue) > 0 && len(seen) < 2048 {
		n := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if n == nil || seen[n] || IsEndNode(n) || barriers[n] {
			continue
		}
		if n == goal {
			return true
		}
		seen[n] = true
		for _, next := range n.Next {
			if next == nil || barriers[next] || forwardIsLoopBackTransfer(n, next, whileSet) {
				continue
			}
			if next == goal {
				return true
			}
			if !seen[next] && !IsEndNode(next) {
				queue = append(queue, next)
			}
		}
	}
	return false
}

// forwardTerminalChainWithoutReaching reports whether every path starting from
// from terminates at an original method exit without reaching avoided or barriers,
// within a small dominator-safe DAG bound.
func forwardTerminalChainWithoutReaching(from, avoided *core.Node, barriers, whileSet map[*core.Node]bool) bool {
	if from == nil || from == avoided || barriers[from] {
		return false
	}
	seen := map[*core.Node]bool{}
	queue := []*core.Node{from}
	for len(queue) > 0 && len(seen) < 512 {
		n := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if n == nil || seen[n] {
			continue
		}
		if n == avoided || barriers[n] || IsEndNode(n) {
			return false
		}
		seen[n] = true
		if forwardOriginalMethodExit(n) || (len(n.Next) == 0 && forwardStructuredAbruptMethodExit(n.Statement, 0)) {
			continue
		}
		if len(n.Next) == 0 {
			return false
		}
		for _, next := range n.Next {
			if next == nil || next == avoided || barriers[next] || forwardIsLoopBackTransfer(n, next, whileSet) {
				return false
			}
			if !seen[next] {
				queue = append(queue, next)
			}
		}
	}
	return len(seen) > 0 && len(queue) == 0
}

// forwardExitNestedBefore reports whether innerExit must be wrapped before outerExit
// for regions owned by owner.
func forwardExitNestedBefore(manager *RewriteManager, owner, innerExit, outerExit *core.Node) (nested bool, ordered bool) {
	if innerExit == nil || outerExit == nil || innerExit == outerExit {
		return false, false
	}
	barriers := forwardEnclosingBarriers(manager, owner, nil)
	whileSet := manager.whileNodeSet()
	innerToOuter := forwardExitReaches(innerExit, outerExit, barriers, whileSet)
	outerToInner := forwardExitReaches(outerExit, innerExit, barriers, whileSet)
	if innerToOuter && outerToInner {
		return false, false
	}
	if innerToOuter {
		return true, true
	}
	if outerToInner {
		return false, true
	}
	if manager != nil && len(manager.DominatorMap) > 0 {
		innerDom := utils2.IsDominate(manager.DominatorMap, owner, innerExit)
		outerDom := utils2.IsDominate(manager.DominatorMap, owner, outerExit)
		if innerDom && !outerDom {
			return true, true
		}
		if outerDom && !innerDom {
			return false, true
		}
	}
	// An inner branch whose tail terminates in return/throw sits strictly inside an
	// enclosing region targeting a post-if continuation that never returns to it.
	innerTerm := forwardTerminalChainWithoutReaching(innerExit, outerExit, barriers, whileSet) && !outerToInner
	outerTerm := forwardTerminalChainWithoutReaching(outerExit, innerExit, barriers, whileSet) && !innerToOuter
	if innerTerm && !outerTerm {
		return true, true
	}
	if outerTerm && !innerTerm {
		return false, true
	}
	return false, false
}

func (manager *RewriteManager) insertForwardRegion(owner *core.Node, region *forwardRegion) {
	if manager.forwardRegions == nil {
		manager.forwardRegions = map[*core.Node][]*forwardRegion{}
	}
	for _, existing := range manager.forwardRegions[owner] {
		if existing != nil && existing.exit == region.exit {
			return
		}
	}
	list := append(manager.forwardRegions[owner], region)
	for i := len(list) - 1; i > 0; i-- {
		nested, ordered := forwardExitNestedBefore(manager, owner, list[i].exit, list[i-1].exit)
		if ordered && nested {
			list[i], list[i-1] = list[i-1], list[i]
		}
	}
	manager.forwardRegions[owner] = list
}

func findCommonForwardJoin(candidates []*core.Node, condition *core.Node, excluded map[*core.Node]bool) *core.Node {
	if len(candidates) < 2 {
		return nil
	}
	candSet := make(map[*core.Node]bool, len(candidates))
	for _, c := range candidates {
		candSet[c] = true
	}
	reachCount := map[*core.Node]int{}
	dist0 := map[*core.Node]int{}
	for idx, start := range candidates {
		seen := map[*core.Node]int{start: 0}
		queue := []*core.Node{start}
		for len(queue) > 0 && len(seen) < 128 {
			n := queue[0]
			queue = queue[1:]
			d := seen[n]
			if d > 0 {
				reachCount[n]++
				if idx == 0 {
					dist0[n] = d
				}
			}
			if excluded[n] {
				continue
			}
			for _, next := range n.Next {
				if next == nil || next == condition || IsEndNode(next) {
					continue
				}
				if _, ok := seen[next]; !ok {
					seen[next] = d + 1
					queue = append(queue, next)
				}
			}
		}
	}
	var best *core.Node
	bestDist := 1 << 30
	for node, cnt := range reachCount {
		if cnt != len(candidates) || candSet[node] || excluded[node] {
			continue
		}
		d := dist0[node]
		if best == nil || d < bestDist || (d == bestDist && node.Id < best.Id) {
			best = node
			bestDist = d
		}
	}
	return best
}

// An inner decision can reach a shared computation or bypass it and leave
// an enclosing region. Dominance alone cannot turn both outcomes into normal
// fall-through. Seal the bypass as a region break before collecting the arm;
// later, enclose the proved region in a single-execution source block.
func materializeForwardRegionExits(manager *RewriteManager, condition *core.Node) {
	if manager == nil || condition == nil {
		return
	}
	maxSteps := 16
	if jdecenv.Get("JDEC_NESTED_FORWARD_REGION_OFF") == "1" {
		maxSteps = 1
	}
	registeredJoins := map[*core.Node]bool{}
	type pendingJoin struct {
		owner  *core.Node
		region *forwardRegion
	}
	var pending *pendingJoin
	rollbackPending := func() {
		if pending == nil {
			return
		}
		list := manager.forwardRegions[pending.owner]
		kept := list[:0]
		for _, r := range list {
			if r != pending.region {
				kept = append(kept, r)
			}
		}
		if len(kept) == 0 {
			delete(manager.forwardRegions, pending.owner)
		} else {
			manager.forwardRegions[pending.owner] = kept
		}
		pending = nil
	}
	for step := 0; step < maxSteps; step++ {
		var frontier []forwardEdge
		seen := map[*core.Node]bool{}
		queue := []*core.Node{condition}
		remaining := 4096
		for len(queue) > 0 {
			n := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			if n == nil || len(seen) >= 1024 {
				rollbackPending()
				return
			}
			if seen[n] {
				continue
			}
			seen[n] = true
			// Method exits have no normal frontier. Counting their synthetic END
			// edge would turn a simple return-or-continue into a region bypass.
			if forwardOriginalMethodExit(n) {
				continue
			}
			for _, next := range n.Next {
				remaining--
				if remaining < 0 {
					rollbackPending()
					return
				}
				if encodedJumpTo(n, next) {
					continue
				}
				if next == condition {
					rollbackPending()
					return
				}
				if utils2.IsDominate(manager.DominatorMap, condition, next) {
					queue = append(queue, next)
				} else {
					frontier = append(frontier, forwardEdge{n, next})
				}
			}
		}
		if len(frontier) < 2 {
			return
		}
		// The continuation may also have predecessors outside this decision
		// (for example an enclosing guard's false arm). Its immediate dominator
		// is then too broad. Find the smallest ancestor containing this decision
		// and every non-exit frontier, and prove that region independently.
		parent := map[*core.Node]*core.Node{}
		work := 65536
		for ancestor, children := range manager.DominatorMap {
			for _, child := range children {
				work--
				if work < 0 || child == nil || parent[child] != nil {
					rollbackPending()
					return
				}
				parent[child] = ancestor
			}
		}
		var candidates []*core.Node
		tried := map[*core.Node]bool{}
		for _, edge := range frontier {
			if !tried[edge.to] {
				tried[edge.to] = true
				candidates = append(candidates, edge.to)
			}
		}
		// Prefer outermost exits first when multiple exits coexist so inner
		// branches cleanly break to the enclosing boundary first.
		if maxSteps > 1 && len(candidates) > 1 {
			for i := 1; i < len(candidates); i++ {
				for j := i; j > 0; j-- {
					nested, ordered := forwardExitNestedBefore(manager, condition, candidates[j-1], candidates[j])
					if ordered && nested {
						candidates[j-1], candidates[j] = candidates[j], candidates[j-1]
					}
				}
			}
		}
		findOwnerForExit := func(targetExit *core.Node) *core.Node {
			for candidate, depth := parent[condition], 0; candidate != nil && depth < 1024; candidate, depth = parent[candidate], depth+1 {
				if _, raw := candidate.Statement.(*statements.ConditionStatement); !raw {
					continue
				}
				other, contains := false, true
				for _, e := range frontier {
					work--
					if work < 0 {
						return nil
					}
					if e.to != targetExit {
						other = true
						contains = contains && utils2.IsDominate(manager.DominatorMap, candidate, e.to)
					}
				}
				if other && contains {
					return candidate
				}
			}
			return nil
		}
		if maxSteps > 1 && len(candidates) > 1 {
			var existingFirst, fresh []*core.Node
			for _, cand := range candidates {
				matched := false
				if owner := findOwnerForExit(cand); owner != nil {
					for _, r := range manager.forwardRegions[owner] {
						if r != nil && r.exit == cand {
							matched = true
							break
						}
					}
				}
				if matched {
					existingFirst = append(existingFirst, cand)
				} else {
					fresh = append(fresh, cand)
				}
			}
			candidates = append(existingFirst, fresh...)
		}
		sealed := false
		for _, exit := range candidates {
			owner := findOwnerForExit(exit)
			if owner == nil {
				continue
			}
			ok, siblingBreaks, outerLabels := closedForwardRegionWithSiblingExits(manager, owner, exit)
			if !ok {
				continue
			}
			var region *forwardRegion
			for _, existing := range manager.forwardRegions[owner] {
				if existing != nil && existing.exit == exit {
					region = existing
					break
				}
			}
			if maxSteps == 1 && len(manager.forwardRegions[owner]) > 0 && region == nil {
				continue
			}
			if region == nil {
				region = &forwardRegion{exit: exit, label: manager.NewLoopLabel()}
				manager.insertForwardRegion(owner, region)
			}
			for _, sb := range siblingBreaks {
				if !slices.Contains(sb.from.Next, sb.to) || encodedJumpTo(sb.from, sb.to) {
					continue
				}
				jump := manager.NewNode(statements.NewSourceTransferStatement("break", outerLabels[sb.to]))
				jump.AddNext(sb.to)
				jump.EncodedJumps = map[*core.Node]bool{sb.to: true}
				replaceNextInPlace(sb.from, sb.to, jump)
			}
			for _, e := range frontier {
				if e.to != exit || !slices.Contains(e.from.Next, exit) || encodedJumpTo(e.from, exit) {
					continue
				}
				jump := manager.NewNode(statements.NewSourceTransferStatement("break", region.label))
				jump.AddNext(exit)
				jump.EncodedJumps = map[*core.Node]bool{exit: true}
				replaceNextInPlace(e.from, exit, jump)
			}
			manager.refreshDominatorsShared()
			pending = nil
			sealed = true
			break
		}
		if sealed {
			continue
		}
		rollbackPending()
		if maxSteps > 1 && len(candidates) >= 2 {
			if join := findCommonForwardJoin(candidates, condition, registeredJoins); join != nil {
				if owner := findOwnerForExit(join); owner != nil && closedForwardRegion(manager, owner, join) {
					registeredJoins[join] = true
					already := false
					for _, existing := range manager.forwardRegions[owner] {
						if existing != nil && existing.exit == join {
							already = true
							break
						}
					}
					if !already {
						joinRegion := &forwardRegion{exit: join, label: manager.NewLoopLabel()}
						manager.insertForwardRegion(owner, joinRegion)
						pending = &pendingJoin{owner: owner, region: joinRegion}
						continue
					}
				}
			}
		}
		return
	}
	rollbackPending()
}

// Prove single entry and a closed acyclic normal-flow region. Structured
// inner loops are atomic; raw cycles, hidden/exception edges and external
// transfers cannot be swallowed by this new lexical scope. The proof makes
// no assumptions about local names, values, branch polarity or arm effects.
func closedForwardRegion(manager *RewriteManager, owner, exit *core.Node) bool {
	ok, _, _ := closedForwardRegionWithSiblingExits(manager, owner, exit)
	return ok
}

func closedForwardRegionWithSiblingExits(manager *RewriteManager, owner, exit *core.Node) (bool, []forwardEdge, map[*core.Node]string) {
	if manager == nil || !manager.originalHandlersKnown || owner == nil || exit == nil || !owner.HasOriginPC || exit.IsCatchStart || IsEndNode(exit) {
		return false, nil, nil
	}
	if _, ok := forwardEntryPC(manager, exit); !ok {
		return false, nil, nil
	}
	singleOnly := jdecenv.Get("JDEC_NESTED_FORWARD_REGION_OFF") == "1"
	type regionKind uint8
	const (
		regionInvalid regionKind = iota
		regionSame
		regionInner
		regionOuter
	)
	allowedExit := map[*core.Node]regionKind{}
	allowedLabel := map[*core.Node]string{}
	ownerOuterLabel := map[*core.Node]string{}
	barriers := forwardEnclosingBarriers(manager, owner, nil)
	whileSet := manager.whileNodeSet()
	exitDom := utils2.IsDominate(manager.DominatorMap, owner, exit)
	needsTailCheck := false
	for _, r := range manager.forwardRegions[owner] {
		if r == nil || r.exit == nil {
			continue
		}
		if r.exit == exit {
			allowedExit[r.exit] = regionSame
			allowedLabel[r.exit] = r.label
			continue
		}
		if singleOnly {
			return false, nil, nil
		}
		rDom := utils2.IsDominate(manager.DominatorMap, owner, r.exit)
		nestedInner, ordered := forwardExitNestedBefore(manager, owner, r.exit, exit)
		if !ordered {
			if len(manager.forwardRegions[owner]) >= 16 || !exitDom || !rDom {
				return false, nil, nil
			}
			if !forwardExitReaches(r.exit, exit, barriers, whileSet) && !forwardExitReaches(exit, r.exit, barriers, whileSet) {
				allowedExit[r.exit] = regionOuter
				ownerOuterLabel[r.exit] = r.label
				allowedLabel[r.exit] = r.label
				needsTailCheck = true
				continue
			}
			return false, nil, nil
		}
		if nestedInner {
			if !rDom {
				return false, nil, nil
			}
			allowedExit[r.exit] = regionInner
		} else {
			if !exitDom {
				return false, nil, nil
			}
			allowedExit[r.exit] = regionOuter
			ownerOuterLabel[r.exit] = r.label
			if !forwardExitReaches(exit, r.exit, barriers, whileSet) {
				needsTailCheck = true
			}
		}
		allowedLabel[r.exit] = r.label
	}
	if !singleOnly {
		for anc, regions := range manager.forwardRegions {
			if anc == nil || anc == owner || !utils2.IsDominate(manager.DominatorMap, anc, owner) {
				continue
			}
			ancBarriers := forwardEnclosingBarriers(manager, anc, nil)
			for _, r := range regions {
				if r == nil || r.exit == nil || r.exit == owner || r.exit == exit {
					continue
				}
				if _, already := allowedExit[r.exit]; already {
					continue
				}
				if forwardExitReaches(r.exit, exit, ancBarriers, whileSet) {
					continue
				}
				allowedExit[r.exit] = regionOuter
				allowedLabel[r.exit] = r.label
			}
		}
	}
	remaining := 65536
	sameDomain := func(n *core.Node) bool {
		pc, known := forwardEntryPC(manager, n)
		if !known || owner.OriginPC < 0 || owner.OriginPC > 65535 {
			return false
		}
		remaining -= len(manager.originalHandlers) + 1
		if remaining < 0 {
			return false
		}
		// javac can split one handler's interval around an unprotected GOTO.
		// Domain identity is the ordered exception dispatch, not row identity.
		// Keep table order: overlapping catches with reversed precedence are
		// different domains even when they contain the same handlers.
		var current, original [][2]uint16
		for _, row := range manager.originalHandlers {
			if row.StartPc >= row.EndPc {
				return false
			}
			if pc >= int(row.StartPc) && pc < int(row.EndPc) {
				current = append(current, [2]uint16{row.HandlerPc, row.CatchType})
			}
			if owner.OriginPC >= int(row.StartPc) && owner.OriginPC < int(row.EndPc) {
				original = append(original, [2]uint16{row.HandlerPc, row.CatchType})
			}
		}
		return slices.Equal(current, original)
	}
	// The boundary is not emitted inside the new block. A proved typed-try
	// continuation may therefore lie outside this domain; the existing try
	// collector must also stop there. Every included node still needs the
	// owner's complete dispatch domain and the closed-region proof below.
	if !sameDomain(exit) && !typedProtectedIfCollectionStops(manager, owner)[exit] {
		return false, nil, nil
	}
	var siblingBreaks []forwardEdge
	siblingSeen := map[forwardEdge]bool{}
	canSealSiblingEdge := func(n, target *core.Node) bool {
		if singleOnly || ownerOuterLabel[target] == "" || n == exit || forwardExitReaches(exit, n, barriers, whileSet) {
			return false
		}
		if _, isCond := n.Statement.(*statements.ConditionStatement); isCond {
			return true
		}
		nonEncoded := 0
		for _, nx := range n.Next {
			if !encodedJumpTo(n, nx) {
				nonEncoded++
			}
		}
		return nonEncoded == 1
	}
	state := map[*core.Node]uint8{}
	statementBudget := 4096
	var visit func(*core.Node) bool
	visit = func(n *core.Node) bool {
		if n == exit {
			return true
		}
		if n == nil || len(state) >= 1024 || n.HideNext != nil || n.IsCatchStart || n.HasPendingTryDispatch() || n.HasProtectedRange || (n != owner && !utils2.IsDominate(manager.DominatorMap, owner, n)) {
			return false
		}
		if state[n] != 0 {
			return state[n] == 2
		}
		if n.HasOriginPC && !sameDomain(n) {
			return false
		}
		// A witnessed return/throw closes its own path, not the normal
		// continuation. Keep that original terminal inside the region.
		if forwardOriginalMethodExit(n) {
			state[n] = 2
			return true
		}
		switch n.Statement.(type) {
		case *statements.ConditionStatement, *statements.AssignStatement, *statements.ExpressionStatement, *values.JavaExpression:
			if !sameDomain(n) {
				return false
			}
		case *statements.IfStatement, *statements.DoWhileStatement, *statements.WhileStatement, *statements.TryCatchStatement:
			if !forwardBlockKeepsTransferBindings(n.Statement, 0, 0, &statementBudget) {
				return false
			}
		case *statements.CustomStatement:
			kind, label, ok := n.Statement.(*statements.CustomStatement).SourceTransferProtocol()
			if !ok || kind != "break" || len(n.Next) != 1 {
				return false
			}
			target := n.Next[0]
			rk := allowedExit[target]
			if rk == regionInvalid || label != allowedLabel[target] || !n.EncodedJumps[target] {
				return false
			}
		default:
			return false
		}
		if len(n.Next) == 0 {
			if forwardStructuredAbruptMethodExit(n.Statement, 0) {
				state[n] = 2
				return true
			}
			return false
		}
		state[n] = 1
		for _, next := range n.Next {
			remaining--
			if remaining < 0 {
				return false
			}
			if encodedJumpTo(n, next) {
				rk := allowedExit[next]
				if rk == regionInvalid {
					return false
				}
				if rk == regionOuter {
					// Escapes cleanly to an enclosing forward-region boundary.
					continue
				}
			} else if canSealSiblingEdge(n, next) {
				edge := forwardEdge{from: n, to: next}
				if !siblingSeen[edge] {
					siblingSeen[edge] = true
					siblingBreaks = append(siblingBreaks, edge)
				}
				continue
			}
			if !visit(next) {
				return false
			}
		}
		state[n] = 2
		return true
	}
	if !visit(owner) {
		return false, nil, nil
	}
	// Dominance alone does not prove single entry after choosing an exit:
	// the proposed exit might lead back into the collected interior. Check
	// all live incoming edges, not stale Source links from consumed bodies.
	// An edge into the owner is a fresh invocation/outer-loop iteration;
	// every other interior predecessor must belong to this same region.
	seen := map[*core.Node]bool{}
	pending := []*core.Node{manager.RootNode}
	for len(pending) > 0 {
		n := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		remaining--
		if n == nil || remaining < 0 || len(seen) >= 4096 || len(pending) > 4096 {
			return false, nil, nil
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		for _, next := range n.Next {
			remaining--
			if remaining < 0 || next != owner && state[next] != 0 && state[n] == 0 {
				return false, nil, nil
			}
			pending = append(pending, next)
		}
	}
	if needsTailCheck {
		tailState := map[*core.Node]uint8{}
		var visitTail func(*core.Node) bool
		visitTail = func(n *core.Node) bool {
			if n == nil || len(tailState) >= 512 || n == owner || !utils2.IsDominate(manager.DominatorMap, owner, n) || n.HideNext != nil || n.IsCatchStart || n.HasPendingTryDispatch() || n.HasProtectedRange {
				return false
			}
			if tailState[n] == 1 {
				return false
			}
			if tailState[n] == 2 {
				return true
			}
			if n.HasOriginPC && !sameDomain(n) {
				return false
			}
			if forwardOriginalMethodExit(n) {
				tailState[n] = 2
				return true
			}
			switch n.Statement.(type) {
			case *statements.ConditionStatement, *statements.AssignStatement, *statements.ExpressionStatement, *values.JavaExpression:
				if !sameDomain(n) {
					return false
				}
			case *statements.IfStatement, *statements.DoWhileStatement, *statements.WhileStatement, *statements.TryCatchStatement:
				if !forwardBlockKeepsTransferBindings(n.Statement, 0, 0, &statementBudget) {
					return false
				}
			case *statements.CustomStatement:
				kind, label, ok := n.Statement.(*statements.CustomStatement).SourceTransferProtocol()
				if !ok || kind != "break" || len(n.Next) != 1 {
					return false
				}
				target := n.Next[0]
				rk := allowedExit[target]
				if rk == regionInvalid || label != allowedLabel[target] || !n.EncodedJumps[target] {
					return false
				}
			default:
				return false
			}
			if len(n.Next) == 0 {
				if forwardStructuredAbruptMethodExit(n.Statement, 0) {
					tailState[n] = 2
					return true
				}
				return false
			}
			tailState[n] = 1
			for _, nx := range n.Next {
				remaining--
				if remaining < 0 {
					return false
				}
				if encodedJumpTo(n, nx) {
					rk := allowedExit[nx]
					if rk == regionInvalid {
						return false
					}
					if rk == regionOuter {
						continue
					}
				} else if ownerOuterLabel[nx] != "" {
					continue
				}
				if !visitTail(nx) {
					return false
				}
			}
			tailState[n] = 2
			return true
		}
		if !visitTail(exit) {
			return false, nil, nil
		}
	}
	return true, siblingBreaks, ownerOuterLabel
}

// A typed original method terminal completes abruptly. Its synthetic END
// successor is not an executable continuation or a forward-region frontier.
// Unknown origins, hidden scopes and a normal successor cannot establish this.
func forwardOriginalMethodExit(n *core.Node) bool {
	if n == nil || !n.HasOriginPC || n.HideNext != nil || n.IsCatchStart || n.HasPendingTryDispatch() || n.HasProtectedRange {
		return false
	}
	known := false
	switch st := n.Statement.(type) {
	case *statements.ReturnStatement:
		known = st != nil && st.HasOriginPC && st.OriginPC == n.OriginPC
	case *statements.CustomStatement:
		if st != nil {
			_, thrown := st.SourceThrowOperand()
			known = thrown && st.HasOriginPC && st.OriginPC == n.OriginPC
		}
	}
	if !known {
		return false
	}
	for _, next := range n.Next {
		if !IsEndNode(next) || encodedJumpTo(n, next) {
			return false
		}
	}
	return true
}

// Adding a one-shot loop must not capture an existing unqualified transfer.
// Transfers inside an already structured loop keep that nearer owner. Explicit
// labels keep their exact target. Opaque leaves and protected/monitor scopes
// provide no such certificate and are deliberately left to their own rewrites.
func forwardBlockKeepsTransferBindings(st statements.Statement, loops, depth int, remaining *int) bool {
	*remaining--
	if st == nil || depth > 64 || *remaining < 0 {
		return false
	}
	block := func(body []statements.Statement, loopDepth int) bool {
		for _, child := range body {
			if !forwardBlockKeepsTransferBindings(child, loopDepth, depth+1, remaining) {
				return false
			}
		}
		return true
	}
	switch x := st.(type) {
	case *statements.AssignStatement:
		return x != nil
	case *statements.ExpressionStatement:
		return x != nil
	case *statements.ReturnStatement:
		return x != nil
	case *values.JavaExpression:
		return x != nil
	case *statements.IfStatement:
		return x != nil && x.Condition != nil && block(x.IfBody, loops) && block(x.ElseBody, loops)
	case *statements.DoWhileStatement:
		return x != nil && x.ConditionValue != nil && block(x.Body, loops+1)
	case *statements.WhileStatement:
		return x != nil && x.ConditionValue != nil && block(x.Body, loops+1)
	case *statements.TryCatchStatement:
		// The complete protected statement stays atomic: only its enclosing
		// lexical block changes. A new loop must not capture an unqualified
		// transfer in either the original body or any original handler.
		if x == nil || len(x.Exception) == 0 || len(x.Exception) > 32 || len(x.Exception) != len(x.CatchBodies) || len(x.Handlers) != len(x.Exception) || !block(x.TryBody, loops) {
			return false
		}
		for i, handler := range x.Handlers {
			if x.Exception[i] == nil || handler.EntryPC < 0 || handler.EntryPC > 65535 {
				return false
			}
			if _, valid := canonicalHandlerRanges(handler.ProtectedRanges); !valid {
				return false
			}
			if !block(x.CatchBodies[i], loops) {
				return false
			}
		}
		return true
	case *statements.CustomStatement:
		if _, label, ok := x.SourceTransferProtocol(); ok {
			return label != "" || loops > 0
		}
		if _, ok := x.SourceThrowOperand(); ok {
			return true
		}
		_, ok := x.SourceAssignmentForward()
		return ok
	default:
		return false
	}
}

// When an enclosing if-owner already owns forward regions, its true/false arms
// can still exit to multiple distinct targets (for example parallel tails
// reaching an owned join or an owned continuation alongside an unowned arm).
// Seal excess arm-exit targets into owned/proved forward regions before IfRewriter
// collects the arms, leaving at most one normal fall-through continuation.
func sealOwnerForwardRegionArmExits(manager *RewriteManager, ifNode *core.Node) {
	if manager == nil || ifNode == nil || jdecenv.Get("JDEC_NESTED_FORWARD_REGION_OFF") == "1" || len(manager.forwardRegions[ifNode]) == 0 {
		return
	}
	registeredJoins := map[*core.Node]bool{}
	collectArmExits := func() ([]forwardEdge, []*core.Node) {
		trueArm, falseArm := ifBranchNodes(ifNode)
		arms := make([]*core.Node, 0, 2)
		if trueArm != nil {
			arms = append(arms, trueArm)
		}
		if falseArm != nil && falseArm != trueArm {
			arms = append(arms, falseArm)
		}
		protectedStops := typedProtectedIfCollectionStops(manager, ifNode)
		forwardStops := pendingForwardRegionExits(manager, ifNode)
		visited := map[*core.Node]bool{}
		var armNodes []*core.Node
		var rawEdges []forwardEdge
		for _, arm := range arms {
			if arm == nil {
				continue
			}
			if forwardStops[arm] || !slices.Contains(manager.DominatorMap[ifNode], arm) {
				if !IsEndNode(arm) && !encodedJumpTo(ifNode, arm) {
					rawEdges = append(rawEdges, forwardEdge{from: ifNode, to: arm})
				}
				continue
			}
			queue := []*core.Node{arm}
			for len(queue) > 0 && len(visited) < 2048 {
				n := queue[0]
				queue = queue[1:]
				if n == nil || visited[n] {
					continue
				}
				if forwardStops[n] || (protectedStops[n] && !protectedArmCanOwnReturn(n)) {
					continue
				}
				visited[n] = true
				armNodes = append(armNodes, n)
				for _, nx := range n.Next {
					if nx == nil || IsEndNode(nx) || encodedJumpTo(n, nx) {
						continue
					}
					if !forwardStops[nx] && !(protectedStops[nx] && !protectedArmCanOwnReturn(nx)) && slices.Contains(manager.DominatorMap[n], nx) {
						if !visited[nx] {
							queue = append(queue, nx)
						}
					} else {
						rawEdges = append(rawEdges, forwardEdge{from: n, to: nx})
					}
				}
			}
		}
		hasNormal := false
		ownedByIfNode := make(map[*core.Node]bool, len(manager.forwardRegions[ifNode]))
		for _, r := range manager.forwardRegions[ifNode] {
			if r != nil && r.exit != nil {
				ownedByIfNode[r.exit] = true
			}
		}
		distinctNonArmTargets := map[*core.Node]bool{}
		hasOwnedTarget := false
		for _, e := range rawEdges {
			if slices.Contains(armNodes, e.to) {
				continue
			}
			distinctNonArmTargets[e.to] = true
			if ownedByIfNode[e.to] {
				hasOwnedTarget = true
			}
			if !isMethodExitTerminator(e.to) {
				hasNormal = true
			}
		}
		includeAllExitTargets := !hasNormal && hasOwnedTarget && len(distinctNonArmTargets) >= 2
		if !hasNormal && !includeAllExitTargets {
			return nil, nil
		}
		var edges []forwardEdge
		var targets []*core.Node
		seenTargets := map[*core.Node]bool{}
		for _, e := range rawEdges {
			if slices.Contains(armNodes, e.to) {
				continue
			}
			if !includeAllExitTargets && isMethodExitTerminator(e.to) && !forwardStops[e.to] && !ownedByIfNode[e.to] {
				continue
			}
			edges = append(edges, e)
			if !seenTargets[e.to] {
				seenTargets[e.to] = true
				targets = append(targets, e.to)
			}
		}
		return edges, targets
	}
	sealBreaks := func(breaks []forwardEdge, labels map[*core.Node]string) {
		for _, sb := range breaks {
			if sb.from == nil || sb.to == nil || labels[sb.to] == "" || !slices.Contains(sb.from.Next, sb.to) || encodedJumpTo(sb.from, sb.to) {
				continue
			}
			jump := manager.NewNode(statements.NewSourceTransferStatement("break", labels[sb.to]))
			jump.AddNext(sb.to)
			jump.EncodedJumps = map[*core.Node]bool{sb.to: true}
			replaceNextInPlace(sb.from, sb.to, jump)
		}
	}
	sortOutermostFirst := func(list []*core.Node) []*core.Node {
		out := slices.Clone(list)
		for i := 1; i < len(out); i++ {
			for j := i; j > 0; j-- {
				nested, ordered := forwardExitNestedBefore(manager, ifNode, out[j-1], out[j])
				if ordered && nested {
					out[j-1], out[j] = out[j], out[j-1]
				}
			}
		}
		return out
	}

	for step := 0; step < 16; step++ {
		edges, targets := collectArmExits()
		if len(targets) <= 1 {
			return
		}
		ownedLabel := map[*core.Node]string{}
		for _, r := range manager.forwardRegions[ifNode] {
			if r != nil && r.exit != nil {
				ownedLabel[r.exit] = r.label
			}
		}
		var ownedTargets, unownedTargets []*core.Node
		for _, t := range targets {
			if ownedLabel[t] != "" {
				ownedTargets = append(ownedTargets, t)
			} else {
				unownedTargets = append(unownedTargets, t)
			}
		}
		if len(unownedTargets) >= 2 {
			orderedUnowned := sortOutermostFirst(unownedTargets)
			tryRegister := func() (bool, *forwardRegion, []forwardEdge, map[*core.Node]string) {
				for _, cand := range orderedUnowned {
					ok, siblingBreaks, outerLabels := closedForwardRegionWithSiblingExits(manager, ifNode, cand)
					if ok {
						reg := &forwardRegion{exit: cand, label: manager.NewLoopLabel()}
						return true, reg, siblingBreaks, outerLabels
					}
				}
				return false, nil, nil, nil
			}
			registered, reg, sb, outerLabels := tryRegister()
			if !registered {
				if join := findCommonForwardJoin(unownedTargets, ifNode, registeredJoins); join != nil {
					if okJ, sbJ, lblJ := closedForwardRegionWithSiblingExits(manager, ifNode, join); okJ && ownedLabel[join] == "" {
						registeredJoins[join] = true
						joinRegion := &forwardRegion{exit: join, label: manager.NewLoopLabel()}
						manager.insertForwardRegion(ifNode, joinRegion)
						registered, reg, sb, outerLabels = tryRegister()
						if registered {
							sealBreaks(sbJ, lblJ)
						} else {
							list := manager.forwardRegions[ifNode]
							kept := list[:0]
							for _, r := range list {
								if r != joinRegion {
									kept = append(kept, r)
								}
							}
							manager.forwardRegions[ifNode] = kept
						}
					}
				}
			}
			if !registered {
				return
			}
			manager.insertForwardRegion(ifNode, reg)
			sealBreaks(sb, outerLabels)
			manager.refreshDominatorsShared()
			continue
		}
		if len(ownedTargets) == 0 {
			return
		}
		var sealTarget *core.Node
		orderedOwned := sortOutermostFirst(ownedTargets)
		if len(unownedTargets) == 1 {
			u := unownedTargets[0]
			if nested, ordered := forwardExitNestedBefore(manager, ifNode, orderedOwned[0], u); ordered && nested {
				if okU, sbU, lblU := closedForwardRegionWithSiblingExits(manager, ifNode, u); okU {
					regU := &forwardRegion{exit: u, label: manager.NewLoopLabel()}
					manager.insertForwardRegion(ifNode, regU)
					sealBreaks(sbU, lblU)
					manager.refreshDominatorsShared()
					continue
				}
			}
			sealTarget = orderedOwned[0]
		} else if len(orderedOwned) >= 2 {
			// All targets are owned: keep an innermost target as normal fall-through.
			sealTarget = orderedOwned[0]
		}
		if sealTarget == nil || ownedLabel[sealTarget] == "" {
			return
		}
		changed := false
		for _, e := range edges {
			if e.to != sealTarget || !slices.Contains(e.from.Next, sealTarget) || encodedJumpTo(e.from, sealTarget) {
				continue
			}
			jump := manager.NewNode(statements.NewSourceTransferStatement("break", ownedLabel[sealTarget]))
			jump.AddNext(sealTarget)
			jump.EncodedJumps = map[*core.Node]bool{sealTarget: true}
			replaceNextInPlace(e.from, sealTarget, jump)
			changed = true
		}
		if !changed {
			return
		}
		manager.refreshDominatorsShared()
	}
}

func forwardStructuredAbruptMethodExit(st statements.Statement, depth int) bool {
	if st == nil || depth > 32 {
		return false
	}
	switch s := st.(type) {
	case *statements.ReturnStatement:
		return s != nil && s.HasOriginPC
	case *statements.CustomStatement:
		if s == nil {
			return false
		}
		_, thrown := s.SourceThrowOperand()
		return thrown && s.HasOriginPC
	case *statements.IfStatement:
		if s == nil || len(s.IfBody) == 0 || len(s.ElseBody) == 0 {
			return false
		}
		lastStmt := func(body []statements.Statement) statements.Statement {
			for i := len(body) - 1; i >= 0; i-- {
				if body[i] != nil {
					return body[i]
				}
			}
			return nil
		}
		return forwardStructuredAbruptMethodExit(lastStmt(s.IfBody), depth+1) &&
			forwardStructuredAbruptMethodExit(lastStmt(s.ElseBody), depth+1)
	case *statements.SwitchStatement:
		if s == nil || len(s.Cases) == 0 {
			return false
		}
		lastStmt := func(body []statements.Statement) statements.Statement {
			for i := len(body) - 1; i >= 0; i-- {
				if body[i] != nil {
					return body[i]
				}
			}
			return nil
		}
		hasDefault := false
		pendingFallthrough := false
		for _, item := range s.Cases {
			if item == nil {
				continue
			}
			if item.IsDefault {
				hasDefault = true
			}
			if len(item.Body) == 0 {
				pendingFallthrough = true
				continue
			}
			pendingFallthrough = false
			if !forwardStructuredAbruptMethodExit(lastStmt(item.Body), depth+1) {
				return false
			}
		}
		return hasDefault && !pendingFallthrough
	case *statements.TryCatchStatement:
		if s == nil || len(s.TryBody) == 0 {
			return false
		}
		lastStmt := func(body []statements.Statement) statements.Statement {
			for i := len(body) - 1; i >= 0; i-- {
				if body[i] != nil {
					return body[i]
				}
			}
			return nil
		}
		if !forwardStructuredAbruptMethodExit(lastStmt(s.TryBody), depth+1) {
			return false
		}
		for _, cb := range s.CatchBodies {
			if !forwardStructuredAbruptMethodExit(lastStmt(cb), depth+1) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func finishForwardRegion(manager *RewriteManager, original, start *core.Node) error {
	regions := manager.forwardRegions[original]
	if len(regions) == 0 {
		return nil
	}
	var pending []*forwardRegion
	pendingByExit := map[*core.Node]*forwardRegion{}
	for _, r := range regions {
		if r != nil && r.exit != nil {
			pending = append(pending, r)
			pendingByExit[r.exit] = r
		}
	}
	otherOwnerExits := map[*core.Node]bool{}
	for owner, list := range manager.forwardRegions {
		if owner == original {
			continue
		}
		for _, r := range list {
			if r != nil && r.exit != nil {
				otherOwnerExits[r.exit] = true
			}
		}
	}

	barriers := forwardEnclosingBarriers(manager, original, start)
	whileSet := manager.whileNodeSet()
	domByOwner := make(map[*core.Node]bool, len(pending))
	for _, r := range pending {
		domByOwner[r.exit] = len(manager.DominatorMap) > 0 && utils2.IsDominate(manager.DominatorMap, original, r.exit)
	}
	mustCloseBefore := func(a, b *forwardRegion) bool {
		if a == nil || b == nil || a == b {
			return false
		}
		aToB := forwardExitReaches(a.exit, b.exit, barriers, whileSet)
		bToA := forwardExitReaches(b.exit, a.exit, barriers, whileSet)
		if aToB && bToA {
			return true
		}
		if aToB {
			return true
		}
		if bToA {
			return false
		}
		if domByOwner[a.exit] && !domByOwner[b.exit] {
			return true
		}
		if !domByOwner[a.exit] && domByOwner[b.exit] {
			return false
		}
		return false
	}

	pickTopologicalSource := func(candidates []*forwardRegion, preferred map[*core.Node]bool, reachFilter *forwardRegion) (*forwardRegion, bool) {
		var best *forwardRegion
		bestPreferred := false
		bestDom := false
		for _, cand := range candidates {
			if reachFilter != nil && (cand == reachFilter || !mustCloseBefore(cand, reachFilter)) {
				continue
			}
			isSource := true
			for _, other := range pending {
				if other != cand && mustCloseBefore(other, cand) {
					isSource = false
					break
				}
			}
			if !isSource {
				continue
			}
			pref := preferred[cand.exit]
			candDom := domByOwner[cand.exit]
			if best == nil || (pref && !bestPreferred) || (pref == bestPreferred && candDom && !bestDom) {
				best = cand
				bestPreferred = pref
				bestDom = candDom
			}
		}
		return best, best != nil
	}

	for len(pending) > 0 {
		var body []*core.Node
		seen := map[*core.Node]bool{}
		var outerTargets []*core.Node
		outerSeen := map[*core.Node]bool{}
		var hitExit *forwardRegion
		for n := start; ; {
			if n == nil || seen[n] || len(seen) >= 1024 {
				return fmt.Errorf("forward region did not close")
			}
			if n != start {
				if hit := pendingByExit[n]; hit != nil {
					hitExit = hit
					break
				}
			}
			if _, raw := n.Statement.(*statements.ConditionStatement); raw {
				return fmt.Errorf("forward region contains an unstructured decision")
			}
			seen[n] = true
			body = append(body, n)
			// The remaining linear arm can itself end the method. Its END
			// sentinel is bookkeeping, never a statement or region fall-through.
			if forwardOriginalMethodExit(n) || (len(n.Next) == 0 && forwardStructuredAbruptMethodExit(n.Statement, 0)) {
				break
			}
			var normal []*core.Node
			for _, next := range n.Next {
				if encodedJumpTo(n, next) {
					if pendingByExit[next] != nil || otherOwnerExits[next] {
						if !outerSeen[next] {
							outerSeen[next] = true
							outerTargets = append(outerTargets, next)
						}
						continue
					}
					return fmt.Errorf("forward region has a foreign transfer")
				}
				normal = append(normal, next)
			}
			// The last statement may complete only by already encoded
			// region breaks (other arms may return/throw). It has no ordinary
			// fall-through edge, but reaches exact region boundaries.
			if len(normal) == 0 && len(n.Next) > 0 {
				break
			}
			if len(normal) != 1 {
				return fmt.Errorf("forward region has %d normal continuations", len(normal))
			}
			n = normal[0]
		}

		var chosen *forwardRegion
		if hitExit != nil {
			// Never close hitExit while an earlier pending region targeted by
			// an encoded break in body or strictly dominated inner to hitExit must close first.
			needsEarlier := false
			for _, other := range pending {
				if other != hitExit && mustCloseBefore(other, hitExit) && (outerSeen[other.exit] || (domByOwner[other.exit] && !domByOwner[hitExit.exit])) {
					needsEarlier = true
					break
				}
			}
			if !needsEarlier {
				chosen = hitExit
			} else if src, ok := pickTopologicalSource(pending, outerSeen, hitExit); ok {
				chosen = src
				jump := manager.NewNode(statements.NewSourceTransferStatement("break", hitExit.label))
				body = append(body, jump)
				if !outerSeen[hitExit.exit] {
					outerSeen[hitExit.exit] = true
					outerTargets = append(outerTargets, hitExit.exit)
				}
			} else {
				return fmt.Errorf("forward region exits form a cycle")
			}
		} else {
			src, ok := pickTopologicalSource(pending, outerSeen, nil)
			if !ok {
				return fmt.Errorf("forward region exits form a cycle")
			}
			chosen = src
		}

		block := statements.NewDoWhileStatement(values.NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean)), core.NodesToStatements(body))
		block.Label = chosen.label
		wrapper := manager.NewNode(block)
		manager.replaceStructuredNode(start, wrapper)
		if manager.RootNode == start {
			manager.RootNode = wrapper
		}
		wrapper.RemoveAllNext()
		for _, n := range body {
			n.RemoveAllNext()
		}
		wrapper.AddNext(chosen.exit)
		for _, outerTarget := range outerTargets {
			if outerTarget == chosen.exit {
				continue
			}
			wrapper.AddNext(outerTarget)
			if wrapper.EncodedJumps == nil {
				wrapper.EncodedJumps = map[*core.Node]bool{}
			}
			wrapper.EncodedJumps[outerTarget] = true
		}
		retargetProtectedIfBoundary(manager.TryNodes, start, wrapper)
		start = wrapper
		barriers[wrapper] = true

		delete(pendingByExit, chosen.exit)
		nextPending := pending[:0]
		for _, r := range pending {
			if r != chosen {
				nextPending = append(nextPending, r)
			}
		}
		pending = nextPending
	}
	delete(manager.forwardRegions, original)
	return nil
}

// Pending region exits name graph identities, so every structured replacement
// must retarget those identities together with the incoming CFG edges. The
// source break label still names the same lexical region and is unchanged.
func (manager *RewriteManager) replaceStructuredNode(old, replacement *core.Node) {
	old.Replace(replacement)
	for _, list := range manager.forwardRegions {
		for _, region := range list {
			if region == nil {
				continue
			}
			if region.exit == old {
				region.exit = replacement
			}
			if region.protectedEnd == old {
				region.protectedEnd = replacement
			}
		}
	}
}
