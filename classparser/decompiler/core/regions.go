package core

import "fmt"

// ValidateReducible rejects SCCs with multiple normal entry nodes. Such regions
// need node splitting or an explicit state machine; the current structurer must
// not silently turn them into a natural loop. Exceptional edges are not loop
// backedges; handler control flow is validated separately from this normal region.
func (g *SemanticCFG) ValidateReducible() error {
	// Exception handlers start separate normal-flow regions. Their exceptional
	// re-entry into a protected loop does not make that loop irreducible.
	reachable := map[*OpCode]bool{}
	if len(g.Nodes) > 0 {
		queue := []*OpCode{g.Nodes[0]}
		for len(queue) > 0 {
			v := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			if reachable[v] {
				continue
			}
			reachable[v] = true
			for _, ei := range g.outgoing[v] {
				if e := g.Edges[ei]; e.Kind != EdgeException {
					queue = append(queue, e.To)
				}
			}
		}
	}
	index, low := map[*OpCode]int{}, map[*OpCode]int{}
	on := map[*OpCode]bool{}
	stack := []*OpCode{}
	serial := 0
	var failure error
	var visit func(*OpCode)
	visit = func(v *OpCode) {
		serial++
		index[v] = serial
		low[v] = serial
		stack = append(stack, v)
		on[v] = true
		for _, ei := range g.outgoing[v] {
			e := g.Edges[ei]
			if e.Kind == EdgeException {
				continue
			}
			w := e.To
			if index[w] == 0 {
				visit(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if on[w] && index[w] < low[v] {
				low[v] = index[w]
			}
		}
		if low[v] != index[v] {
			return
		}
		component := map[*OpCode]bool{}
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			on[w] = false
			component[w] = true
			if w == v {
				break
			}
		}
		if len(component) < 2 {
			return
		}
		entries := map[*OpCode]bool{}
		for n := range component {
			if len(g.Nodes) > 0 && n == g.Nodes[0] {
				entries[n] = true
			}
			for _, ei := range g.incoming[n] {
				if edge := g.Edges[ei]; edge.Kind != EdgeException && reachable[edge.From] && !component[edge.From] {
					entries[n] = true
				}
			}
		}
		if len(entries) > 1 && failure == nil {
			failure = fmt.Errorf("unsupported_irreducible_control_flow: region near PC %d has %d entries", v.CurrentOffset, len(entries))
		}
	}
	for _, n := range g.Nodes {
		if reachable[n] && index[n] == 0 {
			visit(n)
		}
	}
	return failure
}
