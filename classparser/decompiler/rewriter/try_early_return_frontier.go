package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

// An exclusive endpoint at a pure early RETURN does not identify the normal
// continuation. Find that continuation across the other normal CFG paths.
// A frontier must contain actual outside effects and no protected effects;
// pure control/local prefixes can stay with their arm without changing which
// operations a handler covers. No rendered text or successor ordering is used.
func typedEarlyReturnNormalFrontier(region, start *core.Node, successors []*core.Node, dom map[*core.Node][]*core.Node) *core.Node {
	if region == nil || start == nil || !region.HasProtectedRange || region.ProtectedStartPC < 0 || region.ProtectedStartPC >= region.ProtectedEndPC || !protectedArmCanOwnReturn(region.ProtectedEnd) || region.ProtectedEnd.OriginPC != region.ProtectedEndPC {
		return nil
	}
	var handler *core.Node
	for _, n := range successors {
		if n != nil && n.IsCatchStart {
			if handler != nil || n.CatchHandler == nil || n.CatchHandler.CatchAll {
				return nil
			}
			handler = n
		}
	}
	if handler == nil || !typedHandlerTerminatesInCollectedRegion(handler, dom) {
		return nil
	}
	ranges, known := canonicalHandlerRanges(handler.CatchHandler.ProtectedRanges)
	if !known || len(ranges) != 1 || ranges[0] != [2]int{region.ProtectedStartPC, region.ProtectedEndPC} {
		return nil
	}
	inside := func(pc int) bool { return pc >= region.ProtectedStartPC && pc < region.ProtectedEndPC }
	outside := func(pc int) bool { return pc >= 0 && !inside(pc) }
	queue := []*core.Node{start}
	seen := map[*core.Node]bool{}
	var frontier *core.Node
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if n == nil || len(seen) >= 512 || n.IsCatchStart {
			return nil
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		if IsEndNode(n) {
			continue
		}
		if len(n.EncodedJumps) != 0 {
			return nil
		}
		if protectedArmCanOwnReturn(n) {
			continue
		}
		if _, jump := n.Statement.(*statements.GOTOStatement); jump {
			if len(n.Next) != 1 {
				return nil
			}
			queue = append(queue, n.Next[0])
			continue
		}
		inProof := handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}}
		in := inProof.block([]statements.Statement{n.Statement}, inside, 0)
		if condition, raw := n.Statement.(*statements.ConditionStatement); raw {
			if condition == nil || !n.HasOriginPC || len(n.Next) != 2 || !finallyCoveredValue(condition.Condition, inside) {
				return nil
			}
			in = true
		}
		if !in {
			outProof := handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}}
			if !outProof.block([]statements.Statement{n.Statement}, outside, 0) {
				return nil
			}
			if frontier != nil && frontier != n {
				return nil
			}
			frontier = n
			continue
		}
		if isMethodExitTerminator(n) || func() bool {
			v, ok := n.Statement.(*statements.IfStatement)
			return ok && typedStructuredHandlerTerminates(v)
		}() {
			for _, next := range n.Next {
				if !IsEndNode(next) {
					return nil
				}
			}
			continue
		}
		if len(n.Next) == 0 {
			return nil
		}
		queue = append(queue, n.Next...)
	}
	if frontier == nil || !typedOutsideFrontierClosed(frontier, outside) {
		return nil
	}
	return frontier
}

// Cutting a normal edge must not move a later re-entry into the protected
// domain outside its handler. Prove every reachable continuation effect, not
// just the first outside statement. Unknown transfers/trees fail closed.
func typedOutsideFrontierClosed(start *core.Node, outside func(int) bool) bool {
	queue := []*core.Node{start}
	seen := map[*core.Node]bool{}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if n == nil || len(seen) >= 512 || n.IsCatchStart {
			return false
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		if IsEndNode(n) {
			continue
		}
		if len(n.EncodedJumps) != 0 {
			return false
		}
		switch value := n.Statement.(type) {
		case *statements.GOTOStatement:
			if len(n.Next) != 1 {
				return false
			}
		case *statements.ConditionStatement:
			if value == nil || len(n.Next) != 2 || !finallyCoveredValue(value.Condition, outside) {
				return false
			}
		default:
			proof := handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}}
			if !proof.block([]statements.Statement{n.Statement}, outside, 0) {
				return false
			}
		}
		queue = append(queue, n.Next...)
	}
	return true
}
