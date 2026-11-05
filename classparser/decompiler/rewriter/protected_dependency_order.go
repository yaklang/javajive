package rewriter

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

// Only a shared protected successor creates this extra dependency. Keep all
// other branch ordering and each complete dominator subtree intact. A loop's
// transfer/cleanup ownership cannot be established from forward reachability.
func protectedSiblingRewriteOrder(m *RewriteManager, base []*core.Node) []*core.Node {
	if m == nil || m.RootNode == nil || !m.originalHandlersKnown || len(m.originalHandlers) > 512 || len(m.TryNodes) == 0 || len(m.TryNodes) > 256 || len(m.DominatorMap) > 4096 {
		return base
	}
	nodes := []*core.Node{}
	seen := map[*core.Node]bool{}
	todo := []*core.Node{m.RootNode}
	budget := 65536
	for len(todo) > 0 {
		n := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		if n == nil || n.HideNext != nil || len(seen) > 4096 {
			return base
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		nodes = append(nodes, n)
		for _, next := range n.Next {
			budget--
			if budget < 0 {
				return base
			}
			todo = append(todo, next)
		}
	}
	parent := map[*core.Node]*core.Node{}
	for n, children := range m.DominatorMap {
		if !seen[n] {
			return base
		}
		for _, child := range children {
			budget--
			if budget < 0 {
				return base
			}
			if child == nil || !seen[child] || parent[child] != nil {
				return base
			}
			parent[child] = n
		}
	}
	if parent[m.RootNode] != nil || len(parent) != len(seen)-1 {
		return base
	}
	containsTry := map[*core.Node]bool{}
	for _, owner := range m.TryNodes {
		if owner != nil {
			budget -= len(owner.Next) * (len(m.originalHandlers) + 1) * (len(owner.SharedProtectedRanges) + 1)
		}
		if budget < 0 {
			return base
		}
		if owner == nil || !seen[owner] || !owner.HasProtectedRange || !pendingTypedTryRowsKnown(m, owner) {
			continue
		}
		for n, depth := owner, 0; n != nil; n, depth = parent[n], depth+1 {
			if depth > 1024 {
				return base
			}
			containsTry[n] = true
		}
	}
	cyclic := cyclicCFGNodes(m.RootNode)
	needs := map[*core.Node]map[*core.Node]map[*core.Node]bool{}
	sameDomain := func(a, b *core.Node) bool {
		if a == nil || b == nil || !a.HasOriginPC || !b.HasOriginPC || a.OriginPC < 0 || a.OriginPC > 65535 || b.OriginPC < 0 || b.OriginPC > 65535 {
			return false
		}
		for _, row := range m.originalHandlers {
			budget--
			if budget < 0 {
				return false
			}
			inside := func(pc int) bool { return pc >= int(row.StartPc) && pc < int(row.EndPc) }
			if inside(a.OriginPC) != inside(b.OriginPC) {
				return false
			}
		}
		return true
	}
	for _, from := range nodes {
		for _, next := range from.Next {
			if cyclic[from] || cyclic[next] || next.IsCatchStart || encodedJumpTo(from, next) || !containsTry[next] {
				continue
			}
			owner := parent[next]
			if owner == nil {
				continue
			}
			if cond, ok := owner.Statement.(*statements.ConditionStatement); !ok || cond == nil || cond.Condition == nil {
				continue
			}
			if !sameDomain(owner, from) || !sameDomain(owner, next) {
				continue
			}
			sibling := from
			for depth := 0; sibling != nil && parent[sibling] != owner; depth++ {
				if depth > 1024 {
					return base
				}
				sibling = parent[sibling]
			}
			if sibling == nil || sibling == next {
				continue
			}
			if needs[owner] == nil {
				needs[owner] = map[*core.Node]map[*core.Node]bool{}
			}
			if needs[owner][sibling] == nil {
				needs[owner][sibling] = map[*core.Node]bool{}
			}
			needs[owner][sibling][next] = true
		}
	}
	if len(needs) == 0 || budget < 0 {
		return base
	}
	selected := map[*core.Node]bool{}
	for _, n := range base {
		selected[n] = true
	}
	type frame struct {
		node   *core.Node
		finish bool
	}
	stack := []frame{{node: m.RootNode}}
	visited := map[*core.Node]bool{}
	out := make([]*core.Node, 0, len(base))
	for len(stack) > 0 {
		item := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n := item.node
		if item.finish {
			if selected[n] {
				out = append(out, n)
			}
			continue
		}
		if n == nil || visited[n] || len(visited) > 4096 {
			return base
		}
		visited[n] = true
		children := slices.Clone(m.DominatorMap[n])
		var complete bool
		children, complete = stableProtectedDependencies(children, needs[n], &budget)
		if !complete {
			return base
		}
		stack = append(stack, frame{node: n, finish: true})
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, frame{node: children[i]})
		}
	}
	if len(out) != len(base) || len(visited) != len(seen) {
		return base
	}
	return out
}

// Preserve the complete original handler packet. A typed handler can share
// its interval with javac's catch-all cleanup; seeing only one row is not a
// proof that the other handler may be omitted. Compare ranges and dispatch
// entries without relying on generated catch names or successor order.
func pendingTypedTryRowsKnown(m *RewriteManager, owner *core.Node) bool {
	if m == nil || !m.originalHandlersKnown || len(m.originalHandlers) > 512 || owner == nil || !owner.HasProtectedRange || owner.ProtectedStartPC < 0 || owner.ProtectedStartPC >= owner.ProtectedEndPC || owner.ProtectedEndPC > 65535 || len(owner.SharedProtectedRanges) > 512 || len(owner.Next) < 2 || len(owner.Next) > 33 {
		return false
	}
	st, ok := owner.Statement.(*statements.MiddleStatement)
	if !ok || st == nil || st.Flag != statements.MiddleTryStart {
		return false
	}
	expected := map[int]bool{}
	typed := false
	for _, row := range m.originalHandlers {
		if int(row.StartPc) == owner.ProtectedStartPC && int(row.EndPc) == owner.ProtectedEndPC {
			expected[int(row.HandlerPc)] = true
			typed = typed || row.CatchType != 0
		}
	}
	if !typed {
		return false
	}
	actual := map[int]bool{}
	normals := 0
	for _, next := range owner.Next {
		if next == nil {
			return false
		}
		if !next.IsCatchStart {
			normals++
			continue
		}
		h := next.CatchHandler
		if h == nil || !next.HasOriginPC || next.OriginPC != h.EntryPC || !expected[h.EntryPC] || actual[h.EntryPC] || len(h.ProtectedRanges) > 512 {
			return false
		}
		actual[h.EntryPC] = true
		var ranges [][2]int
		all := true
		for _, row := range m.originalHandlers {
			if int(row.HandlerPc) == h.EntryPC {
				ranges = append(ranges, [2]int{int(row.StartPc), int(row.EndPc)})
				all = all && row.CatchType == 0
			}
		}
		original, valid := canonicalHandlerRanges(ranges)
		captured, known := canonicalHandlerRanges(h.ProtectedRanges)
		if !valid || !known || all != h.CatchAll || !slices.Equal(original, captured) {
			return false
		}
	}
	if normals != 1 || len(actual) != len(expected) {
		return false
	}
	// This field records a sole handler's original rows even for a single
	// interval. Every recorded row must have an actual dispatch destination.
	for _, row := range owner.SharedProtectedRanges {
		if !actual[int(row.HandlerPc)] || !slices.Contains(m.originalHandlers, row) {
			return false
		}
	}
	return true
}

func stableProtectedDependencies(base []*core.Node, needs map[*core.Node]map[*core.Node]bool, budget *int) ([]*core.Node, bool) {
	if len(needs) == 0 {
		return base, true
	}
	left := map[*core.Node]bool{}
	for _, n := range base {
		*budget--
		if n == nil || left[n] || *budget < 0 {
			return base, false
		}
		left[n] = true
	}
	for n, deps := range needs {
		if !left[n] {
			return base, false
		}
		for dep := range deps {
			*budget--
			if !left[dep] || *budget < 0 {
				return base, false
			}
		}
	}
	out := make([]*core.Node, 0, len(base))
	for len(out) < len(base) {
		var chosen *core.Node
		for _, n := range base {
			*budget--
			if *budget < 0 {
				return base, false
			}
			if !left[n] {
				continue
			}
			ready := true
			for dependency := range needs[n] {
				*budget--
				if *budget < 0 {
					return base, false
				}
				if left[dependency] {
					ready = false
					break
				}
			}
			if ready {
				chosen = n
				break
			}
		}
		if chosen == nil {
			return base, false
		}
		delete(left, chosen)
		out = append(out, chosen)
	}
	return out, true
}
