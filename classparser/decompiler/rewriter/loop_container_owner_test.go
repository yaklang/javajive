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

func TestTightLoopIgnoresPredecessorNumbering(t *testing.T) {
	for _, preheaderID := range []int{1, 99, 1000} {
		root := core.NewNode(&statements.ConditionStatement{})
		outer := core.NewNode(statements.NewDoWhileStatement(nil, nil))
		preheader := core.NewNode(statements.NewDoWhileStatement(nil, nil))
		loop := core.NewNode(statements.NewDoWhileStatement(nil, nil))
		header := core.NewNode(&statements.ConditionStatement{})
		body := core.NewNode(&statements.ConditionStatement{})
		tail := core.NewNode(&statements.ConditionStatement{})
		preheader.Id, loop.Id, header.Id, body.Id = preheaderID, 101, 10, 11
		root.AddNext(outer)
		outer.AddNext(preheader)
		preheader.AddNext(loop)
		loop.AddNext(header)
		header.AddNext(body)
		header.AddNext(tail)
		body.AddNext(loop)
		body.AddNext(outer) // a labeled continue is not an inner back edge
		tail.AddNext(outer)
		dom := GenerateDominatorTree(root)
		set := circleElementSet(loop, header, dom, true)
		for _, n := range []*core.Node{root, outer, preheader, tail} {
			if set.Has(n) {
				t.Fatalf("preheader id %d: external node %d entered the tight loop", preheaderID, n.Id)
			}
		}
		if !set.Has(body) || !set.Has(header) || searchCircleEndNode(loop, header, dom, true) != tail {
			t.Fatalf("preheader id %d: lost the loop body or its normal exit", preheaderID)
		}
	}
}
