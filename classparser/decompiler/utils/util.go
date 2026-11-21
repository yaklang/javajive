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
	tree           map[*core.Node][]*core.Node
	parent         map[*core.Node]*core.Node
	tin            map[*core.Node]int32
	tout           map[*core.Node]int32
	valid          bool
	intervalsValid bool
}

type domIntervalFrame struct {
	node     *core.Node
	childIdx int
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
	d.intervalsValid = false
	if d.parent == nil {
		d.parent = make(map[*core.Node]*core.Node, len(tree))
	} else {
		clear(d.parent)
	}
	if d.tin == nil {
		d.tin = make(map[*core.Node]int32, len(tree))
		d.tout = make(map[*core.Node]int32, len(tree))
	} else {
		clear(d.tin)
		clear(d.tout)
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
	if !d.valid {
		return
	}
	// Euler-tour timestamps on the single-parent dominator forest answer
	// dominance queries in O(1) without walking parent chains.
	var roots []*core.Node
	for dom := range tree {
		if _, hasParent := d.parent[dom]; !hasParent {
			roots = append(roots, dom)
		}
	}
	visitedChildren := 0
	var timer int32
	stack := make([]domIntervalFrame, 0, len(tree))
	for _, root := range roots {
		if _, seen := d.tin[root]; seen {
			return
		}
		timer++
		d.tin[root] = timer
		stack = append(stack, domIntervalFrame{node: root})
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			children := tree[top.node]
			if top.childIdx < len(children) {
				child := children[top.childIdx]
				top.childIdx++
				if child == nil || child == top.node {
					return
				}
				if _, seen := d.tin[child]; seen {
					return
				}
				visitedChildren++
				timer++
				d.tin[child] = timer
				stack = append(stack, domIntervalFrame{node: child})
			} else {
				timer++
				d.tout[top.node] = timer
				stack = stack[:len(stack)-1]
			}
		}
	}
	if visitedChildren == len(d.parent) {
		d.intervalsValid = true
	}
}

func (d *DominateIndex) IsDominate(node1, node2 *core.Node) bool {
	if node1 == node2 {
		return true
	}
	if d != nil && d.valid && d.intervalsValid {
		in1, ok1 := d.tin[node1]
		in2, ok2 := d.tin[node2]
		return ok1 && ok2 && in1 <= in2 && d.tout[node2] <= d.tout[node1]
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
