package core

import "testing"

func TestT26ParentChainQueriesMatchExhaustiveReachability(t *testing.T) {
	for n := 0; n <= 4; n++ {
		parents := make([]int, n)
		var enumerate func(int)
		enumerate = func(pos int) {
			if pos < n {
				for p := -2; p <= n; p++ {
					parents[pos] = p
					enumerate(pos + 1)
				}
				return
			}
			// Independent transitive closure, including cycles, multiple roots,
			// unreachable nodes, virtual exits and out-of-domain parent edges.
			reach := make([][]bool, n)
			for i, p := range parents {
				reach[i] = make([]bool, n)
				if p >= 0 && p < n {
					reach[i][p] = true
				}
			}
			for k := 0; k < n; k++ {
				for i := 0; i < n; i++ {
					for j := 0; j < n; j++ {
						reach[i][j] = reach[i][j] || reach[i][k] && reach[k][j]
					}
				}
			}
			analysis := &GraphAnalysis{IDom: parents, IPDom: parents}
			for node := 0; node < n; node++ {
				for ancestor := 0; ancestor < n; ancestor++ {
					wantDom := parents[node] >= 0 && (node == ancestor || reach[node][ancestor])
					wantPost := parents[node] != -1 && (node == ancestor || reach[node][ancestor])
					if analysis.Dominates(ancestor, node) != wantDom || idomDominates(parents, ancestor, node) != wantDom || analysis.PostDominates(ancestor, node) != wantPost {
						t.Fatalf("parents=%v query=%d,%d wantDom=%v wantPost=%v", parents, ancestor, node, wantDom, wantPost)
					}
				}
				if analysis.PostDominates(VirtualExitSentinel, node) {
					t.Fatal("virtual exit is not a real postdominator")
				}
			}
			for _, node := range []int{-1, n, n + 1} {
				if analysis.Dominates(0, node) || idomDominates(parents, 0, node) || analysis.PostDominates(0, node) {
					t.Fatal("out-of-domain node")
				}
			}
		}
		enumerate(0)
	}
	var nilAnalysis *GraphAnalysis
	if nilAnalysis.Dominates(0, 0) || nilAnalysis.PostDominates(0, 0) {
		t.Fatal("nil snapshot")
	}
}

func TestT26ParentChainQueriesDoNotAllocateVisitedSets(t *testing.T) {
	const n = 4096
	parents := make([]int, n)
	for i := range parents {
		parents[i] = i - 1
	}
	parents[0] = 0
	a := &GraphAnalysis{IDom: parents, IPDom: parents}
	for _, query := range []struct {
		name string
		call func() bool
	}{{"dominance", func() bool { return a.Dominates(0, n-1) }}, {"loop forest dominance", func() bool { return idomDominates(parents, 0, n-1) }}, {"postdominance", func() bool { return a.PostDominates(0, n-1) }}} {
		t.Run(query.name, func(t *testing.T) {
			if !query.call() {
				t.Fatal("chain root")
			}
			if got := testing.AllocsPerRun(20, func() {
				if !query.call() {
					panic("lost root")
				}
			}); got != 0 {
				t.Fatalf("query allocations=%v; immutable parent traversal needs no visited set", got)
			}
		})
	}
}
