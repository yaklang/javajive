package rewriter

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	utils2 "github.com/yaklang/javajive/classparser/decompiler/utils"
)

// A protected selection can leave normally either through an unprotected
// commit or by skipping that commit. Represent this as one lexical block
// around the original try and commit, with an explicit break for the skip.
// Neither the commit nor the loop update is copied or moved into the catch.
func materializeProtectedCommitExit(m *RewriteManager, condition *core.Node) {
	if m == nil || !m.originalHandlersKnown || len(m.originalHandlers) > 512 || len(m.TryNodes) > 256 || condition == nil || !condition.HasOriginPC {
		return
	}
	for _, owner := range m.TryNodes {
		if owner == nil || !owner.HasProtectedRange || m.forwardRegions[owner] != nil || owner.HideNext != nil || len(owner.EncodedJumps) != 0 || len(owner.Next) != 2 || !protectedExitOwnerHandlerKnown(m, owner) || !utils2.IsDominate(m.DominatorMap, owner, condition) {
			continue
		}
		start, handler := owner.Next[0], owner.Next[1]
		if start != nil && start.IsCatchStart {
			start, handler = handler, start
		}
		if start == nil || handler == nil || !handler.IsCatchStart || handler.HideNext != nil || len(handler.EncodedJumps) != 0 || handler.CatchHandler == nil || handler.CatchHandler.CatchAll || !handler.HasOriginPC || handler.OriginPC != handler.CatchHandler.EntryPC {
			continue
		}
		ranges, valid := canonicalHandlerRanges(handler.CatchHandler.ProtectedRanges)
		if !valid || !typedTryHandlerRangesMatch(owner, handler, ranges) || !finallyContains(ranges, condition.OriginPC) || !typedHandlerTerminatesInCollectedRegion(handler, m.DominatorMap) {
			continue
		}
		// Certify the complete physical dispatch, not only the region's summary.
		// Competing/overlapping handlers require a separate nested-domain proof.
		var physical [][2]int
		var catchType uint16
		valid = true
		for _, row := range m.originalHandlers {
			if row.StartPc >= row.EndPc {
				valid = false
				break
			}
			overlaps := false
			for _, r := range ranges {
				overlaps = overlaps || int(row.StartPc) < r[1] && int(row.EndPc) > r[0]
			}
			if !overlaps && int(row.HandlerPc) != handler.OriginPC {
				continue
			}
			if row.CatchType == 0 || int(row.HandlerPc) != handler.OriginPC || catchType != 0 && catchType != row.CatchType {
				valid = false
				break
			}
			catchType = row.CatchType
			physical = append(physical, [2]int{int(row.StartPc), int(row.EndPc)})
		}
		rows, known := canonicalHandlerRanges(physical)
		if !valid || !known || !slices.Equal(rows, ranges) {
			continue
		}
		inside := func(pc int) bool { return finallyContains(ranges, pc) }
		outside := func(pc int) bool { return pc >= 0 && !inside(pc) }
		state := map[*core.Node]uint8{}
		fronts := map[*core.Node]bool{}
		var edges []forwardEdge
		var visit func(*core.Node) bool
		visit = func(n *core.Node) bool {
			if n == nil || len(state) >= 256 || n.HideNext != nil || n.IsCatchStart || n.HasPendingTryDispatch() || n.HasProtectedRange || len(n.EncodedJumps) != 0 || len(n.Next) > 2 {
				return false
			}
			if n.HasOriginPC && !inside(n.OriginPC) {
				p := outsideHandlerDomainProof(ranges)
				if !p.block([]statements.Statement{n.Statement}, outside, 0) {
					return false
				}
				fronts[n] = true
				return len(fronts) <= 2
			}
			if state[n] != 0 {
				return state[n] == 2
			}
			if !utils2.IsDominate(m.DominatorMap, owner, n) || !protectedNormalNodeCovered(m, owner, n, inside) {
				return false
			}
			state[n] = 1
			if !forwardOriginalMethodExit(n) && len(n.Next) == 0 {
				return false
			}
			for _, next := range n.Next {
				if !visit(next) {
					return false
				}
				if fronts[next] {
					edges = append(edges, forwardEdge{n, next})
				}
			}
			state[n] = 2
			return true
		}
		if !visit(start) || len(fronts) != 2 || state[condition] == 0 {
			continue
		}
		// All entries into the protected prefix must pass through this try.
		live := protectedCommitLiveGraph(m.RootNode)
		if live == nil {
			continue
		}
		for _, n := range live {
			if n.HideNext != nil && state[n.HideNext] != 0 {
				valid = false
			}
			for _, next := range n.Next {
				if state[next] != 0 && state[n] == 0 && !(n == owner && next == start) {
					valid = false
				}
			}
		}
		if !valid {
			continue
		}
		var commit, alternate *core.Node
		var tail map[*core.Node]bool
		for a := range fronts {
			for b := range fronts {
				if a == b {
					continue
				}
				if nodes := protectedOutsideCommitTail(m, owner, a, b, ranges, live, state); nodes != nil {
					if commit != nil {
						valid = false
					}
					commit, alternate, tail = a, b, nodes
				}
			}
		}
		if !valid || commit == nil || len(tail) == 0 {
			continue
		}
		region := &forwardRegion{exit: alternate, protectedEnd: commit, protectedRanges: slices.Clone(ranges), label: m.NewLoopLabel()}
		if m.forwardRegions == nil {
			m.forwardRegions = map[*core.Node]*forwardRegion{}
		}
		m.forwardRegions[owner] = region
		for _, edge := range edges {
			if edge.to == alternate {
				jump := m.NewNode(statements.NewSourceTransferStatement("break", region.label))
				jump.AddNext(alternate)
				jump.EncodedJumps = map[*core.Node]bool{alternate: true}
				replaceNextInPlace(edge.from, alternate, jump)
			}
		}
		m.DominatorMap = GenerateDominatorTree(m.RootNode)
		return
	}
}

// The commit is an exclusive linear prefix ending at the other frontier.
// Effects retain their node identities and their entire original domains.
func protectedOutsideCommitTail(m *RewriteManager, owner, start, end *core.Node, ranges [][2]int, live []*core.Node, protected map[*core.Node]uint8) map[*core.Node]bool {
	seen := map[*core.Node]bool{}
	outside := func(pc int) bool { return pc >= 0 && !finallyContains(ranges, pc) }
	for n := start; n != end; {
		if n == nil || seen[n] || len(seen) >= 128 || n.HideNext != nil || n.IsCatchStart || n.HasPendingTryDispatch() || n.HasProtectedRange || len(n.EncodedJumps) != 0 || len(n.Next) != 1 || !utils2.IsDominate(m.DominatorMap, owner, n) {
			return nil
		}
		switch n.Statement.(type) {
		case *statements.AssignStatement, *statements.ExpressionStatement:
		default:
			return nil
		}
		p := outsideHandlerDomainProof(ranges)
		if !n.HasOriginPC || !outside(n.OriginPC) || !p.block([]statements.Statement{n.Statement}, outside, 0) {
			return nil
		}
		seen[n] = true
		n = n.Next[0]
	}
	for _, n := range live {
		if n.HideNext != nil && seen[n.HideNext] {
			return nil
		}
		for _, next := range n.Next {
			if seen[next] && !seen[n] && !(next == start && protected[n] != 0) {
				return nil
			}
		}
	}
	return seen
}

func protectedCommitLiveGraph(root *core.Node) []*core.Node {
	seen := map[*core.Node]bool{}
	pending := []*core.Node{root}
	var result []*core.Node
	remaining := 8192
	for len(pending) != 0 {
		n := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		remaining--
		if n == nil || remaining < 0 || len(seen) >= 4096 {
			return nil
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		if len(n.Next) > remaining {
			return nil
		}
		result = append(result, n)
		pending = append(pending, n.Next...)
		if n.HideNext != nil {
			pending = append(pending, n.HideNext)
		}
	}
	return result
}
