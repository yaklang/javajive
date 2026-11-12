package core

import "testing"

func t26ScalarGraph() (*MutableAnalysisGraph, GraphAnalysisDomain) {
	cache := NewGraphAnalysisCache(8)
	graph := NewMutableAnalysisGraph(5, cache)
	graph.AddEdge(0, 1, false)
	graph.AddEdge(0, 2, false)
	graph.AddEdge(1, 3, false)
	graph.AddEdge(2, 3, false)
	graph.AddEdge(3, 4, false)
	return graph, GraphAnalysisDomain{Roots: []int{0}}
}

func TestT26ScalarDominanceQueriesShareCachedAnalysis(t *testing.T) {
	graph, domain := t26ScalarGraph()

	if !graph.Dominates(domain, 0, 4) || !graph.Dominates(domain, 3, 4) {
		t.Fatal("expected entry and join to dominate the exit")
	}
	if graph.Dominates(domain, 1, 4) {
		t.Fatal("one arm must not dominate a join")
	}
	if !graph.Dominates(domain, 4, 4) {
		t.Fatal("reflexive dominance missing")
	}
	if graph.Dominates(domain, 4, 0) {
		t.Fatal("exit must not dominate the entry")
	}

	computes := graph.ComputeCount()
	if computes != 1 {
		t.Fatalf("scalar dominator queries computed %d times, want 1", computes)
	}
	for i := 0; i < 200; i++ {
		if !graph.Dominates(domain, 0, 4) || graph.Dominates(domain, 1, 4) {
			t.Fatal("scalar query changed on an unchanged epoch")
		}
	}
	if graph.ComputeCount() != computes {
		t.Fatalf("repeated scalar queries recomputed: %d -> %d", computes, graph.ComputeCount())
	}

	// The public snapshot accessor must still copy: mutating it cannot poison
	// the scalar path.
	dom := graph.GetOrCompute(AnalysisDominators, domain)
	if graph.ComputeCount() != computes {
		t.Fatal("GetOrCompute missed the cached analysis")
	}
	if len(dom.IDom) == 0 {
		t.Fatal("empty idom snapshot")
	}
	dom.IDom[0] = 999
	again := graph.GetOrCompute(AnalysisDominators, domain)
	if again.IDom[0] == 999 {
		t.Fatal("caller mutation poisoned the cached snapshot")
	}
	if !graph.Dominates(domain, 0, 4) {
		t.Fatal("poisoning a copy changed scalar dominance")
	}
}

func TestT26ScalarPostDominanceQueriesShareCachedAnalysis(t *testing.T) {
	graph, domain := t26ScalarGraph()

	if !graph.PostDominates(domain, 3, 0) {
		t.Fatal("join must post-dominate the entry")
	}
	if graph.PostDominates(domain, 1, 0) {
		t.Fatal("one arm must not post-dominate the entry")
	}
	if !graph.PostDominates(domain, 4, 0) {
		t.Fatal("exit must post-dominate the entry")
	}
	if graph.PostDominates(domain, VirtualExitSentinel, 0) {
		t.Fatal("virtual exit exposed as a physical return")
	}
	if !graph.PostDominates(domain, 4, 4) {
		t.Fatal("real exit must post-dominate itself")
	}

	computes := graph.ComputeCount()
	for i := 0; i < 200; i++ {
		if !graph.PostDominates(domain, 3, 0) {
			t.Fatal("post-dominance changed on an unchanged epoch")
		}
	}
	if graph.ComputeCount() != computes {
		t.Fatalf("repeated scalar post queries recomputed: %d -> %d", computes, graph.ComputeCount())
	}

	post := graph.GetOrCompute(AnalysisPostDominators, domain)
	if graph.ComputeCount() != computes {
		t.Fatal("GetOrCompute post query missed the cached analysis")
	}
	for a := 0; a < 5; a++ {
		for b := 0; b < 5; b++ {
			if got, want := graph.PostDominates(domain, a, b), post.PostDominates(a, b); got != want {
				t.Fatalf("PostDominates(%d,%d)=%v, want %v", a, b, got, want)
			}
		}
	}

	// A topology change must invalidate both derived lists.
	graph.DeleteEdge(1, 3, false)
	if graph.PostDominates(domain, 3, 0) {
		t.Fatal("join still post-dominates the entry after one arm became exit-less")
	}
	if !graph.Dominates(domain, 2, 3) || !graph.Dominates(domain, 0, 4) {
		t.Fatal("dominance not recomputed after edge deletion")
	}
	if graph.ComputeCount() <= computes {
		t.Fatal("edge deletion reused the cached analysis")
	}
}
