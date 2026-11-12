package utils

import (
	"math/rand"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestDominateIndexMatchesRecursiveWalk(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for round := 0; round < 300; round++ {
		n := 1 + r.Intn(20)
		nodes := make([]*core.Node, n)
		for i := range nodes {
			nodes[i] = &core.Node{Id: i}
		}
		tree := map[*core.Node][]*core.Node{}
		for i := 1; i < n; i++ {
			parent := nodes[r.Intn(i)]
			tree[parent] = append(tree[parent], nodes[i])
		}
		index := NewDominateIndex(tree)
		for _, a := range nodes {
			for _, b := range nodes {
				want := IsDominate(tree, a, b)
				if got := index.IsDominate(a, b); got != want {
					t.Fatalf("round %d: IsDominate(%d,%d)=%t want %t", round, a.Id, b.Id, got, want)
				}
			}
		}
	}
}

func TestDominateIndexFallsBackForSharedChildren(t *testing.T) {
	root := &core.Node{}
	other := &core.Node{}
	shared := &core.Node{}
	leaf := &core.Node{}
	tree := map[*core.Node][]*core.Node{
		root:   {shared},
		other:  {shared},
		shared: {leaf},
	}
	index := NewDominateIndex(tree)
	for _, a := range []*core.Node{root, other, shared, leaf} {
		for _, b := range []*core.Node{root, other, shared, leaf} {
			want := IsDominate(tree, a, b)
			if got := index.IsDominate(a, b); got != want {
				t.Fatalf("shared child: IsDominate(%v,%v)=%t want %t", a, b, got, want)
			}
		}
	}
}

func TestDominateIndexEmptyTree(t *testing.T) {
	var nilIndex *DominateIndex
	a, b := &core.Node{}, &core.Node{}
	if nilIndex.IsDominate(a, b) {
		t.Fatal("nil index must not report dominance")
	}
	if !nilIndex.IsDominate(a, a) {
		t.Fatal("nil index must preserve the equality rule")
	}
	empty := NewDominateIndex(nil)
	if empty.IsDominate(a, b) {
		t.Fatal("empty index must not report dominance")
	}
}
