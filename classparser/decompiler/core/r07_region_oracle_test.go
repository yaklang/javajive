package core

import (
	"math/rand"
	"testing"
)

// These oracles consume the fixture adjacency directly. They deliberately do
// not call production reachability, dominators, root selection, or cycle code.
func r07Reach(succ [][]int, root, blocked int) []bool {
	seen := make([]bool, len(succ))
	if root == blocked {
		return seen
	}
	queue := []int{root}
	seen[root] = true
	for head := 0; head < len(queue); head++ {
		for _, next := range succ[queue[head]] {
			if next != blocked && !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return seen
}

func r07Dominance(succ [][]int, root int) [][]bool {
	before := r07Reach(succ, root, -1)
	matrix := make([][]bool, len(succ))
	for removed := range succ {
		after := r07Reach(succ, root, removed)
		matrix[removed] = make([]bool, len(succ))
		for node := range succ {
			matrix[removed][node] = before[node] && !after[node]
		}
	}
	return matrix
}

// Kahn elimination independently checks the graph after dominance backedges
// are removed; production uses a DFS color stack for its residual-cycle test.
func r07Reducible(succ [][]int, root int) bool {
	dom := r07Dominance(succ, root)
	reach := r07Reach(succ, root, -1)
	degree := make([]int, len(succ))
	remaining := 0
	for u := range succ {
		if !reach[u] {
			continue
		}
		remaining++
		for _, v := range succ[u] {
			if !dom[v][u] {
				degree[v]++
			}
		}
	}
	var queue []int
	for u := range succ {
		if reach[u] && degree[u] == 0 {
			queue = append(queue, u)
		}
	}
	for head := 0; head < len(queue); head++ {
		u := queue[head]
		remaining--
		for _, v := range succ[u] {
			if dom[v][u] {
				continue
			}
			degree[v]--
			if degree[v] == 0 {
				queue = append(queue, v)
			}
		}
	}
	return remaining == 0
}

func r07CheckMatrix(t *testing.T, got *GraphAnalysis, succ [][]int, root int) {
	t.Helper()
	want := r07Dominance(succ, root)
	for a := range succ {
		for b := range succ {
			if got.Dominates(a, b) != want[a][b] {
				t.Fatalf("root=%d dominates(%d,%d) got=%t want=%t adjacency=%v idom=%v", root, a, b, got.Dominates(a, b), want[a][b], succ, got.IDom)
			}
		}
	}
}

func TestR07AllThreeNodeGraphs(t *testing.T) {
	for mask := 0; mask < 1<<9; mask++ {
		succ := make([][]int, 3)
		for from := 0; from < 3; from++ {
			for to := 0; to < 3; to++ {
				if mask&(1<<(3*from+to)) != 0 {
					succ[from] = append(succ[from], to)
				}
			}
		}
		for root := 0; root < 3; root++ {
			// Force this root to be a handler as well as checking it directly.
			// Its exceptional edge must never become a normal graph edge.
			g := t09CFGFromSuccs(succ, [][2]int{{0, root}, {0, root}})
			got := g.GetOrCompute(AnalysisDominators, GraphAnalysisDomain{Roots: []int{root}})
			r07CheckMatrix(t, got, succ, root)
			want := r07Reducible(succ, 0) && r07Reducible(succ, root)
			if err := g.ValidateReducible(); (err == nil) != want {
				t.Fatalf("mask=%d handler=%d reducible=%t error=%v", mask, root, want, err)
			}
		}
	}
}

func TestR07RandomGraphsAndHandlerDomains(t *testing.T) {
	const seed = 2026092207
	rng := rand.New(rand.NewSource(seed))
	for trial := 0; trial < 240; trial++ {
		n := 4 + rng.Intn(13)
		succ := make([][]int, n)
		for u := range succ {
			for v := range succ {
				if rng.Intn(6) == 0 {
					succ[u] = append(succ[u], v)
				}
			}
		}
		h1, h2 := rng.Intn(n), rng.Intn(n)
		g := t09CFGFromSuccs(succ, [][2]int{{0, h1}, {0, h2}, {h1, h2}})
		want := true
		for _, root := range []int{0, h1, h2} {
			r07CheckMatrix(t, g.GetOrCompute(AnalysisDominators, GraphAnalysisDomain{Roots: []int{root}}), succ, root)
			want = want && r07Reducible(succ, root)
		}
		if err := g.ValidateReducible(); (err == nil) != want {
			t.Fatalf("seed=%d trial=%d roots=%v adjacency=%v err=%v", seed, trial, []int{0, h1, h2}, succ, err)
		}
	}
}

func TestR07RegionShapes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		succ      [][]int
		handlers  [][2]int
		reducible bool
	}{
		{"infinite", [][]int{{1}, {1}}, nil, true},
		{"nested_two_entry_cycle", [][]int{{1}, {2, 3}, {3}, {2, 1, 4}, {}}, nil, false},
		{"ordinary_nested_loop", [][]int{{1}, {2, 4}, {3, 1}, {2}, {}}, nil, true},
		{"shared_tail_handlers", [][]int{{4}, {3}, {3}, {4}, {}}, [][2]int{{0, 1}, {0, 2}}, true},
		{"independent_handler_cycle", [][]int{{}, {2, 3}, {3}, {2}}, [][2]int{{0, 1}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := t09CFGFromSuccs(tc.succ, tc.handlers)
			if err := g.ValidateReducible(); (err == nil) != tc.reducible {
				t.Fatalf("reducible=%t err=%v", tc.reducible, err)
			}
		})
	}
	// Switch alternatives form a natural loop without a fallthrough edge.
	g := t09Graph(3, t09Edge{0, 1, EdgeCase}, t09Edge{0, 2, EdgeDefault}, t09Edge{1, 0, EdgeCase}, t09Edge{2, 0, EdgeDefault})
	if err := g.ValidateReducible(); err != nil {
		t.Fatal(err)
	}
}

func TestR07ExitReachabilityAndPostdominanceDomain(t *testing.T) {
	// 0 can exit via 1, but can instead loop forever in 2. Node 3 is
	// unreachable from the analysis root and must not postdominate itself.
	succ := [][]int{{1, 2}, {}, {2}, {}}
	reach := []bool{true, true, true, false}
	exits := []bool{false, true, false, false}
	got := canReachExitMarks(4, succ, reach, exits)
	want := []bool{true, true, false, false}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("existential exit reachability=%v", got)
		}
	}
	g := t09CFGFromSuccs(succ, nil)
	post := g.GetOrCompute(AnalysisPostDominators, GraphAnalysisDomain{Roots: []int{0}})
	if post.PostDominates(1, 0) || post.PostDominates(1, 2) {
		t.Fatalf("exit reachability mistaken for mandatory exit: %v", post.IPDom)
	}
	if post.PostDominates(3, 3) {
		t.Fatal("unreachable node postdominates itself outside analysis domain")
	}
	if !post.PostDominates(2, 2) || !post.PostDominates(1, 1) {
		t.Fatal("reachable self-postdominance lost")
	}
	if post.PostDominates(VirtualExitSentinel, 2) {
		t.Fatal("virtual exit exposed as real node")
	}
}

func TestR07MutationChangesAnswers(t *testing.T) {
	m := NewMutableAnalysisGraph(3, nil)
	m.AddEdge(0, 1, false)
	m.AddEdge(1, 2, false)
	domain := GraphAnalysisDomain{Roots: []int{0}}
	check := func(succ [][]int) {
		t.Helper()
		r07CheckMatrix(t, m.GetOrCompute(AnalysisDominators, domain), succ, 0)
	}
	check([][]int{{1}, {2}, {}})
	before := m.GetOrCompute(AnalysisPostDominators, domain)
	if !before.PostDominates(1, 0) {
		t.Fatal("chain postdominance")
	}
	count, epoch := m.ComputeCount(), m.Epoch()
	m.AddEdge(0, 2, false)
	check([][]int{{1, 2}, {2}, {}})
	after := m.GetOrCompute(AnalysisPostDominators, domain)
	if after.PostDominates(1, 0) || m.Epoch() <= epoch || m.ComputeCount() != count+2 {
		t.Fatal("mutation reused old analysis")
	}
	if !before.PostDominates(1, 0) {
		t.Fatal("mutation changed returned snapshot")
	}
	m.DeleteEdge(0, 2, false)
	check([][]int{{1}, {2}, {}})
	if !m.GetOrCompute(AnalysisPostDominators, domain).PostDominates(1, 0) {
		t.Fatal("deletion reused bypass answer")
	}
	// Replacing a node changes both graph size and the dominance identities.
	fresh := m.ReplaceNode(1)
	if fresh != 3 {
		t.Fatal("unexpected replacement index")
	}
	check([][]int{{3}, {}, {}, {2}})
	// Roots and exception inclusion are separate cache dimensions.
	m.AddEdge(0, 2, true)
	check([][]int{{3}, {}, {}, {2}})
	r07CheckMatrix(t, m.GetOrCompute(AnalysisDominators, GraphAnalysisDomain{Roots: []int{0}, IncludeException: true}), [][]int{{3, 2}, {}, {}, {2}}, 0)
	r07CheckMatrix(t, m.GetOrCompute(AnalysisDominators, GraphAnalysisDomain{Roots: []int{3}}), [][]int{{3}, {}, {}, {2}}, 3)
}
