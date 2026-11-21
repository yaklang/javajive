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

// forwardExitReachesWithoutOwner reports whether goal is reachable from from
// along normal CFG edges without passing through owner or crossing END.
func forwardExitReachesWithoutOwner(from, goal, owner *core.Node) bool {
	if from == nil || goal == nil || from == owner {
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
		if n == nil || seen[n] || IsEndNode(n) || n == owner {
			continue
		}
		if n == goal {
			return true
		}
		seen[n] = true
		for _, next := range n.Next {
			if next == goal {
				return true
			}
			if !seen[next] && next != owner && !IsEndNode(next) {
				queue = append(queue, next)
			}
		}
	}
	return false
}

// forwardTerminalChainWithoutReaching reports whether every path starting from
// from terminates at an original method exit without reaching avoided or owner,
// within a small dominator-safe DAG bound.
func forwardTerminalChainWithoutReaching(from, avoided, owner *core.Node) bool {
	if from == nil || from == avoided || from == owner {
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
		if n == avoided || n == owner || IsEndNode(n) || len(n.Next) == 0 {
			return false
		}
		seen[n] = true
		if forwardOriginalMethodExit(n) {
			continue
		}
		for _, next := range n.Next {
			if next == nil || next == avoided || next == owner {
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
func forwardExitNestedBefore(owner, innerExit, outerExit *core.Node) (nested bool, ordered bool) {
	if innerExit == nil || outerExit == nil || innerExit == outerExit {
		return false, false
	}
	innerToOuter := forwardExitReachesWithoutOwner(innerExit, outerExit, owner)
	outerToInner := forwardExitReachesWithoutOwner(outerExit, innerExit, owner)
	if innerToOuter && outerToInner {
		return false, false
	}
	if innerToOuter {
		return true, true
	}
	if outerToInner {
		return false, true
	}
	// An inner branch whose tail terminates in return/throw sits strictly inside an
	// enclosing region targeting a post-if continuation that never returns to it.
	if forwardTerminalChainWithoutReaching(innerExit, outerExit, owner) && !forwardExitReachesWithoutOwner(outerExit, innerExit, owner) {
		return true, true
	}
	if forwardTerminalChainWithoutReaching(outerExit, innerExit, owner) && !forwardExitReachesWithoutOwner(innerExit, outerExit, owner) {
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
		nested, ordered := forwardExitNestedBefore(owner, list[i].exit, list[i-1].exit)
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
					nested, ordered := forwardExitNestedBefore(condition, candidates[j-1], candidates[j])
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
		nestedInner, ordered := forwardExitNestedBefore(owner, r.exit, exit)
		if !ordered {
			return false, nil, nil
		}
		if nestedInner {
			allowedExit[r.exit] = regionInner
		} else {
			allowedExit[r.exit] = regionOuter
			ownerOuterLabel[r.exit] = r.label
		}
		allowedLabel[r.exit] = r.label
	}
	if !singleOnly {
		for anc, regions := range manager.forwardRegions {
			if anc == nil || anc == owner || !utils2.IsDominate(manager.DominatorMap, anc, owner) {
				continue
			}
			for _, r := range regions {
				if r == nil || r.exit == nil || r.exit == owner || r.exit == exit {
					continue
				}
				if _, already := allowedExit[r.exit]; already {
					continue
				}
				if forwardExitReachesWithoutOwner(r.exit, exit, anc) {
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
		if singleOnly || ownerOuterLabel[target] == "" || n == exit || forwardExitReachesWithoutOwner(exit, n, owner) {
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

func finishForwardRegion(manager *RewriteManager, original, start *core.Node) error {
	regions := manager.forwardRegions[original]
	if len(regions) == 0 {
		return nil
	}
	for idx, region := range regions {
		if region == nil {
			continue
		}
		allowedOuter := map[*core.Node]bool{}
		for owner, list := range manager.forwardRegions {
			for j, r := range list {
				if r == nil || r.exit == nil || r.exit == region.exit {
					continue
				}
				if owner == original && j < idx {
					continue
				}
				allowedOuter[r.exit] = true
			}
		}
		var body []*core.Node
		seen := map[*core.Node]bool{}
		var outerTargets []*core.Node
		outerSeen := map[*core.Node]bool{}
		for n := start; n != region.exit; {
			if n == nil || seen[n] || len(seen) >= 1024 {
				return fmt.Errorf("forward region did not close")
			}
			if _, raw := n.Statement.(*statements.ConditionStatement); raw {
				return fmt.Errorf("forward region contains an unstructured decision")
			}
			seen[n] = true
			body = append(body, n)
			// The remaining linear arm can itself end the method. Its END
			// sentinel is bookkeeping, never a statement or region fall-through.
			if forwardOriginalMethodExit(n) {
				break
			}
			var normal []*core.Node
			for _, next := range n.Next {
				if encodedJumpTo(n, next) {
					if next == region.exit {
						continue
					}
					if allowedOuter[next] {
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
		block := statements.NewDoWhileStatement(values.NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean)), core.NodesToStatements(body))
		block.Label = region.label
		wrapper := manager.NewNode(block)
		manager.replaceStructuredNode(start, wrapper)
		if manager.RootNode == start {
			manager.RootNode = wrapper
		}
		wrapper.RemoveAllNext()
		for _, n := range body {
			n.RemoveAllNext()
		}
		wrapper.AddNext(region.exit)
		for _, outerTarget := range outerTargets {
			wrapper.AddNext(outerTarget)
			if wrapper.EncodedJumps == nil {
				wrapper.EncodedJumps = map[*core.Node]bool{}
			}
			wrapper.EncodedJumps[outerTarget] = true
		}
		retargetProtectedIfBoundary(manager.TryNodes, start, wrapper)
		start = wrapper
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
