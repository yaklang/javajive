package rewriter

import "github.com/yaklang/javajive/classparser/decompiler/core"

// cyclicCFGNodes returns nodes that belong to a cycle in the executable method CFG. Exception
// edges are included: a handler can transfer back into a protected region, so removing dispatch
// edges can hide a real cycle. Acyclic forward diamonds remain excluded by SCC membership.
func cyclicCFGNodes(root *core.Node) map[*core.Node]bool {
	nodes := make([]*core.Node, 0)
	if root == nil {
		return map[*core.Node]bool{}
	}
	_ = core.WalkGraph[*core.Node](root, func(node *core.Node) ([]*core.Node, error) {
		nodes = append(nodes, node)
		return node.Next, nil
	})

	successors := make(map[*core.Node][]*core.Node, len(nodes))
	predecessors := make(map[*core.Node][]*core.Node, len(nodes))
	for _, node := range nodes {
		for _, next := range node.Next {
			successors[node] = append(successors[node], next)
			predecessors[next] = append(predecessors[next], node)
		}
	}

	// Iterative Kosaraju avoids recursion depth depending on generated method size.
	type frame struct {
		node *core.Node
		next int
	}
	seen := make(map[*core.Node]bool, len(nodes))
	order := make([]*core.Node, 0, len(nodes))
	for _, start := range nodes {
		if seen[start] {
			continue
		}
		seen[start] = true
		stack := []frame{{node: start}}
		for len(stack) != 0 {
			last := len(stack) - 1
			current := &stack[last]
			if current.next < len(successors[current.node]) {
				next := successors[current.node][current.next]
				current.next++
				if !seen[next] {
					seen[next] = true
					stack = append(stack, frame{node: next})
				}
				continue
			}
			order = append(order, current.node)
			stack = stack[:last]
		}
	}

	assigned := make(map[*core.Node]bool, len(nodes))
	cyclic := make(map[*core.Node]bool)
	for i := len(order) - 1; i >= 0; i-- {
		start := order[i]
		if assigned[start] {
			continue
		}
		component := make([]*core.Node, 0, 1)
		assigned[start] = true
		stack := []*core.Node{start}
		for len(stack) != 0 {
			last := len(stack) - 1
			current := stack[last]
			stack = stack[:last]
			component = append(component, current)
			for _, prev := range predecessors[current] {
				if !assigned[prev] {
					assigned[prev] = true
					stack = append(stack, prev)
				}
			}
		}

		inCycle := len(component) > 1
		if !inCycle {
			for _, next := range successors[start] {
				if next == start {
					inCycle = true
					break
				}
			}
		}
		if inCycle {
			for _, node := range component {
				cyclic[node] = true
			}
		}
	}
	return cyclic
}
