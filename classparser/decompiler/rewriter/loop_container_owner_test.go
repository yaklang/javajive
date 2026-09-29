package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/utils"
)

func TestLoopContainerOwnerExcludesDominatedContinuation(t *testing.T) {
	nodes := make([]*core.Node, 10)
	for i := range nodes {
		nodes[i] = core.NewNode(&statements.ConditionStatement{})
		nodes[i].Id = i
	}
	for _, e := range [][2]int{
		{0, 1}, {1, 2}, {2, 3}, {2, 9}, {3, 4}, {4, 5},
		{5, 6}, {5, 7}, {6, 4}, {7, 9}, {7, 8}, {8, 1},
	} {
		nodes[e[0]].AddNext(nodes[e[1]])
	}
	manager := NewRootStatementManager(nodes[0])
	manager.DominatorMap = GenerateDominatorTree(nodes[0])
	manager.LoopRegionReducible = true
	if !utils.IsDominate(manager.DominatorMap, nodes[4], nodes[7]) {
		t.Fatal("fixture must distinguish dominance from loop ownership")
	}
	for _, tc := range []struct {
		loop, node int
		want       bool
	}{
		{4, 4, true}, {4, 5, true}, {4, 6, true},
		{4, 7, false}, {4, 8, false}, {4, 9, false},
		{1, 4, true}, {1, 7, true}, {1, 8, true}, {1, 9, false},
	} {
		if got := loopOwnsRewriteNode(manager, nodes[tc.loop], nodes[tc.node]); got != tc.want {
			t.Errorf("loop %d owns %d = %t, want %t", tc.loop, tc.node, got, tc.want)
		}
	}
}
