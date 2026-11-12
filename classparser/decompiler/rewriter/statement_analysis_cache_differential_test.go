package rewriter

import (
	"math/rand"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

// cacheDifferentialGraph builds a random CFG whose nodes are all reachable
// from node 0. Random extra edges introduce branches, joins, back edges and
// exit-less cycles; no assumption is made about reducibility.
func cacheDifferentialGraph(r *rand.Rand, n int) (*core.Node, []*core.Node) {
	nodes := make([]*core.Node, n)
	for i := range nodes {
		nodes[i] = core.NewNode(&statements.MiddleStatement{Flag: "n"})
		nodes[i].Id = i
	}
	for i := 1; i < n; i++ {
		nodes[r.Intn(i)].AddNext(nodes[i])
	}
	extra := r.Intn(2*n) + 1
	for i := 0; i < extra; i++ {
		nodes[r.Intn(n)].AddNext(nodes[r.Intn(n)])
	}
	return nodes[0], nodes
}

// cacheDifferentialMutate exercises every mutation shape the rewriter uses:
// the Node methods plus the direct Next assignments/slicing that cannot be
// intercepted.
func cacheDifferentialMutate(r *rand.Rand, m *RewriteManager, nodes []*core.Node) {
	if len(nodes) == 0 {
		return
	}
	node := nodes[r.Intn(len(nodes))]
	switch r.Intn(9) {
	case 0:
		node.AddNext(nodes[r.Intn(len(nodes))])
	case 1:
		if len(node.Next) > 0 {
			node.RemoveNext(node.Next[r.Intn(len(node.Next))])
		}
	case 2:
		if len(node.Next) > 0 {
			node.ReplaceNext(node.Next[0], nodes[r.Intn(len(nodes))])
		}
	case 3:
		if len(node.Next) > 0 {
			count := 1 + r.Intn(2)
			news := make([]*core.Node, 0, count)
			for i := 0; i < count; i++ {
				news = append(news, nodes[r.Intn(len(nodes))])
			}
			node.ReplaceNextSliceKeepOrder(node.Next[0], news)
		}
	case 4:
		node.InsertNextAt(r.Intn(len(node.Next)+1), nodes[r.Intn(len(nodes))])
	case 5:
		if len(node.Next) > 0 {
			node.DuplicateNext(r.Intn(len(node.Next)))
		}
	case 6:
		node.Next = []*core.Node{nodes[r.Intn(len(nodes))]}
	case 7:
		if len(node.Next) > 0 {
			node.Next = append(node.Next[:0:0], node.Next[1:]...)
		}
	case 8:
		if len(node.Next) > 0 {
			node.Next[r.Intn(len(node.Next))] = nodes[r.Intn(len(nodes))]
		}
	}
}

func TestStatementDominanceCacheDifferentialRandomized(t *testing.T) {
	r := rand.New(rand.NewSource(0x5eed))
	for iteration := 0; iteration < 1500; iteration++ {
		nodeCount := 1 + r.Intn(14)
		root, nodes := cacheDifferentialGraph(r, nodeCount)
		m := NewRootStatementManager(root)
		m.SetId(nodeCount + 100)

		m.refreshDominators()
		if want := GenerateDominatorTree(root); !cacheTestEqualDominatorTrees(m.DominatorMap, want) {
			t.Fatalf("iteration %d initial dominators differ\ncached=%v\nfresh=%v", iteration, m.DominatorMap, want)
		}
		if want := GeneratePostDominatorMap(root); !cacheTestEqualPostDominators(m.postDominatorMap(), want) {
			t.Fatalf("iteration %d initial post-dominators differ", iteration)
		}

		for step := 0; step < 6; step++ {
			cacheDifferentialMutate(r, m, nodes)
			m.refreshDominators()
			if want := GenerateDominatorTree(m.RootNode); !cacheTestEqualDominatorTrees(m.DominatorMap, want) {
				t.Fatalf("iteration %d step %d dominators differ after mutation\ncached=%v\nfresh=%v", iteration, step, m.DominatorMap, want)
			}
			if want := GeneratePostDominatorMap(m.RootNode); !cacheTestEqualPostDominators(m.postDominatorMap(), want) {
				t.Fatalf("iteration %d step %d post-dominators differ after mutation", iteration, step)
			}
		}
	}
}

func TestStatementDominanceCacheOnlyAnalyzesReachableNodes(t *testing.T) {
	root := core.NewNode(&statements.MiddleStatement{Flag: "root"})
	reachable := core.NewNode(&statements.ReturnStatement{})
	isolated := core.NewNode(&statements.MiddleStatement{Flag: "isolated"})
	isolatedTarget := core.NewNode(&statements.ReturnStatement{})
	root.Id, reachable.Id, isolated.Id, isolatedTarget.Id = 0, 1, 2, 3
	root.AddNext(reachable)
	isolated.AddNext(isolatedTarget)

	m := NewRootStatementManager(root)
	m.refreshDominators()
	if want := GenerateDominatorTree(root); !cacheTestEqualDominatorTrees(m.DominatorMap, want) {
		t.Fatal("initial reachable-only tree differs")
	}
	domComputes := m.dominance.domComputes

	// Mutating an unreachable component cannot change the result and must not
	// force a recompute.
	isolatedTarget.AddNext(isolated)
	isolated.RemoveNext(isolatedTarget)
	m.refreshDominators()
	if m.dominance.domComputes != domComputes {
		t.Fatal("unreachable component mutation forced a recompute")
	}
	if !cacheTestEqualDominatorTrees(m.DominatorMap, GenerateDominatorTree(root)) {
		t.Fatal("unreachable component mutation changed the cached tree")
	}

	// Making it reachable must invalidate.
	root.AddNext(isolated)
	m.refreshDominators()
	if m.dominance.domComputes != domComputes+1 {
		t.Fatalf("reachable component addition computes=%d, want %d", m.dominance.domComputes, domComputes+1)
	}
	if want := GenerateDominatorTree(root); !cacheTestEqualDominatorTrees(m.DominatorMap, want) {
		t.Fatal("newly reachable nodes missing from cached tree")
	}
}
