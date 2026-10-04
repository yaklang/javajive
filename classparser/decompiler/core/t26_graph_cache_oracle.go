package core

// Independent slow idom/postdom reference used only by tests.
// Production GraphAnalysisCache / ValidateReducible must not call these functions.
// Algorithm: classical iterative dataflow set-intersection, not the cached CHKEN IDom.

func t26SlowImmediateDominators(n int, succs [][]int, roots []int) []int {
	idom := make([]int, n)
	for i := range idom {
		idom[i] = -1
	}
	if n == 0 {
		return idom
	}
	roots = uniqueInts(roots)
	reach := reachableFrom(n, succs, roots)
	filtered := make([]int, 0, len(roots))
	for _, r := range roots {
		if r >= 0 && r < n && reach[r] {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		return idom
	}
	preds := make([][]int, n)
	for u := 0; u < n; u++ {
		if !reach[u] {
			continue
		}
		for _, v := range succs[u] {
			if v >= 0 && v < n && reach[v] {
				preds[v] = append(preds[v], u)
			}
		}
	}
	// Virtual source predecessors: every filtered root.
	dom := make([][]bool, n)
	all := make([]bool, n)
	for i := 0; i < n; i++ {
		if reach[i] {
			all[i] = true
		}
	}
	for i := 0; i < n; i++ {
		if !reach[i] {
			continue
		}
		dom[i] = make([]bool, n)
		isRoot := false
		for _, r := range filtered {
			if r == i {
				isRoot = true
				break
			}
		}
		if isRoot {
			dom[i][i] = true
			continue
		}
		copy(dom[i], all)
	}
	changed := true
	for changed {
		changed = false
		for i := 0; i < n; i++ {
			if !reach[i] {
				continue
			}
			isRoot := false
			for _, r := range filtered {
				if r == i {
					isRoot = true
					break
				}
			}
			if isRoot {
				continue
			}
			// Intersection of predecessor dominator sets. Roots have a virtual pred
			// that dominates nothing except through the root itself.
			var acc []bool
			got := false
			for _, p := range preds[i] {
				if !reach[p] || dom[p] == nil {
					continue
				}
				if !got {
					acc = append([]bool(nil), dom[p]...)
					got = true
					continue
				}
				for k := 0; k < n; k++ {
					acc[k] = acc[k] && dom[p][k]
				}
			}
			if !got {
				continue
			}
			acc[i] = true
			same := true
			for k := 0; k < n; k++ {
				if acc[k] != dom[i][k] {
					same = false
					break
				}
			}
			if !same {
				dom[i] = acc
				changed = true
			}
		}
	}
	for i := 0; i < n; i++ {
		if !reach[i] || dom[i] == nil {
			continue
		}
		isRoot := false
		for _, r := range filtered {
			if r == i {
				isRoot = true
				break
			}
		}
		if isRoot {
			idom[i] = i
			continue
		}
		// Closest strict dominator: unique member of (dom[i]\{i}) dominated by every other member.
		cands := make([]int, 0, n)
		for d := 0; d < n; d++ {
			if d != i && dom[i][d] {
				cands = append(cands, d)
			}
		}
		best := -1
		for _, c := range cands {
			ok := true
			for _, o := range cands {
				if o == c {
					continue
				}
				if dom[c] == nil || !dom[c][o] {
					ok = false
					break
				}
			}
			if ok {
				best = c
				break
			}
		}
		if best >= 0 {
			idom[i] = best
		} else if len(cands) == 1 {
			idom[i] = cands[0]
		} else {
			idom[i] = i
		}
	}
	return idom
}

func t26SlowImmediatePostDominators(n int, succs [][]int, roots []int) []int {
	ipdom := make([]int, n)
	for i := range ipdom {
		ipdom[i] = -1
	}
	if n == 0 {
		return ipdom
	}
	reach := reachableFrom(n, succs, roots)
	exits := make([]bool, n)
	for i := 0; i < n; i++ {
		if !reach[i] {
			continue
		}
		alive := false
		for _, v := range succs[i] {
			if v >= 0 && v < n && reach[v] {
				alive = true
				break
			}
		}
		if !alive {
			exits[i] = true
		}
	}
	canExit := canReachExitMarks(n, succs, reach, exits)
	ve := n
	total := n + 1
	fwd := make([][]int, total)
	for i := 0; i < n; i++ {
		if !reach[i] {
			continue
		}
		for _, v := range succs[i] {
			if v >= 0 && v < n && reach[v] {
				fwd[i] = append(fwd[i], v)
			}
		}
		if exits[i] || !canExit[i] {
			fwd[i] = append(fwd[i], ve)
		}
	}
	rev := make([][]int, total)
	for u := 0; u < total; u++ {
		for _, v := range fwd[u] {
			rev[v] = append(rev[v], u)
		}
	}
	// Slow dominators on the reverse CFG from the virtual exit.
	slow := t26SlowImmediateDominators(total, rev, []int{ve})
	for i := 0; i < n; i++ {
		if !reach[i] {
			continue
		}
		d := slow[i]
		if d == ve {
			ipdom[i] = VirtualExitSentinel
		} else if d >= 0 && d < n && d != i {
			ipdom[i] = d
		} else if d == i {
			ipdom[i] = i
		}
	}
	return ipdom
}
