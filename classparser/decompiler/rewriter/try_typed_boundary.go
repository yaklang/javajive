package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"slices"
)

// A terminal typed handler does not make a normal continuation protected.
// Follow the exact decoded exclusive boundary through encoded jumps and stop
// only at an ordinary continuation whose entire effect tree lies outside every
// original handler range. The handler must terminate inside its own collected
// region: otherwise that continuation would also become reachable after catch.
// Return/break/continue ownership stays with its original normal arm, even when
// javac places the transfer outside the protected interval.
func typedTryNormalBoundary(region, start *core.Node, successors []*core.Node, dom map[*core.Node][]*core.Node) *core.Node {
	if region == nil || start == nil || !region.HasProtectedRange || region.ProtectedStartPC < 0 || region.ProtectedStartPC >= region.ProtectedEndPC {
		return nil
	}
	var rows [][2]int
	handlers := 0
	var handler *core.Node
	for _, next := range successors {
		if next == nil || !next.IsCatchStart {
			continue
		}
		handlers++
		handler = next
		if next.CatchHandler == nil || next.CatchHandler.CatchAll {
			return nil
		}
		rows = append(rows, next.CatchHandler.ProtectedRanges...)
	}
	if handlers != 1 {
		return nil
	}
	ranges, valid := canonicalHandlerRanges(rows)
	if !valid || len(ranges) != 1 || ranges[0] != [2]int{region.ProtectedStartPC, region.ProtectedEndPC} {
		return nil
	}
	candidate := region.ProtectedEnd
	seen := map[*core.Node]bool{}
	for candidate != nil && !seen[candidate] && len(seen) < 32 {
		seen[candidate] = true
		if _, jump := candidate.Statement.(*statements.GOTOStatement); !jump {
			break
		}
		if len(candidate.Next) != 1 {
			return nil
		}
		candidate = candidate.Next[0]
	}
	if candidate == nil || candidate.IsCatchStart {
		return nil
	}
	// A control transfer is not a movable continuation. Following its GOTO
	// and detaching a RETURN also destroys an enclosing retry loop's break.
	switch candidate.Statement.(type) {
	case *statements.AssignStatement, *statements.ExpressionStatement:
	default:
		return nil
	}
	if !typedHandlerTerminatesInCollectedRegion(handler, dom) {
		return nil
	}
	outside := func(pc int) bool { return pc >= 0 && !finallyContains(ranges, pc) }
	proof := handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}}
	if !proof.block([]statements.Statement{candidate.Statement}, outside, 0) {
		return nil
	}
	// ProtectedEnd can be an obsolete node after graph rewrites. Its original
	// pointer is evidence only while an actual normal CFG path still reaches it.
	queue := []*core.Node{start}
	visited := map[*core.Node]bool{}
	for len(queue) > 0 && len(visited) < 512 {
		n := queue[0]
		queue = queue[1:]
		if n == candidate {
			return candidate
		}
		if n == nil || visited[n] || n.IsCatchStart {
			continue
		}
		visited[n] = true
		queue = append(queue, n.Next...)
	}
	return nil
}

// Match the exact ownership rule used by TryRewriter's body collector. Merely
// reaching some later return is insufficient: an empty catch may fall through
// to a shared fallback, and a retry catch may resume its enclosing loop. Both
// leave this collected handler instead of terminating it. Do not mutate the
// graph while checking this bounded, closed proof.
func typedHandlerTerminatesInCollectedRegion(entry *core.Node, dom map[*core.Node][]*core.Node) bool {
	if entry == nil || dom == nil {
		return false
	}
	state := map[*core.Node]uint8{}
	var visit func(*core.Node) bool
	visit = func(n *core.Node) bool {
		if n == nil || len(state) >= 512 || n != entry && n.IsCatchStart {
			return false
		}
		if state[n] != 0 {
			return state[n] == 2
		}
		state[n] = 1
		terminal := false
		switch st := n.Statement.(type) {
		case *statements.ReturnStatement:
			terminal = true
		case *statements.CustomStatement:
			terminal = st.ThrownValue != nil && st.HasOriginPC && st.LoopTransferKind == ""
		case *statements.IfStatement:
			terminal = len(n.EncodedJumps) == 0 && typedStructuredHandlerTerminates(st)
		}
		if terminal {
			for _, next := range n.Next {
				if !IsEndNode(next) {
					return false
				}
			}
		} else {
			if len(n.Next) == 0 || len(n.EncodedJumps) != 0 {
				return false
			}
			for _, next := range n.Next {
				if !slices.Contains(dom[n], next) || !visit(next) {
					return false
				}
			}
		}
		state[n] = 2
		return true
	}
	return visit(entry)
}

// Structuring an if replaces its terminal branch nodes with one source node.
// Its lack of normal successors is a terminal only when both complete source
// arms terminate. Otherwise an empty arm, retry transfer or shared fallback
// must retain the collector's existing dominance/ownership requirements.
func typedStructuredHandlerTerminates(branch *statements.IfStatement) bool {
	proof := handlerLayerProof{remaining: 512, active: map[statements.Statement]bool{}}
	if !proof.block([]statements.Statement{branch}, func(pc int) bool { return pc >= 0 }, 0) {
		return false
	}
	var terminates func(statements.Statement, int) bool
	terminates = func(st statements.Statement, depth int) bool {
		if depth > 24 {
			return false
		}
		switch x := st.(type) {
		case *statements.ReturnStatement:
			return x != nil && x.HasOriginPC
		case *statements.CustomStatement:
			_, sealed := x.SourceThrowOperand()
			return sealed && x.HasOriginPC && x.LoopTransferKind == ""
		case *statements.IfStatement:
			return x != nil && len(x.IfBody) > 0 && len(x.ElseBody) > 0 &&
				terminates(x.IfBody[len(x.IfBody)-1], depth+1) && terminates(x.ElseBody[len(x.ElseBody)-1], depth+1)
		}
		return false
	}
	return terminates(branch, 0)
}
