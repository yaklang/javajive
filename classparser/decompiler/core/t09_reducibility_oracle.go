package core

// Independent slow reducibility oracle used only by tests.
// Production ValidateReducible must not import or call these functions.
// Dominance is computed by iterative dataflow set intersection (not T26 CHKEN).
// A normal edge u→v is a back-edge iff v dominates u; the remaining graph must be a DAG.

func t09OracleReducible(n int, succs [][]int, roots []int) (bool, string) {
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
		ok, diag := t09OracleDomain(n, succs, root)
		if !ok {
			return false, diag
		}
	}
	return true, ""
}

func t09OracleDomain(n int, succs [][]int, root int) (bool, string) {
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
			if idomDominates(idom, v, u) {
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
	succs := g.successorIndexLists(false)
	idx := g.nodeIndexMap()
	roots := make([]int, 0, 4)
	for _, r := range g.normalFlowRoots() {
		if i, ok := idx[r]; ok {
			roots = append(roots, i)
		}
	}
	return t09OracleReducible(n, succs, roots)
}
