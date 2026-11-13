package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
)

var (
	benchmarkDominatorSink   map[*core.Node][]*core.Node
	benchmarkPostDomSink     map[*core.Node]*core.Node
	benchmarkDominanceAnswer bool
)

func cacheBenchGraph(n int) []*core.Node {
	nodes := make([]*core.Node, n)
	for i := range nodes {
		nodes[i] = core.NewNode(&statements.MiddleStatement{Flag: "bench"})
		nodes[i].Id = i
	}
	for i := 0; i+1 < n; i++ {
		nodes[i].AddNext(nodes[i+1])
	}
	for i := 1; i+2 < n; i += 4 {
		nodes[i].AddNext(nodes[i+2])
	}
	for i := 0; i+7 < n; i += 7 {
		nodes[i].AddNext(nodes[(i*31+17)%n])
	}
	if n > 3 {
		nodes[n-2].AddNext(nodes[n/3])
	}
	return nodes
}

func BenchmarkGenerateDominatorTreeFresh(b *testing.B) {
	nodes := cacheBenchGraph(1024)
	root := nodes[0]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkDominatorSink = GenerateDominatorTree(root)
	}
}

func BenchmarkGenerateDominatorTreeAfterMutation(b *testing.B) {
	nodes := cacheBenchGraph(1024)
	root, last := nodes[0], nodes[len(nodes)-1]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			root.AddNext(last)
		} else {
			root.RemoveNext(last)
		}
		benchmarkDominatorSink = GenerateDominatorTree(root)
	}
}

func BenchmarkStatementDominanceCacheRefreshUnchanged(b *testing.B) {
	nodes := cacheBenchGraph(1024)
	m := NewRootStatementManager(nodes[0])
	m.refreshDominators()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.refreshDominators()
		benchmarkDominatorSink = m.DominatorMap
	}
}

func BenchmarkStatementDominanceCacheRefreshAfterMutation(b *testing.B) {
	nodes := cacheBenchGraph(1024)
	m := NewRootStatementManager(nodes[0])
	m.refreshDominators()
	root, last := nodes[0], nodes[len(nodes)-1]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			root.AddNext(last)
		} else {
			root.RemoveNext(last)
		}
		m.refreshDominators()
		benchmarkDominatorSink = m.DominatorMap
	}
}

func BenchmarkGeneratePostDominatorMapFresh(b *testing.B) {
	nodes := cacheBenchGraph(1024)
	root := nodes[0]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkPostDomSink = GeneratePostDominatorMap(root)
	}
}

func BenchmarkStatementDominanceCachePostDominatorsUnchanged(b *testing.B) {
	nodes := cacheBenchGraph(1024)
	m := NewRootStatementManager(nodes[0])
	_ = m.postDominatorMap()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkPostDomSink = m.postDominatorMap()
	}
}

func BenchmarkStatementDominanceCacheDominatesUnchanged(b *testing.B) {
	nodes := cacheBenchGraph(1024)
	m := NewRootStatementManager(nodes[0])
	root, last := nodes[0], nodes[len(nodes)-1]
	_ = m.Dominates(root, last)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkDominanceAnswer = m.Dominates(root, last)
	}
}

func BenchmarkStatementDominanceCachePostDominatesUnchanged(b *testing.B) {
	nodes := cacheBenchGraph(1024)
	m := NewRootStatementManager(nodes[0])
	root, last := nodes[0], nodes[len(nodes)-1]
	_ = m.PostDominates(last, root)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkDominanceAnswer = m.PostDominates(last, root)
	}
}
