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

// A topology change must keep base bodies for loops whose dominator subtree and
// immediate adjacency were untouched, and drop those that contain the change.
func TestLoopBaseCacheInvalidatesOnlyAffectedLoops(t *testing.T) {
	root := core.NewNode(&statements.MiddleStatement{Flag: "root"})
	unrelatedA := core.NewNode(&statements.MiddleStatement{Flag: "a"})
	unrelatedB := core.NewNode(&statements.MiddleStatement{Flag: "b"})
	header := core.NewNode(statements.NewDoWhileStatement(nil, nil))
	bodyA := core.NewNode(&statements.MiddleStatement{Flag: "bodyA"})
	bodyB := core.NewNode(&statements.MiddleStatement{Flag: "bodyB"})
	latch := core.NewNode(&statements.MiddleStatement{Flag: "latch"})
	exit := core.NewNode(&statements.MiddleStatement{Flag: "exit"})
	root.Id, unrelatedA.Id, unrelatedB.Id, header.Id = 0, 1, 2, 3
	bodyA.Id, bodyB.Id, latch.Id, exit.Id = 4, 5, 6, 7
	root.AddNext(unrelatedA)
	unrelatedA.AddNext(unrelatedB)
	root.AddNext(header)
	header.AddNext(bodyA)
	header.AddNext(bodyB)
	bodyA.AddNext(latch)
	latch.AddNext(header)
	bodyB.AddNext(exit)

	m := NewRootStatementManager(root)
	m.LoopRegionReducible = true
	m.refreshDominatorsShared()
	m.loopBases = map[loopBodyKey]loopBaseEntry{}
	m.loopBody(header, bodyA, true)
	if len(m.loopBases) != 1 {
		t.Fatalf("initial base cache size=%d", len(m.loopBases))
	}

	// A duplicate edge between two nodes outside the loop subtree and not
	// adjacent to it must not invalidate the body.
	unrelatedA.Next = append(unrelatedA.Next, unrelatedB)
	m.refreshDominatorsShared()
	if len(m.loopBases) != 1 {
		t.Fatal("unrelated edge change invalidated the loop body")
	}

	// An edge from a dominated body node to another node belongs to the loop's
	// subtree and must invalidate it.
	bodyA.AddNext(exit)
	m.refreshDominatorsShared()
	if len(m.loopBases) != 0 {
		t.Fatal("loop-body edge change kept the cached body")
	}

	// A new external entry directly into the loop body is adjacent to the
	// dominated subtree and must also invalidate it.
	m.loopBody(header, bodyA, true)
	if len(m.loopBases) != 1 {
		t.Fatal("base body was not repopulated")
	}
	root.AddNext(bodyA)
	m.refreshDominatorsShared()
	if len(m.loopBases) != 0 {
		t.Fatal("external entry into the loop kept the cached body")
	}
}
