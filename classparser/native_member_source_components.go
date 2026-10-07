package javaclassparser

import (
	"sort"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Graph discovery and one source transaction have different resource domains.
// A wide acyclic dependency graph contains many independent commits; it must
// not inherit the small bound for families committed atomically in one SCC.
const (
	nativeMemberDependencyNodeLimit      = nativeMemberLayoutNodeLimit
	nativeMemberDependencyEdgeLimit      = 8192
	nativeMemberDependencyComponentLimit = 64
)

// Original declaration dependency cycles are transactions, not permission to
// share private access. Components identify the families whose source commits
// must succeed together; an incomplete graph cannot establish a component.
func nativeMemberSourceComponents(graph map[string]map[string]bool, work *workbudget.Budget) (map[string][]string, bool) {
	if len(graph) == 0 || len(graph) > nativeMemberDependencyNodeLimit || !nativeProofWork(work, int64(len(graph))) {
		return nil, false
	}
	var owners []string
	var nameBytes int64
	edgesSeen := 0
	for owner, edges := range graph {
		if owner == "" || len(edges) > nativeMemberDependencyNodeLimit {
			return nil, false
		}
		owners = append(owners, owner)
		nameBytes += int64(len(owner))
		for target, present := range edges {
			edgesSeen++
			nameBytes += int64(len(target)) + 32
			if edgesSeen > nativeMemberDependencyEdgeLimit || !nativeProofWork(work, 1) {
				return nil, false
			}
			if _, exists := graph[target]; !exists || !present {
				return nil, false
			}
		}
	}
	if work != nil && work.CheckAlloc(nameBytes*2+int64(len(graph))*512) != nil {
		return nil, false
	}
	sort.Strings(owners)
	index, low := map[string]int{}, map[string]int{}
	active := map[string]bool{}
	var stack []string
	components := map[string][]string{}
	sequence := 0
	var visit func(string) bool
	visit = func(owner string) bool {
		if !nativeProofWork(work, 1) {
			return false
		}
		sequence++
		index[owner], low[owner] = sequence, sequence
		stack = append(stack, owner)
		active[owner] = true
		var targets []string
		for target := range graph[owner] {
			targets = append(targets, target)
		}
		sort.Strings(targets)
		for _, target := range targets {
			if !nativeProofWork(work, 1) {
				return false
			}
			if index[target] == 0 {
				if !visit(target) {
					return false
				}
				if low[target] < low[owner] {
					low[owner] = low[target]
				}
			} else if active[target] && index[target] < low[owner] {
				low[owner] = index[target]
			}
		}
		if low[owner] == index[owner] {
			var component []string
			for {
				last := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				active[last] = false
				component = append(component, last)
				if last == owner {
					break
				}
			}
			if len(component) > nativeMemberDependencyComponentLimit {
				return false
			}
			sort.Strings(component)
			for _, name := range component {
				components[name] = component
			}
		}
		return true
	}
	for _, owner := range owners {
		if index[owner] == 0 && !visit(owner) {
			return nil, false
		}
	}
	return components, len(stack) == 0 && len(components) == len(graph)
}
