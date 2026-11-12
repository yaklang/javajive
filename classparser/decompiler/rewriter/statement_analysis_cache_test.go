package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/utils"
)

// cacheTestGraph builds a deterministic, branchy CFG with a back edge and a
// real method exit. All nodes are reachable from node 0.
func cacheTestGraph(n int) []*core.Node {
	nodes := make([]*core.Node, n)
	for i := range nodes {
		nodes[i] = core.NewNode(&statements.MiddleStatement{Flag: "start"})
		nodes[i].Id = i
	}
	for i := 0; i+1 < n; i++ {
		nodes[i].AddNext(nodes[i+1])
	}
	for i := 1; i+2 < n; i += 3 {
		nodes[i].AddNext(nodes[i+2])
	}
	if n > 3 {
		nodes[n-2].AddNext(nodes[n/2])
	}
	return nodes
}

func cacheTestManager(n int) (*RewriteManager, []*core.Node) {
	nodes := cacheTestGraph(n)
	m := NewRootStatementManager(nodes[0])
	m.SetId(n + 100)
	return m, nodes
}

func cacheTestEqualDominatorTrees(left, right map[*core.Node][]*core.Node) bool {
	if len(left) != len(right) {
		return false
	}
	for parent, children := range left {
		other, ok := right[parent]
		if !ok || len(children) != len(other) {
			return false
		}
		for i := range children {
			if children[i] != other[i] {
				return false
			}
		}
	}
	return true
}

func cacheTestEqualPostDominators(left, right map[*core.Node]*core.Node) bool {
	if len(left) != len(right) {
		return false
	}
	for node, parent := range left {
		if right[node] != parent {
			return false
		}
	}
	return true
}

func cacheTestAssertFreshDominators(t *testing.T, m *RewriteManager) {
	t.Helper()
	m.refreshDominators()
	if want := GenerateDominatorTree(m.RootNode); !cacheTestEqualDominatorTrees(m.DominatorMap, want) {
		t.Fatalf("cached dominator tree differs from fresh GenerateDominatorTree\ncached=%v\nfresh=%v", m.DominatorMap, want)
	}
}

func cacheTestAssertFreshPostDominators(t *testing.T, m *RewriteManager) {
	t.Helper()
	got := m.postDominatorMap()
	if want := GeneratePostDominatorMap(m.RootNode); !cacheTestEqualPostDominators(got, want) {
		t.Fatalf("cached post-dominators differ from fresh GeneratePostDominatorMap\ncached=%v\nfresh=%v", got, want)
	}
}

func TestStatementDominanceCacheRepeatedQueriesComputeOnce(t *testing.T) {
	m, _ := cacheTestManager(14)
	root := m.RootNode

	m.refreshDominators()
	if m.dominance == nil {
		t.Fatal("dominance state not initialized")
	}
	if m.dominance.domComputes != 1 {
		t.Fatalf("first refresh computed %d times, want 1", m.dominance.domComputes)
	}
	first := cloneDominatorTree(m.DominatorMap)
	if !cacheTestEqualDominatorTrees(first, GenerateDominatorTree(root)) {
		t.Fatal("first cached tree differs from fresh analysis")
	}

	firstPost := m.postDominatorMap()
	if m.dominance.postComputes != 1 {
		t.Fatalf("first post query computed %d times, want 1", m.dominance.postComputes)
	}

	for i := 0; i < 100; i++ {
		m.refreshDominators()
		if !cacheTestEqualDominatorTrees(m.DominatorMap, first) {
			t.Fatalf("refresh %d changed an unchanged topology", i)
		}
		if post := m.postDominatorMap(); !cacheTestEqualPostDominators(post, firstPost) {
			t.Fatalf("post refresh %d changed an unchanged topology", i)
		}
	}
	if m.dominance.domComputes != 1 || m.dominance.postComputes != 1 {
		t.Fatalf("repeated unchanged queries recomputed: dom=%d post=%d", m.dominance.domComputes, m.dominance.postComputes)
	}
	if m.dominance.graph == nil || m.dominance.graph.ComputeCount() != 1 {
		t.Fatalf("GraphAnalysisCache constructions = %v, want 1", m.dominance.graph)
	}

	// Non-topology fields must not invalidate the memoized analyses either.
	firstDom := cloneDominatorTree(m.DominatorMap)
	m.DominatorMap = nil
	m.refreshDominators()
	if m.dominance.domComputes != 1 || !cacheTestEqualDominatorTrees(m.DominatorMap, firstDom) {
		t.Fatalf("nil field assignment should not invalidate: computes=%d", m.dominance.domComputes)
	}
}

func TestStatementDominanceCacheTracksTopologyMutationPaths(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(m *RewriteManager, nodes []*core.Node)
	}{
		{
			name: "AddNext",
			mutate: func(_ *RewriteManager, nodes []*core.Node) {
				nodes[5].AddNext(nodes[1])
			},
		},
		{
			name: "RemoveNext",
			mutate: func(_ *RewriteManager, nodes []*core.Node) {
				nodes[4].RemoveNext(nodes[5])
			},
		},
		{
			name: "ReplaceNext",
			mutate: func(_ *RewriteManager, nodes []*core.Node) {
				nodes[2].ReplaceNext(nodes[3], nodes[8])
			},
		},
		{
			name: "ReplaceNextSliceKeepOrder",
			mutate: func(_ *RewriteManager, nodes []*core.Node) {
				nodes[2].ReplaceNextSliceKeepOrder(nodes[3], []*core.Node{nodes[8], nodes[9]})
			},
		},
		{
			name: "InsertNextAt",
			mutate: func(_ *RewriteManager, nodes []*core.Node) {
				nodes[1].InsertNextAt(1, nodes[10])
			},
		},
		{
			name: "DuplicateNext",
			mutate: func(_ *RewriteManager, nodes []*core.Node) {
				nodes[1].DuplicateNext(0)
			},
		},
		{
			name: "DirectNextAssignment",
			mutate: func(_ *RewriteManager, nodes []*core.Node) {
				nodes[2].Next = []*core.Node{nodes[7]}
			},
		},
		{
			name: "DirectSliceTruncation",
			mutate: func(_ *RewriteManager, nodes []*core.Node) {
				nodes[2].Next = append(nodes[2].Next[:0], nodes[2].Next[1:]...)
			},
		},
		{
			name: "SuccessorOrderSwap",
			mutate: func(_ *RewriteManager, nodes []*core.Node) {
				nodes[12].Next[0], nodes[12].Next[1] = nodes[12].Next[1], nodes[12].Next[0]
			},
		},
		{
			name: "RemoveAllNext",
			mutate: func(_ *RewriteManager, nodes []*core.Node) {
				nodes[6].RemoveAllNext()
			},
		},
		{
			name: "NodeReplace",
			mutate: func(m *RewriteManager, nodes []*core.Node) {
				replacement := m.NewNode(&statements.MiddleStatement{Flag: "start"})
				nodes[7].Replace(replacement)
			},
		},
		{
			name: "RootReplacement",
			mutate: func(m *RewriteManager, nodes []*core.Node) {
				m.RootNode = nodes[3]
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, nodes := cacheTestManager(14)
			m.refreshDominators()
			_ = m.postDominatorMap()
			domBefore := m.dominance.domComputes
			postBefore := m.dominance.postComputes

			tc.mutate(m, nodes)

			cacheTestAssertFreshDominators(t, m)
			if got := m.dominance.domComputes; got != domBefore+1 {
				t.Fatalf("dominator computes after %s = %d, want %d", tc.name, got, domBefore+1)
			}
			cacheTestAssertFreshPostDominators(t, m)
			if got := m.dominance.postComputes; got != postBefore+1 {
				t.Fatalf("post-dominator computes after %s = %d, want %d", tc.name, got, postBefore+1)
			}
		})
	}
}

func TestStatementDominanceCacheIgnoresNonTopologyFields(t *testing.T) {
	m, nodes := cacheTestManager(14)
	m.refreshDominators()
	_ = m.postDominatorMap()
	domBefore := m.dominance.domComputes
	postBefore := m.dominance.postComputes
	first := cloneDominatorTree(m.DominatorMap)
	firstPost := clonePostDominatorMap(m.postDominatorMap())

	nodes[3].Id = 999
	nodes[3].Statement = &statements.MiddleStatement{Flag: "changed"}
	nodes[3].Source = append(nodes[3].Source, nodes[4])
	nodes[3].IsCircle = true
	nodes[3].IsIf = true
	nodes[3].EncodedJumps = map[*core.Node]bool{nodes[4]: true}
	_ = core.NewNode(&statements.MiddleStatement{Flag: "unreachable"})

	m.refreshDominators()
	if m.dominance.domComputes != domBefore {
		t.Fatalf("non-topology mutation recomputed dominators: %d -> %d", domBefore, m.dominance.domComputes)
	}
	if !cacheTestEqualDominatorTrees(m.DominatorMap, first) {
		t.Fatal("non-topology mutation changed the cached dominator tree")
	}
	if post := m.postDominatorMap(); m.dominance.postComputes != postBefore || !cacheTestEqualPostDominators(post, firstPost) {
		t.Fatal("non-topology mutation invalidated post-dominators")
	}
	cacheTestAssertFreshDominators(t, m)
	cacheTestAssertFreshPostDominators(t, m)
}

func TestStatementDominanceCacheReturnsDefensiveCopies(t *testing.T) {
	m, nodes := cacheTestManager(14)
	root := m.RootNode
	m.refreshDominators()

	// Poison the caller-visible tree exactly as a mutating caller could.
	for node := range m.DominatorMap {
		delete(m.DominatorMap, node)
	}
	m.DominatorMap[nodes[0]] = []*core.Node{nodes[1], nil}
	if children := m.DominatorMap[nodes[0]]; len(children) > 0 {
		children[0] = nil
	}

	cacheTestAssertFreshDominators(t, m)
	if m.dominance.domComputes != 1 {
		t.Fatalf("poisoned public map forced a recompute: %d", m.dominance.domComputes)
	}
	if !cacheTestEqualDominatorTrees(m.DominatorMap, GenerateDominatorTree(root)) {
		t.Fatal("internal dominator tree leaked through a mutable public map")
	}

	post := m.postDominatorMap()
	for node := range post {
		delete(post, node)
	}
	post[nodes[0]] = nodes[1]
	cacheTestAssertFreshPostDominators(t, m)
	if m.dominance.postComputes != 1 {
		t.Fatalf("poisoned post-dominator map forced a recompute: %d", m.dominance.postComputes)
	}
}

func TestStatementDominanceCacheScalarQueries(t *testing.T) {
	m, nodes := cacheTestManager(14)
	root := m.RootNode
	m.refreshDominators()
	tree := GenerateDominatorTree(root)
	for _, left := range nodes {
		for _, right := range nodes {
			got := m.Dominates(left, right)
			want := utils.IsDominate(tree, left, right)
			if got != want {
				t.Fatalf("Dominates(%d,%d)=%v, want %v", left.Id, right.Id, got, want)
			}
		}
	}

	postTree := GeneratePostDominatorMap(root)
	for _, left := range nodes {
		for _, right := range nodes {
			want := left == right
			if !want {
				for current := right; current != nil; {
					parent := postTree[current]
					if parent == nil {
						break
					}
					if parent == left {
						want = true
						break
					}
					current = parent
				}
			}
			if got := m.PostDominates(left, right); got != want {
				t.Fatalf("PostDominates(%d,%d)=%v, want %v", left.Id, right.Id, got, want)
			}
		}
	}
}

func TestStatementDominanceCacheScanCoreInfoReusesUnchangedAnalysis(t *testing.T) {
	m, _ := cacheTestManager(12)
	if err := m.ScanCoreInfo(); err != nil {
		t.Fatal(err)
	}
	domComputes := m.dominance.domComputes
	postComputes := m.dominance.postComputes

	// ScanCoreInfo updates classification flags (IsIf/IsMerge/loop callbacks)
	// but not Next edges, so a second scan must reuse the same analyses.
	if err := m.ScanCoreInfo(); err != nil {
		t.Fatal(err)
	}
	if m.dominance.domComputes != domComputes {
		t.Fatalf("second ScanCoreInfo recomputed dominators: %d -> %d", domComputes, m.dominance.domComputes)
	}
	if m.dominance.postComputes != postComputes {
		t.Fatalf("second ScanCoreInfo recomputed post-dominators: %d -> %d", postComputes, m.dominance.postComputes)
	}
	cacheTestAssertFreshDominators(t, m)
	cacheTestAssertFreshPostDominators(t, m)
}

func TestStatementDominanceCacheNilRoot(t *testing.T) {
	m := &RewriteManager{}
	m.refreshDominators()
	if m.DominatorMap == nil || len(m.DominatorMap) != 0 {
		t.Fatalf("nil root produced %v, want an empty dominator map", m.DominatorMap)
	}
	if !cacheTestEqualDominatorTrees(m.DominatorMap, GenerateDominatorTree(nil)) {
		t.Fatal("nil root differs from GenerateDominatorTree(nil)")
	}
	if post := m.postDominatorMap(); post == nil || len(post) != 0 {
		t.Fatalf("nil root produced post-dominators %v, want empty", post)
	}
	if m.Dominates(nil, nil) || m.PostDominates(nil, nil) {
		t.Fatal("nil nodes must not dominate or post-dominate")
	}
	if m.dominance.domComputes != 1 || m.dominance.postComputes != 1 {
		t.Fatalf("nil root computes dom=%d post=%d", m.dominance.domComputes, m.dominance.postComputes)
	}
	m.refreshDominators()
	if m.dominance.domComputes != 1 {
		t.Fatal("repeated nil-root refresh recomputed")
	}
}

func TestStatementDominanceCachePostDominatorExitLessComponent(t *testing.T) {
	root := core.NewNode(&statements.MiddleStatement{Flag: "root"})
	exit := core.NewNode(&statements.ReturnStatement{})
	loop := core.NewNode(&statements.MiddleStatement{Flag: "loop"})
	root.Id, exit.Id, loop.Id = 0, 1, 2
	root.AddNext(exit)
	root.AddNext(loop)
	loop.AddNext(loop)

	m := NewRootStatementManager(root)
	m.refreshDominators()
	cacheTestAssertFreshDominators(t, m)
	cacheTestAssertFreshPostDominators(t, m)

	if !m.PostDominates(exit, root) {
		t.Fatal("reachable exit should post-dominate the branch")
	}
	if m.PostDominates(loop, root) {
		t.Fatal("exit-less SCC must not post-dominate a branching node")
	}
	if m.PostDominates(loop, loop) {
		t.Fatal("exit-less SCC has no post-dominator, including itself")
	}
	if !m.PostDominates(exit, exit) {
		t.Fatal("a real exit post-dominates itself")
	}
	if m.dominance.postComputes != 1 {
		t.Fatalf("post-dominators recomputed on repeated scalar queries: %d", m.dominance.postComputes)
	}
}
