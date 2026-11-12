package rewriter

import (
	"math/rand"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	utils2 "github.com/yaklang/javajive/internal/utils"
)

func cacheTestEqualNodeSets(a, b *utils2.Set[*core.Node], nodes []*core.Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Len() != b.Len() {
		return false
	}
	for _, n := range nodes {
		if a.Has(n) != b.Has(n) {
			return false
		}
	}
	return true
}

// The loop-analysis cache must be observationally identical to recomputing the
// body and exit for every query. This exercises bodies, exits, missing exits,
// excluded pre-headers and the legacy over-approximated body on random CFGs.
func TestLoopAnalysisCacheMatchesFreshQueries(t *testing.T) {
	r := rand.New(rand.NewSource(0x10ca))
	for iteration := 0; iteration < 500; iteration++ {
		nodeCount := 2 + r.Intn(18)
		root, nodes := cacheDifferentialGraph(r, nodeCount)
		m := NewRootStatementManager(root)
		m.refreshDominators()
		m.LoopRegionReducible = true
		if !m.loopCacheSafe() {
			t.Fatalf("iteration %d: cache unexpectedly unsafe for an unprotected graph", iteration)
		}
		m.loopCache = newLoopAnalysisCache()

		for _, loop := range nodes {
			if len(loop.Next) == 0 {
				continue
			}
			start := loop.Next[0]
			for _, exclude := range []bool{true, false} {
				want := circleElementSet(loop, start, m.DominatorMap, exclude)
				got := m.loopBody(loop, start, exclude)
				if !cacheTestEqualNodeSets(got, want, nodes) {
					t.Fatalf("iteration %d loop %d exclude=%t: cached body differs", iteration, loop.Id, exclude)
				}
				if again := m.loopBody(loop, start, exclude); again != got {
					t.Fatalf("iteration %d loop %d exclude=%t: cache did not reuse the body", iteration, loop.Id, exclude)
				}
			}
			for _, reducible := range []bool{true, false} {
				want := searchCircleEndNode(loop, start, m.DominatorMap, reducible)
				got := m.searchLoopEnd(loop, start, reducible)
				if got != want {
					t.Fatalf("iteration %d loop %d reducible=%t: cached exit=%v want %v", iteration, loop.Id, reducible, got, want)
				}
				if again := m.searchLoopEnd(loop, start, reducible); again != got {
					t.Fatalf("iteration %d loop %d reducible=%t: cache did not reuse the exit", iteration, loop.Id, reducible)
				}
			}
		}
		m.loopCache = nil
	}
}

func TestLoopAnalysisCacheDisabledForProtectedRegions(t *testing.T) {
	root := core.NewNode(&statements.MiddleStatement{Flag: "root"})
	body := core.NewNode(&statements.MiddleStatement{Flag: "body"})
	handler := core.NewNode(&statements.MiddleStatement{Flag: "handler"})
	root.Id, body.Id, handler.Id = 0, 1, 2
	root.AddNext(body)
	body.AddNext(root)
	root.AddNext(handler)

	m := NewRootStatementManager(root)
	m.refreshDominators()
	if !m.loopCacheSafe() {
		t.Fatal("unprotected graph must allow the loop cache")
	}
	handler.HasProtectedRange = true
	if m.loopCacheSafe() {
		t.Fatal("protected region must disable the loop cache")
	}
	m.TryNodes = []*core.Node{handler}
	if m.loopCacheSafe() {
		t.Fatal("try node must disable the loop cache")
	}
}
