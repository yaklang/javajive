package core

// normalFlowRoots is the method entry plus every distinct EdgeException.To.
// Exception edges themselves are never back-edges and never SCC entries.
func (g *SemanticCFG) normalFlowRoots() []*OpCode {
	if len(g.Nodes) == 0 {
		return nil
	}
	roots := []*OpCode{g.Nodes[0]}
	seen := map[*OpCode]struct{}{g.Nodes[0]: {}}
	for _, e := range g.Edges {
		if e.Kind != EdgeException || e.To == nil {
			continue
		}
		if _, ok := seen[e.To]; ok {
			continue
		}
		seen[e.To] = struct{}{}
		roots = append(roots, e.To)
	}
	return roots
}

func (g *SemanticCFG) indexOfNode(n *OpCode) int {
	if n == nil {
		return -1
	}
	if g.order != nil {
		if i, ok := g.order[n]; ok {
			return i
		}
	}
	for i, node := range g.Nodes {
		if node == n {
			return i
		}
	}
	return -1
}

func firstForwardCycle(forward [][]int) int {
	const (
		white = 0
		grey  = 1
		black = 2
	)
	n := len(forward)
	color := make([]uint8, n)
	type frame struct{ u, i int }
	best := -1
	for start := 0; start < n; start++ {
		if color[start] != white {
			continue
		}
		color[start] = grey
		stack := []frame{{start, 0}}
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			if top.i < len(forward[top.u]) {
				v := forward[top.u][top.i]
				top.i++
				if v < 0 || v >= n {
					continue
				}
				switch color[v] {
				case grey:
					if best < 0 || v < best {
						best = v
					}
				case white:
					color[v] = grey
					stack = append(stack, frame{v, 0})
				}
				continue
			}
			color[top.u] = black
			stack = stack[:len(stack)-1]
		}
	}
	return best
}

func (g *SemanticCFG) validateDomainReducible(root *OpCode, rootIdx int, analysis *GraphAnalysis) error {
	n := len(g.Nodes)
	idx := g.nodeIndexMap()
	inDomain := func(i int) bool {
		return i >= 0 && i < len(analysis.IDom) && analysis.IDom[i] >= 0
	}
	forward := make([][]int, n)
	seen := make([]map[int]struct{}, n)
	for _, e := range g.Edges {
		if e.Kind == EdgeException {
			continue
		}
		u, ok1 := idx[e.From]
		v, ok2 := idx[e.To]
		if !ok1 || !ok2 || !inDomain(u) || !inDomain(v) {
			continue
		}
		if analysis.Dominates(v, u) {
			continue
		}
		if seen[u] == nil {
			seen[u] = map[int]struct{}{}
		}
		if _, dup := seen[u][v]; dup {
			continue
		}
		seen[u][v] = struct{}{}
		forward[u] = append(forward[u], v)
	}
	if cycle := firstForwardCycle(forward); cycle >= 0 {
		return irreducibleDiagnostic(g.Nodes[cycle].CurrentOffset, root.CurrentOffset, "")
	}
	_ = rootIdx
	return nil
}
