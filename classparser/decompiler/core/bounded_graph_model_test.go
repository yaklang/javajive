package core

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

// E01's frozen normal-edge grammar has 3,634 graph/root-set pairs. Vertex
// deletion is the primary oracle; independently scheduled set equations are
// a second oracle. No production graph, reachability or idom helper is reused.
// Postdominance is checked in the documented augmented finite-exit domain:
// each real sink and each vertex without a real-sink path has a virtual edge.
// This does not prove termination or cover typed exceptional transfers.
func TestBoundedNormalGraphAnalysisModel(t *testing.T) {
	inventory := sha256.New()
	models := 0
	for n := 1; n <= 3; n++ {
		for mask := 0; mask < 1<<(n*n); mask++ {
			succ := make([][]int, n)
			for u := 0; u < n; u++ {
				for v := 0; v < n; v++ {
					if mask&(1<<(u*n+v)) != 0 {
						succ[u] = append(succ[u], v)
					}
				}
			}
			for rootsMask := 1; rootsMask < 1<<n; rootsMask++ {
				roots := []int{}
				for i := 0; i < n; i++ {
					if rootsMask&(1<<i) != 0 {
						roots = append(roots, i)
					}
				}
				want := boundedVertexDeletionDominance(succ, roots)
				for schedule := 0; schedule < 3; schedule++ {
					sets := boundedDominanceFixedPoint(succ, roots, schedule)
					for a := 0; a < n; a++ {
						for b := 0; b < n; b++ {
							if (sets[b]&(1<<a) != 0) != want[a][b] {
								t.Fatalf("fixed-point schedule=%d graph=%d roots=%d a=%d b=%d", schedule, mask, rootsMask, a, b)
							}
						}
					}
				}
				g := t09CFGFromSuccs(succ, nil)
				dom := g.GetOrCompute(AnalysisDominators, GraphAnalysisDomain{Roots: roots})
				post := g.GetOrCompute(AnalysisPostDominators, GraphAnalysisDomain{Roots: roots})
				reachable := boundedReach(succ, roots, -1)
				reverse := boundedAugmentedExitReverse(succ, reachable)
				wantPost := boundedVertexDeletionDominance(reverse, []int{n})
				for a := 0; a < n; a++ {
					for b := 0; b < n; b++ {
						if dom.Dominates(a, b) != want[a][b] {
							t.Fatalf("dominance graph=%d/%d roots=%d a=%d b=%d got=%v want=%v", n, mask, rootsMask, a, b, dom.Dominates(a, b), want[a][b])
						}
						expected := reachable[b] && wantPost[a][b]
						if post.PostDominates(a, b) != expected {
							t.Fatalf("augmented postdominance graph=%d/%d roots=%d a=%d b=%d got=%v want=%v", n, mask, rootsMask, a, b, post.PostDominates(a, b), expected)
						}
					}
				}
				for schedule := 0; schedule < 3; schedule++ {
					sets := boundedDominanceFixedPoint(reverse, []int{n}, schedule)
					for a := 0; a < n; a++ {
						for b := 0; b < n; b++ {
							if (sets[b]&(1<<a) != 0) != wantPost[a][b] {
								t.Fatalf("reverse fixed-point schedule=%d graph=%d roots=%d", schedule, mask, rootsMask)
							}
						}
					}
				}
				if post.PostDominates(VirtualExitSentinel, roots[0]) {
					t.Fatal("virtual sink exposed as a physical return")
				}
				// A physical synthetic entry models every root set for the public
				// reducibility API, whose entry is node zero. Do not silently choose the
				// first root or feed exception edges into the normal graph.
				withEntry := make([][]int, n+1)
				for _, r := range roots {
					withEntry[0] = append(withEntry[0], r+1)
				}
				for u := range succ {
					for _, v := range succ[u] {
						withEntry[u+1] = append(withEntry[u+1], v+1)
					}
				}
				expectedReducible := r07Reducible(withEntry, 0)
				if err := t09CFGFromSuccs(withEntry, nil).ValidateReducible(); (err == nil) != expectedReducible {
					t.Fatalf("reducibility graph=%d/%d roots=%d expected=%v err=%v", n, mask, rootsMask, expectedReducible, err)
				}
				fmt.Fprintf(inventory, "%d\t%d\t%d\t%v\t%v\n", n, mask, rootsMask, want, wantPost)
				models++
			}
		}
	}
	if models != 3634 {
		t.Fatal(models)
	}
	t.Logf("E01 normal graph/root models=%d; deletion and three fixed-point schedules agree; inventory-sha256=%x", models, inventory.Sum(nil))
}

func boundedReach(succ [][]int, roots []int, blocked int) []bool {
	seen := make([]bool, len(succ))
	queue := []int{}
	for _, r := range roots {
		if r != blocked && !seen[r] {
			seen[r] = true
			queue = append(queue, r)
		}
	}
	for head := 0; head < len(queue); head++ {
		for _, v := range succ[queue[head]] {
			if v != blocked && !seen[v] {
				seen[v] = true
				queue = append(queue, v)
			}
		}
	}
	return seen
}

func boundedVertexDeletionDominance(succ [][]int, roots []int) [][]bool {
	before := boundedReach(succ, roots, -1)
	out := make([][]bool, len(succ))
	for a := range succ {
		after := boundedReach(succ, roots, a)
		out[a] = make([]bool, len(succ))
		for b := range succ {
			out[a][b] = before[b] && !after[b]
		}
	}
	return out
}

func boundedDominanceFixedPoint(succ [][]int, roots []int, schedule int) []uint64 {
	n := len(succ)
	reach := boundedReach(succ, roots, -1)
	sets := make([]uint64, n)
	isRoot := make([]bool, n)
	all := uint64(0)
	for _, r := range roots {
		isRoot[r] = true
	}
	for i := range succ {
		if reach[i] {
			all |= 1 << i
		}
	}
	for i := range sets {
		if reach[i] {
			sets[i] = all
		}
	}
	for iteration := 0; iteration <= n*n+1; iteration++ {
		changed := false
		for step := 0; step < n; step++ {
			v := step
			if schedule == 1 {
				v = n - 1 - step
			}
			if schedule == 2 {
				v = (step + iteration) % n
			}
			if !reach[v] {
				continue
			}
			meet := all
			if isRoot[v] {
				meet = 0
			} else {
				for u, next := range succ {
					if !reach[u] {
						continue
					}
					for _, w := range next {
						if w == v {
							meet &= sets[u]
						}
					}
				}
			}
			value := meet | 1<<v
			if sets[v] != value {
				sets[v] = value
				changed = true
			}
		}
		if !changed {
			return sets
		}
	}
	panic("finite monotone model failed to converge")
}

func boundedAugmentedExitReverse(succ [][]int, reach []bool) [][]int {
	n := len(succ)
	reverse := make([][]int, n+1)
	for u, next := range succ {
		if !reach[u] {
			continue
		}
		for _, v := range next {
			reverse[v] = append(reverse[v], u)
		}
		from := boundedReach(succ, []int{u}, -1)
		canExit := false
		for v := range succ {
			if from[v] && len(succ[v]) == 0 {
				canExit = true
			}
		}
		if len(next) == 0 || !canExit {
			reverse[n] = append(reverse[n], u)
		}
	}
	return reverse
}
