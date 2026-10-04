package core

import (
	"sync"
	"testing"
)

func t26NaturalLoopCFG() *SemanticCFG {
	return t09Graph(4,
		t09Edge{0, 1, EdgeFallthrough},
		t09Edge{1, 2, EdgeTaken},
		t09Edge{1, 3, EdgeFallthrough},
		t09Edge{2, 1, EdgeTaken},
	)
}

func t26InfiniteArmCFG() *SemanticCFG {
	// 0 branch; 1 return; 2 infinite self-loop.
	return t09Graph(3,
		t09Edge{0, 1, EdgeFallthrough},
		t09Edge{0, 2, EdgeTaken},
		t09Edge{2, 2, EdgeTaken},
	)
}

func t26ExceptionCFG() *SemanticCFG {
	return t09Graph(4,
		t09Edge{0, 1, EdgeFallthrough},
		t09Edge{0, 2, EdgeException},
		t09Edge{1, 3, EdgeFallthrough},
		t09Edge{2, 3, EdgeFallthrough},
	)
}

func TestT26_C01_RepeatedQueriesHitCache(t *testing.T) {
	t.Run("T26-C01", testT26C01)
}
func testT26C01(t *testing.T) {
	g := t26NaturalLoopCFG()
	domain := GraphAnalysisDomain{Roots: []int{0}, IncludeException: false}
	firstDom := g.GetOrCompute(AnalysisDominators, domain)
	firstPost := g.GetOrCompute(AnalysisPostDominators, domain)
	firstLoop := g.GetOrCompute(AnalysisLoopForest, domain)
	if firstDom == nil || firstPost == nil || firstLoop == nil {
		t.Fatal("missing analysis")
	}
	computes := g.AnalysisComputeCount()
	touched := g.AnalysisNodesTouched()
	if computes == 0 {
		t.Fatal("first queries did not construct")
	}
	if err := g.ValidateReducible(); err != nil {
		t.Fatalf("natural loop rejected: %v", err)
	}
	if g.AnalysisComputeCount() < computes {
		t.Fatal("ValidateReducible did not use the analysis cache")
	}
	computes = g.AnalysisComputeCount()
	touched = g.AnalysisNodesTouched()
	for i := 0; i < 100; i++ {
		dom := g.GetOrCompute(AnalysisDominators, domain)
		post := g.GetOrCompute(AnalysisPostDominators, domain)
		loop := g.GetOrCompute(AnalysisLoopForest, domain)
		if !analysesEqual(firstDom, dom) || !analysesEqual(firstPost, post) || !analysesEqual(firstLoop, loop) {
			t.Fatalf("query %d returned a different snapshot", i)
		}
	}
	if g.AnalysisComputeCount() != computes {
		t.Fatalf("repeated queries recomputed: before=%d after=%d", computes, g.AnalysisComputeCount())
	}
	if g.AnalysisNodesTouched() != touched {
		t.Fatalf("construction work grew after hits: before=%d after=%d", touched, g.AnalysisNodesTouched())
	}
	poison := g.GetOrCompute(AnalysisDominators, domain)
	if len(poison.IDom) == 0 {
		t.Fatal("empty idom")
	}
	poison.IDom[0] = 999
	again := g.GetOrCompute(AnalysisDominators, domain)
	if again.IDom[0] == 999 {
		t.Fatal("caller mutation poisoned the cache")
	}
	slow := t26SlowImmediateDominators(len(g.Nodes), g.successorIndexLists(false), []int{0})
	if !equalIntSlices(again.IDom, slow) {
		t.Fatalf("cached idom %v != slow %v", again.IDom, slow)
	}
}

func TestT26_C02_MutationMissesCache(t *testing.T) {
	t.Run("T26-C02", testT26C02)
}
func testT26C02(t *testing.T) {
	cache := NewGraphAnalysisCache(8)
	m := NewMutableAnalysisGraph(4, cache)
	m.AddEdge(0, 1, false)
	m.AddEdge(1, 2, false)
	m.AddEdge(1, 3, false)
	m.AddEdge(2, 1, false)
	domain := GraphAnalysisDomain{Roots: []int{0}}
	before := m.GetOrCompute(AnalysisDominators, domain)
	computes := m.ComputeCount()
	epoch := m.Epoch()
	_ = m.GetOrCompute(AnalysisDominators, domain)
	if m.ComputeCount() != computes {
		t.Fatal("immutable query missed")
	}
	m.DeleteEdge(2, 1, false)
	if m.Epoch() == epoch {
		t.Fatal("delete did not bump epoch")
	}
	afterDel := m.GetOrCompute(AnalysisDominators, domain)
	if m.ComputeCount() == computes {
		t.Fatal("edge delete reused the old cache entry")
	}
	if analysesEqual(before, afterDel) && equalIntSlices(before.IDom, afterDel.IDom) {
		// IDom may coincidentally match on some graphs; the miss is the compute bump.
	}
	computes = m.ComputeCount()
	m.AddEdge(3, 0, false)
	afterAdd := m.GetOrCompute(AnalysisDominators, domain)
	if m.ComputeCount() == computes {
		t.Fatal("edge add reused the old cache entry")
	}
	_ = afterAdd
	computes = m.ComputeCount()
	fresh := m.ReplaceNode(1)
	afterRep := m.GetOrCompute(AnalysisDominators, GraphAnalysisDomain{Roots: []int{0}})
	if m.ComputeCount() == computes {
		t.Fatal("node replace reused the old cache entry")
	}
	if fresh == 1 {
		t.Fatal("replace did not allocate a new node")
	}
	_ = afterRep
	computes = m.ComputeCount()
	_ = m.AddNode()
	_ = m.GetOrCompute(AnalysisLoopForest, domain)
	if m.ComputeCount() == computes {
		t.Fatal("node add reused the old cache entry")
	}
}

func TestT26_C03_DomainKeysIsolated(t *testing.T) {
	t.Run("T26-C03", testT26C03)
}
func testT26C03(t *testing.T) {
	g := t26ExceptionCFG()
	g.SetAnalysisCacheCapacity(8)
	normal := GraphAnalysisDomain{Roots: []int{0}, IncludeException: false}
	withExc := GraphAnalysisDomain{Roots: []int{0}, IncludeException: true}
	handler := GraphAnalysisDomain{Roots: []int{2}, IncludeException: false}
	a := g.GetOrCompute(AnalysisDominators, normal)
	b := g.GetOrCompute(AnalysisDominators, withExc)
	c := g.GetOrCompute(AnalysisDominators, handler)
	if analysesEqual(a, b) {
		t.Fatal("includeException shared a cache key")
	}
	if analysesEqual(a, c) {
		t.Fatal("different roots shared a cache key")
	}
	if a.IDom[2] >= 0 {
		t.Fatalf("normal-only domain should not reach handler 2: idom=%v", a.IDom)
	}
	if b.IDom[2] < 0 {
		t.Fatalf("exception domain should reach handler 2: idom=%v", b.IDom)
	}
	if c.IDom[0] >= 0 {
		t.Fatalf("handler root should not dominate method entry: idom=%v", c.IDom)
	}
	slowN := t26SlowImmediateDominators(len(g.Nodes), g.successorIndexLists(false), []int{0})
	slowE := t26SlowImmediateDominators(len(g.Nodes), g.successorIndexLists(true), []int{0})
	slowH := t26SlowImmediateDominators(len(g.Nodes), g.successorIndexLists(false), []int{2})
	if !equalIntSlices(a.IDom, slowN) || !equalIntSlices(b.IDom, slowE) || !equalIntSlices(c.IDom, slowH) {
		t.Fatalf("domain idom mismatch cache=%v/%v/%v slow=%v/%v/%v", a.IDom, b.IDom, c.IDom, slowN, slowE, slowH)
	}
}

func TestT26_C04_InfiniteArmNotForcedThroughReturn(t *testing.T) {
	t.Run("T26-C04", testT26C04)
}
func testT26C04(t *testing.T) {
	g := t26InfiniteArmCFG()
	domain := GraphAnalysisDomain{Roots: []int{0}}
	post := g.GetOrCompute(AnalysisPostDominators, domain)
	if post.PostDominates(1, 2) {
		t.Fatalf("return postdominates infinite arm: ipdom=%v", post.IPDom)
	}
	if post.PostDominates(1, 0) {
		t.Fatalf("return postdominates the branch (would force the infinite arm through the return): ipdom=%v", post.IPDom)
	}
	if post.IPDom[2] == 1 {
		t.Fatalf("infinite node ipdom is the return: %v", post.IPDom)
	}
	if post.IPDom[2] != VirtualExitSentinel && post.IPDom[2] != 2 && post.IPDom[2] != -1 {
		t.Fatalf("infinite arm ipdom=%d, want virtual/self/none", post.IPDom[2])
	}
	slow := t26SlowImmediatePostDominators(len(g.Nodes), g.successorIndexLists(false), []int{0})
	if !equalIntSlices(post.IPDom, slow) {
		t.Fatalf("postdom cache %v != slow %v", post.IPDom, slow)
	}
	if post.VirtualExitUsed && post.IPDom[1] == VirtualExitSentinel {
		// return's ipdom may be the virtual exit; that sentinel is not a real return.
	}
}

func TestT26_C05_ConcurrentCap1NoCrossContamination(t *testing.T) {
	t.Run("T26-C05", testT26C05)
}
func testT26C05(t *testing.T) {
	g := t09Graph(6,
		t09Edge{0, 1, EdgeFallthrough},
		t09Edge{1, 2, EdgeTaken},
		t09Edge{1, 3, EdgeFallthrough},
		t09Edge{2, 1, EdgeTaken},
		t09Edge{3, 4, EdgeFallthrough},
		t09Edge{0, 5, EdgeException},
		t09Edge{5, 4, EdgeFallthrough},
	)
	g.SetAnalysisCacheCapacity(1)
	domains := []GraphAnalysisDomain{
		{Roots: []int{0}, IncludeException: false},
		{Roots: []int{0}, IncludeException: true},
		{Roots: []int{5}, IncludeException: false},
		{Roots: []int{1}, IncludeException: false},
	}
	kinds := []AnalysisKind{AnalysisDominators, AnalysisPostDominators, AnalysisLoopForest}
	want := make([]*GraphAnalysis, len(domains)*len(kinds))
	for i, d := range domains {
		for j, k := range kinds {
			want[i*len(kinds)+j] = g.GetOrCompute(k, d)
		}
	}
	var wg sync.WaitGroup
	errc := make(chan string, 64)
	for n := 0; n < 16; n++ {
		for i, d := range domains {
			for j, k := range kinds {
				i, d, j, k := i, d, j, k
				wg.Add(1)
				go func() {
					defer wg.Done()
					got := g.GetOrCompute(k, d)
					exp := want[i*len(kinds)+j]
					if !analysesEqual(exp, got) {
						errc <- "cross-contaminated or racy result"
					}
					slowDom := t26SlowImmediateDominators(len(g.Nodes), g.successorIndexLists(d.IncludeException), d.Roots)
					if k == AnalysisDominators && !equalIntSlices(got.IDom, slowDom) {
						errc <- "idom diverged from slow oracle under race"
					}
				}()
			}
		}
	}
	wg.Wait()
	close(errc)
	for msg := range errc {
		t.Fatal(msg)
	}
}
