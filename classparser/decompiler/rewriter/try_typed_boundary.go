package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

// A terminal typed handler does not make a normal continuation protected.
// Follow the exact decoded exclusive boundary through encoded jumps and stop
// only at a reachable statement whose entire effect tree lies outside every
// original handler range. Folded protected producer effects must stay inside.
func typedTryNormalBoundary(region, start *core.Node, successors []*core.Node) *core.Node {
	if region == nil || start == nil || !region.HasProtectedRange || region.ProtectedStartPC < 0 || region.ProtectedStartPC >= region.ProtectedEndPC {
		return nil
	}
	var rows [][2]int
	handlers := 0
	for _, next := range successors {
		if next == nil || !next.IsCatchStart {
			continue
		}
		handlers++
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
