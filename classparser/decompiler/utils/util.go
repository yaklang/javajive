package utils

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func LinkNode(src, target *core.Node) {
	target.Source = append(target.Source, src)
	src.Next = append(src.Next, target)
}

func NodeFilter(nodes []*core.Node, f func(node *core.Node) bool) []*core.Node {
	var res []*core.Node
	for _, node := range nodes {
		if f(node) {
			res = append(res, node)
		}
	}
	return res
}

func IsDominate(tree map[*core.Node][]*core.Node, node1, node2 *core.Node) bool {
	if node1 == node2 {
		return true
	}
	doms := tree[node1]
	for _, dom := range doms {
		if dom == node2 {
			return true
		} else {
			if IsDominate(tree, dom, node2) {
				return true
			}
		}
	}
	return false
}

// DominateIndex answers IsDominate queries through child->parent links, so a
// query costs the depth of node2 instead of scanning node1's whole subtree.
// The dominator map produced by GenerateDominatorTree is a forest (one parent
// per node), where both formulations are equivalent. A map with a shared child
// is not a tree and falls back to the recursive implementation, preserving its
// behavior for hand-built test fixtures.
type DominateIndex struct {
	tree   map[*core.Node][]*core.Node
	parent map[*core.Node]*core.Node
	valid  bool
}

func NewDominateIndex(tree map[*core.Node][]*core.Node) *DominateIndex {
	index := &DominateIndex{}
	index.Reset(tree)
	return index
}

// Reset rebuilds the index for a new dominator tree while retaining the parent
// map's buckets. Rewriter topology refreshes use this to avoid a new map per
// rewrite step.
func (d *DominateIndex) Reset(tree map[*core.Node][]*core.Node) {
	if d == nil {
		return
	}
	d.tree = tree
	d.valid = true
	if d.parent == nil {
		d.parent = make(map[*core.Node]*core.Node, len(tree))
	} else {
		clear(d.parent)
	}
	for dom, children := range tree {
		for _, child := range children {
			if child == nil {
				d.valid = false
				continue
			}
			if previous, ok := d.parent[child]; ok && previous != dom {
				d.valid = false
				continue
			}
			d.parent[child] = dom
		}
	}
}

func (d *DominateIndex) IsDominate(node1, node2 *core.Node) bool {
	if node1 == node2 {
		return true
	}
	if d == nil || !d.valid {
		if d == nil {
			return false
		}
		return IsDominate(d.tree, node1, node2)
	}
	// One parent per node makes an ancestor walk equivalent to a subtree
	// search. The step bound also terminates on a malformed cyclic map where
	// the recursive form would not.
	for current, steps := node2, 0; current != nil && steps <= len(d.parent); current, steps = d.parent[current], steps+1 {
		if current == node1 {
			return true
		}
	}
	return false
}
