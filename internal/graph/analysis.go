// Package graph provides deterministic, reusable directed-graph analysis.
package graph

import "sort"

// Edge is a directed edge between two nodes.
type Edge[T comparable] struct {
	From T
	To   T
}

// SCC is one strongly connected region and its boundary. Nodes and entry
// targets follow the order supplied to Analyze. Entry and exit edges follow
// source-node order, then successor order.
type SCC[T comparable] struct {
	Nodes        []T
	Cyclic       bool
	EntryEdges   []Edge[T]
	EntryTargets []T
	ExitEdges    []Edge[T]
}

// Analysis is a deterministic snapshot of the root-reachable graph. SCCs
// contains every reachable component; CyclicSCCs contains the cyclic subset.
type Analysis[T comparable] struct {
	Reachable  []T
	SCCs       []SCC[T]
	CyclicSCCs []SCC[T]
}

// Analyze computes reachability, Tarjan strongly connected components, and
// component boundaries. Nodes defines the stable output order and must contain
// unique values. Roots or successors absent from nodes are ignored.
//
// A reachable root inside a component is an entry target even though it has no
// synthetic entry edge. External edges from unreachable nodes are excluded.
func Analyze[T comparable](nodes, roots []T, successors func(T) []T) Analysis[T] {
	unique := make([]T, 0, len(nodes))
	indexByNode := make(map[T]int, len(nodes))
	for _, node := range nodes {
		if _, exists := indexByNode[node]; exists {
			continue
		}
		indexByNode[node] = len(unique)
		unique = append(unique, node)
	}
	nodes = unique
	n := len(nodes)

	adjacency := make([][]int, n)
	if successors != nil {
		for from, node := range nodes {
			seen := make(map[int]struct{})
			for _, next := range successors(node) {
				to, ok := indexByNode[next]
				if !ok {
					continue
				}
				if _, duplicate := seen[to]; duplicate {
					continue
				}
				seen[to] = struct{}{}
				adjacency[from] = append(adjacency[from], to)
			}
		}
	}

	rootIndices := make([]int, 0, len(roots))
	rootSeen := make([]bool, n)
	for _, root := range roots {
		if index, ok := indexByNode[root]; ok && !rootSeen[index] {
			rootSeen[index] = true
			rootIndices = append(rootIndices, index)
		}
	}

	reachable := reachableNodes(adjacency, rootIndices)
	components := tarjan(adjacency, reachable)
	result := Analysis[T]{
		Reachable: make([]T, 0, n),
		SCCs:      make([]SCC[T], len(components)),
	}
	for i, ok := range reachable {
		if ok {
			result.Reachable = append(result.Reachable, nodes[i])
		}
	}

	componentOf := make([]int, n)
	for i := range componentOf {
		componentOf[i] = -1
	}
	entryTargets := make([]map[int]struct{}, len(components))
	for componentIndex, component := range components {
		region := &result.SCCs[componentIndex]
		region.Nodes = make([]T, len(component))
		region.Cyclic = len(component) > 1
		entryTargets[componentIndex] = make(map[int]struct{})
		for i, nodeIndex := range component {
			componentOf[nodeIndex] = componentIndex
			region.Nodes[i] = nodes[nodeIndex]
		}
	}

	for from := 0; from < n; from++ {
		if !reachable[from] {
			continue
		}
		fromComponent := componentOf[from]
		for _, to := range adjacency[from] {
			if !reachable[to] {
				continue
			}
			toComponent := componentOf[to]
			if fromComponent == toComponent {
				if from == to {
					result.SCCs[fromComponent].Cyclic = true
				}
				continue
			}
			edge := Edge[T]{From: nodes[from], To: nodes[to]}
			result.SCCs[fromComponent].ExitEdges = append(result.SCCs[fromComponent].ExitEdges, edge)
			result.SCCs[toComponent].EntryEdges = append(result.SCCs[toComponent].EntryEdges, edge)
			entryTargets[toComponent][to] = struct{}{}
		}
	}
	for _, root := range rootIndices {
		if reachable[root] {
			entryTargets[componentOf[root]][root] = struct{}{}
		}
	}
	for componentIndex, component := range components {
		region := &result.SCCs[componentIndex]
		for _, nodeIndex := range component {
			if _, entered := entryTargets[componentIndex][nodeIndex]; entered {
				region.EntryTargets = append(region.EntryTargets, nodes[nodeIndex])
			}
		}
		if region.Cyclic {
			result.CyclicSCCs = append(result.CyclicSCCs, *region)
		}
	}
	return result
}

// AnalyzeAdjacency analyzes an integer-index graph. The node order is
// 0..len(adjacency)-1; out-of-range roots and successors are ignored.
func AnalyzeAdjacency(adjacency [][]int, roots []int) Analysis[int] {
	nodes := make([]int, len(adjacency))
	for i := range nodes {
		nodes[i] = i
	}
	return Analyze(nodes, roots, func(node int) []int { return adjacency[node] })
}

func reachableNodes(adjacency [][]int, roots []int) []bool {
	reachable := make([]bool, len(adjacency))
	stack := make([]int, 0, len(roots))
	for i := len(roots) - 1; i >= 0; i-- {
		root := roots[i]
		if !reachable[root] {
			reachable[root] = true
			stack = append(stack, root)
		}
	}
	for len(stack) > 0 {
		last := len(stack) - 1
		from := stack[last]
		stack = stack[:last]
		for i := len(adjacency[from]) - 1; i >= 0; i-- {
			to := adjacency[from][i]
			if !reachable[to] {
				reachable[to] = true
				stack = append(stack, to)
			}
		}
	}
	return reachable
}

func tarjan(adjacency [][]int, reachable []bool) [][]int {
	indices := make([]int, len(adjacency))
	lowlink := make([]int, len(adjacency))
	for i := range indices {
		indices[i] = -1
	}
	onStack := make([]bool, len(adjacency))
	stack := make([]int, 0, len(adjacency))
	type frame struct {
		node   int
		parent int
		next   int
	}
	frames := make([]frame, 0, len(adjacency))
	components := make([][]int, 0)
	nextIndex := 0

	discover := func(node, parent int) {
		indices[node] = nextIndex
		lowlink[node] = nextIndex
		nextIndex++
		stack = append(stack, node)
		onStack[node] = true
		frames = append(frames, frame{node: node, parent: parent})
	}

	for start := range adjacency {
		if !reachable[start] || indices[start] >= 0 {
			continue
		}
		discover(start, -1)
		for len(frames) > 0 {
			frameIndex := len(frames) - 1
			current := &frames[frameIndex]
			if current.next < len(adjacency[current.node]) {
				to := adjacency[current.node][current.next]
				current.next++
				if !reachable[to] {
					continue
				}
				if indices[to] < 0 {
					discover(to, current.node)
					continue
				}
				if onStack[to] && indices[to] < lowlink[current.node] {
					lowlink[current.node] = indices[to]
				}
				continue
			}

			node, parent := current.node, current.parent
			frames = frames[:frameIndex]
			if parent >= 0 && lowlink[node] < lowlink[parent] {
				lowlink[parent] = lowlink[node]
			}
			if lowlink[node] != indices[node] {
				continue
			}
			component := make([]int, 0, 1)
			for {
				last := len(stack) - 1
				member := stack[last]
				stack = stack[:last]
				onStack[member] = false
				component = append(component, member)
				if member == node {
					break
				}
			}
			sort.Ints(component)
			components = append(components, component)
		}
	}

	sort.Slice(components, func(i, j int) bool { return components[i][0] < components[j][0] })
	return components
}
