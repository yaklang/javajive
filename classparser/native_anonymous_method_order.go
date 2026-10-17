package javaclassparser

import (
	"container/heap"
	"math/bits"
	"sort"
)

type nativeAnonymousMethodIndices []int

func (h nativeAnonymousMethodIndices) Len() int           { return len(h) }
func (h nativeAnonymousMethodIndices) Less(i, j int) bool { return h[i] < h[j] }
func (h nativeAnonymousMethodIndices) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *nativeAnonymousMethodIndices) Push(v any)        { *h = append(*h, v.(int)) }
func (h *nativeAnonymousMethodIndices) Pop() any {
	n := len(*h) - 1
	v := (*h)[n]
	*h = (*h)[:n]
	return v
}

// Classfile method order puts <clinit> after ordinary declarations even when
// its anonymous classes were registered first in the original source. Method
// declarations do not execute at their textual positions. Schedule complete
// declarations by their proved anonymous ordinals, while preserving the order
// of every initialization block. Never split a body or move a statement/field.
func (c *ClassObjectDumper) nativeAnonymousMethodOrder(methods []*dumpedMethods) ([]*dumpedMethods, bool) {
	group := c.nativeAnonymousRoot
	if group == nil || len(group.children) < 2 {
		return methods, true
	}
	if c.obj == nil || group.failed || group.owner != c.obj.GetClassName() || len(methods) > 65535 || !nativeProofWork(c.Work, int64(len(methods))+1) {
		return nil, false
	}
	if c.Work != nil && c.Work.CheckAlloc(int64(len(methods))*96+int64(len(group.children))*32) != nil {
		return nil, false
	}
	type interval struct{ index, first, last int }
	var intervals []interval
	seen := map[int]bool{}
	for i, method := range methods {
		if method == nil || !nativeProofWork(c.Work, int64(len(method.code))+1) {
			return nil, false
		}
		ordinals, known := nativeAnonymousOrdinalsWithinOwner(method.code, group.owner)
		if !known {
			return nil, false
		}
		if len(ordinals) == 0 {
			continue
		}
		for j, ordinal := range ordinals {
			if ordinal <= group.enumPrefix || ordinal > group.enumPrefix+len(group.children) || seen[ordinal] || j > 0 && ordinal != ordinals[j-1]+1 {
				return nil, false
			}
			proved := false
			for _, child := range group.children {
				if !nativeProofWork(c.Work, 1) {
					return nil, false
				}
				if child != nil && child.ordinal == ordinal && child.object != nil {
					owner, _, original := originalAnonymousOwner(child.object)
					proved = original && owner == group.owner
					break
				}
			}
			if !proved {
				return nil, false
			}
			seen[ordinal] = true
		}
		intervals = append(intervals, interval{i, ordinals[0], ordinals[len(ordinals)-1]})
	}
	if len(intervals) < 2 {
		return methods, true
	}
	ordered := true
	for i := 1; i < len(intervals); i++ {
		ordered = ordered && intervals[i-1].last < intervals[i].first
	}
	if ordered {
		return methods, true
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].first < intervals[j].first })
	edges := make([][]int, len(methods))
	indegree := make([]int, len(methods))
	add := func(from, to int) {
		for _, prior := range edges[from] {
			if prior == to {
				return
			}
		}
		edges[from] = append(edges[from], to)
		indegree[to]++
	}
	for i := 1; i < len(intervals); i++ {
		if intervals[i-1].last >= intervals[i].first {
			return nil, false
		}
		add(intervals[i-1].index, intervals[i].index)
	}
	previousInitializer := -1
	for i, method := range methods {
		if method.methodName == "<clinit>" || method.methodName == "<initializer>" {
			if previousInitializer >= 0 {
				add(previousInitializer, i)
			}
			previousInitializer = i
		}
	}
	// A stable ready queue retains the existing order whenever the original
	// registration constraints permit it. Cycles mean these complete atoms
	// cannot realize both initialization order and declaration identity.
	ready := make(nativeAnonymousMethodIndices, 0, len(methods))
	for i, degree := range indegree {
		if degree == 0 {
			ready = append(ready, i)
		}
	}
	heap.Init(&ready)
	result := make([]*dumpedMethods, 0, len(methods))
	for len(ready) != 0 {
		if !nativeProofWork(c.Work, int64(bits.Len(uint(len(methods)))+1)*3) {
			return nil, false
		}
		node := heap.Pop(&ready).(int)
		result = append(result, methods[node])
		for _, next := range edges[node] {
			indegree[next]--
			if indegree[next] == 0 {
				heap.Push(&ready, next)
			}
		}
	}
	return result, len(result) == len(methods)
}
