package core

// Independent slow reducibility oracle used only by tests.
// Production ValidateReducible must not import or call these functions.
// Dominance is computed by iterative dataflow set intersection (not T26 CHKEN).
// A normal edge u→v is a back-edge iff v dominates u; the remaining graph must be a DAG.

func t09OracleReducible(n int, succs [][]int, roots []int, context []int) (bool, string) {
	if n == 0 {
		return true, ""
	}
	roots = uniqueInts(roots)
	if len(roots) == 0 {
		roots = []int{0}
	}
	for _, root := range roots {
		if root < 0 || root >= n {
			continue
		}
		inherited := context
		if root == 0 || !idomDominates(context, 0, root) {
			inherited = nil
		}
		ok, diag := t09OracleDomain(n, succs, root, inherited)
		if !ok {
			return false, diag
		}
	}
	return true, ""
}

func t09OracleDomain(n int, succs [][]int, root int, context []int) (bool, string) {
	idom := t26SlowImmediateDominators(n, succs, []int{root})
	inDomain := func(i int) bool {
		return i >= 0 && i < n && idom[i] >= 0
	}
	forward := make([][]int, n)
	for u := 0; u < n; u++ {
		if !inDomain(u) {
			continue
		}
		for _, v := range succs[u] {
			if !inDomain(v) {
				continue
			}
			if idomDominates(idom, v, u) || idomDominates(context, v, u) {
				continue
			}
			forward[u] = append(forward[u], v)
		}
	}
	if cycle := firstForwardCycle(forward); cycle >= 0 {
		return false, "irreducible"
	}
	return true, ""
}

func t09OracleFromCFG(g *SemanticCFG) (bool, string) {
	n := len(g.Nodes)
	idx := make(map[*OpCode]int, n)
	for i, node := range g.Nodes {
		idx[node] = i
	}
	succs, full := make([][]int, n), make([][]int, n)
	roots := []int{0}
	for _, e := range g.Edges {
		u, ok1 := idx[e.From]
		v, ok2 := idx[e.To]
		if !ok1 || !ok2 {
			continue
		}
		full[u] = append(full[u], v)
		if e.Kind == EdgeException {
			roots = append(roots, v)
		} else {
			succs[u] = append(succs[u], v)
		}
	}
	return t09OracleReducible(n, succs, roots, t26SlowImmediateDominators(n, full, []int{0}))
}
