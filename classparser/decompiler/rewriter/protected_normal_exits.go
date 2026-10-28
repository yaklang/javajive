package rewriter

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	utils2 "github.com/yaklang/javajive/classparser/decompiler/utils"
)

// An exception interval can have two normal exits: one commits an original
// return and another continues after the protected computation. Keep both
// exits outside its handler by sealing the latter as a break from a one-shot
// block containing the try and its original returning continuation.
func materializeProtectedNormalExit(manager *RewriteManager, condition *core.Node) {
	if manager == nil || !manager.originalHandlersKnown || len(manager.TryNodes) > 256 || len(manager.originalHandlers) > 512 || condition == nil || !condition.HasOriginPC {
		return
	}
	for _, owner := range manager.TryNodes {
		if owner == nil || !owner.HasProtectedRange || manager.forwardRegions[owner] != nil ||
			owner.HideNext != nil || len(owner.EncodedJumps) != 0 || !protectedExitOwnerHandlerKnown(manager, owner) ||
			owner.ProtectedStartPC < 0 || owner.ProtectedStartPC >= owner.ProtectedEndPC ||
			condition.OriginPC < owner.ProtectedStartPC || condition.OriginPC >= owner.ProtectedEndPC ||
			!utils2.IsDominate(manager.DominatorMap, owner, condition) || len(owner.Next) != 2 {
			continue
		}
		var start *core.Node
		for _, next := range owner.Next {
			if next != nil && !next.IsCatchStart {
				if start != nil {
					return
				}
				start = next
			}
		}
		end := typedTryNormalBoundary(owner, start, owner.Next, manager.DominatorMap)
		// A direct RETURN/THROW is already an abrupt arm owned by the if
		// collector. Only an ordinary returning continuation needs this
		// two-normal-exit region; wrapping a direct exit conflicts with that
		// existing ownership and can leave the region with no continuation.
		if end == nil || forwardOriginalMethodExit(end) || !protectedReturningTail(manager, end, owner) {
			continue
		}
		inside := func(pc int) bool { return pc >= owner.ProtectedStartPC && pc < owner.ProtectedEndPC }
		ranges := [][2]int{{owner.ProtectedStartPC, owner.ProtectedEndPC}}
		outside := func(pc int) bool { return pc >= 0 && !inside(pc) }
		pending := []*core.Node{condition}
		seen := map[*core.Node]bool{}
		var edges []forwardEdge
		valid := true
		for len(pending) > 0 && valid {
			n := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if n == nil || len(seen) >= 256 || n.HideNext != nil || len(n.EncodedJumps) != 0 || n.IsCatchStart || n.IsTryCatch || n.HasProtectedRange {
				valid = false
				break
			}
			if seen[n] {
				continue
			}
			seen[n] = true
			if n != condition && !utils2.IsDominate(manager.DominatorMap, condition, n) {
				valid = false
				break
			}
			if !protectedNormalNodeCovered(manager, owner, n, inside) || len(n.Next) == 0 {
				valid = false
				break
			}
			for _, next := range n.Next {
				if next == nil {
					valid = false
					break
				}
				if next == end {
					edges = append(edges, forwardEdge{n, next})
					continue
				}
				p := outsideHandlerDomainProof(ranges)
				if (!next.HasOriginPC || !inside(next.OriginPC)) && p.block([]statements.Statement{next.Statement}, outside, 0) {
					edges = append(edges, forwardEdge{n, next})
				} else {
					pending = append(pending, next)
				}
			}
		}
		var alternate *core.Node
		sawEnd := false
		for _, edge := range edges {
			if edge.to == end {
				sawEnd = true
				continue
			}
			if alternate != nil && alternate != edge.to {
				valid = false
				break
			}
			alternate = edge.to
		}
		allowed := map[*core.Node]bool{}
		for _, edge := range edges {
			if edge.to == alternate {
				allowed[edge.from] = true
			}
		}
		if !valid || !sawEnd || alternate == nil || !typedOutsideFrontierClosed(alternate, outside) ||
			!protectedPrefixClosed(manager, owner, start, end, alternate, inside, allowed) || protectedReaches(alternate, end) {
			continue
		}
		region := &forwardRegion{exit: alternate, protectedEnd: end, label: manager.NewLoopLabel()}
		if manager.forwardRegions == nil {
			manager.forwardRegions = map[*core.Node]*forwardRegion{}
		}
		manager.forwardRegions[owner] = region
		for _, edge := range edges {
			if edge.to != alternate {
				continue
			}
			jump := manager.NewNode(statements.NewSourceTransferStatement("break", region.label))
			jump.AddNext(alternate)
			jump.EncodedJumps = map[*core.Node]bool{alternate: true}
			replaceNextInPlace(edge.from, alternate, jump)
		}
		manager.DominatorMap = GenerateDominatorTree(manager.RootNode)
		return
	}
}

func protectedExitOwnerHandlerKnown(m *RewriteManager, owner *core.Node) bool {
	entry, ok := owner.Statement.(*statements.MiddleStatement)
	if !ok || entry == nil || entry.Flag != statements.MiddleTryStart {
		return false
	}
	for _, handler := range owner.Next {
		if handler == nil || !handler.IsCatchStart {
			continue
		}
		if handler.CatchHandler == nil || !handler.HasOriginPC || handler.OriginPC != handler.CatchHandler.EntryPC {
			return false
		}
		for _, row := range m.originalHandlers {
			if int(row.StartPc) == owner.ProtectedStartPC && int(row.EndPc) == owner.ProtectedEndPC && int(row.HandlerPc) == handler.OriginPC && row.CatchType != 0 {
				return true
			}
		}
	}
	return false
}

func protectedReturningTail(m *RewriteManager, start, owner *core.Node) bool {
	if !utils2.IsDominate(m.DominatorMap, owner, start) {
		return false
	}
	// Moving this tail into one lexical block requires an exclusive entry.
	// A shared later return would otherwise disappear from its other owner.
	rows := [][2]int{{owner.ProtectedStartPC, owner.ProtectedEndPC}}
	proof := outsideHandlerDomainProof(rows)
	outside := func(pc int) bool { return pc >= 0 && !finallyContains(rows, pc) }
	seen := map[*core.Node]bool{}
	for n := start; n != nil && !seen[n] && len(seen) < 128; {
		seen[n] = true
		if n != start && !utils2.IsDominate(m.DominatorMap, start, n) || n.HideNext != nil || n.IsCatchStart || n.IsTryCatch || n.HasProtectedRange || len(n.EncodedJumps) != 0 || !proof.block([]statements.Statement{n.Statement}, outside, 0) {
			return false
		}
		if forwardOriginalMethodExit(n) {
			return true
		}
		if len(n.Next) != 1 {
			return false
		}
		n = n.Next[0]
	}
	return false
}

func protectedNormalNodeCovered(m *RewriteManager, owner, n *core.Node, inside func(int) bool) bool {
	if n.HasOriginPC && !inside(n.OriginPC) && !forwardOriginalMethodExit(n) {
		return false
	}
	switch st := n.Statement.(type) {
	case *statements.ConditionStatement:
		return st != nil && n.HasOriginPC && len(n.Next) == 2 && finallyCoveredValue(st.Condition, inside)
	case *values.JavaExpression:
		return finallyCoveredValue(st, inside)
	case *statements.DoWhileStatement:
		return st != nil && len(st.Body) == 0 && slices.Contains(m.WhileNode, n) && finallyCoveredValue(st.ConditionValue, inside)
	case *statements.CustomStatement:
		if _, _, known := st.SourceTransferProtocol(); known {
			return protectedTransferOwned(m, owner, n, st)
		}
	}
	remaining := 512
	var visit func(statements.Statement, int) bool
	visit = func(statement statements.Statement, depth int) bool {
		remaining--
		if statement == nil || depth > 24 || remaining < 0 {
			return false
		}
		switch st := statement.(type) {
		case *statements.IfStatement:
			if st == nil || !finallyCoveredValue(st.Condition, inside) {
				return false
			}
			for _, body := range [][]statements.Statement{st.IfBody, st.ElseBody} {
				for _, child := range body {
					if !visit(child, depth+1) {
						return false
					}
				}
			}
			return true
		case *values.JavaExpression:
			return finallyCoveredValue(st, inside)
		case *statements.CustomStatement:
			if _, _, known := st.SourceTransferProtocol(); known {
				return protectedTransferOwned(m, owner, n, st)
			}
		}
		p := handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}}
		return p.block([]statements.Statement{statement}, inside, 0)
	}
	return visit(n.Statement, 0)
}

func protectedTransferOwned(m *RewriteManager, owner, container *core.Node, leaf *statements.CustomStatement) bool {
	kind, label, known := leaf.SourceTransferProtocol()
	if !known || len(m.loopTransfers) > 1024 {
		return false
	}
	for node, r := range m.loopTransfers {
		if node == nil || node.Statement != leaf || r.kind != kind || r.target == nil || !slices.Contains(m.WhileNode, r.target) || !utils2.IsDominate(m.DominatorMap, owner, r.target) {
			continue
		}
		loop, ok := r.target.Statement.(*statements.DoWhileStatement)
		return ok && loop != nil && (label != "" && label == loop.Label || label == "" && utils2.IsDominate(m.DominatorMap, r.target, container))
	}
	return false
}

func protectedPrefixClosed(m *RewriteManager, owner, start, end, alternate *core.Node, inside func(int) bool, allowed map[*core.Node]bool) bool {
	seen := map[*core.Node]bool{}
	pending := []*core.Node{start}
	remaining := 4096
	for len(pending) > 0 {
		n := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		remaining--
		if n == end || n == alternate {
			continue
		}
		if n == nil || remaining < 0 || len(seen) >= 512 || n.HideNext != nil || n.IsCatchStart || n.IsTryCatch || n.HasProtectedRange || !utils2.IsDominate(m.DominatorMap, owner, n) {
			return false
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		if !protectedNormalNodeCovered(m, owner, n, inside) {
			return false
		}
		if forwardOriginalMethodExit(n) {
			continue
		}
		if len(n.Next) == 0 {
			return false
		}
		for _, next := range n.Next {
			if next == alternate && !allowed[n] {
				return false
			}
		}
		pending = append(pending, n.Next...)
	}
	// Check live predecessors. Consumed graph nodes can retain stale Source
	// entries; only the currently reachable graph establishes single entry.
	visited := map[*core.Node]bool{}
	pending = []*core.Node{m.RootNode}
	for len(pending) > 0 {
		n := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		remaining--
		if n == nil || remaining < 0 {
			return false
		}
		if visited[n] {
			continue
		}
		visited[n] = true
		for _, next := range n.Next {
			if seen[next] && !seen[n] && !(n == owner && next == start) {
				return false
			}
			pending = append(pending, next)
		}
	}
	return true
}

// Exhaustion is possible reachability, never a non-reentry certificate.
func protectedReaches(start, target *core.Node) bool {
	seen := map[*core.Node]bool{}
	pending := []*core.Node{start}
	remaining := 4096
	for len(pending) > 0 {
		n := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		remaining--
		if n == nil || remaining < 0 || n == target {
			return true
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		pending = append(pending, n.Next...)
	}
	return false
}
