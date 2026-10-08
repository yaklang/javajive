package ssabuild

import "fmt"

// simplifyPhiCopies solves copying SCCs after the frame/origin fixed point.
// A component whose only external input is one value denotes that same value,
// including mutually recursive copies. Two external identities remain distinct
// even when their types or runtime values happen to be equal. No predecessor
// edge or computational instruction is removed.
func simplifyPhiCopies(fn *Function, ctr WorkCounter) error {
	if len(fn.Phis) == 0 {
		return nil
	}
	blocks := map[int]BlockFrame{}
	for _, b := range fn.Blocks {
		if err := charge(ctr, 1); err != nil {
			return err
		}
		blocks[int(b.ID)] = b
	}
	origins := make([]Origin, len(fn.Phis))
	index := map[Origin]int{}
	for i, p := range fn.Phis {
		if err := charge(ctr, 1); err != nil {
			return err
		}
		b, ok := blocks[int(p.Block)]
		if !ok {
			return fmt.Errorf("invalid_input: missing phi block")
		}
		slot := p.Slot.Index
		if !p.Slot.Local {
			slot += len(b.In.Locals)
		}
		o := Origin{Kind: OriginPhi, PC: b.First, Slot: slot, Aux: int(p.Block)}
		if _, duplicate := index[o]; duplicate {
			return fmt.Errorf("invalid_input: duplicate phi equation")
		}
		origins[i], index[o] = o, i
	}
	forward, reverse := make([][]int, len(fn.Phis)), make([][]int, len(fn.Phis))
	for i, p := range fn.Phis {
		for _, operand := range p.Operands {
			if err := charge(ctr, 1); err != nil {
				return err
			}
			if operand.Origin.Kind != OriginPhi {
				continue
			}
			j, known := index[operand.Origin]
			if !known {
				return fmt.Errorf("invalid_input: undefined phi input %s", operand.Origin.Key())
			}
			forward[i] = append(forward[i], j)
			reverse[j] = append(reverse[j], i)
		}
	}
	// Iterative Kosaraju traversal avoids a call-stack dependency on adversarial
	// bytecode. Every vertex/edge visit shares the original analysis budget.
	type visit struct{ node, next int }
	seen := make([]bool, len(fn.Phis))
	order := make([]int, 0, len(fn.Phis))
	for start := range fn.Phis {
		if seen[start] {
			continue
		}
		seen[start] = true
		stack := []visit{{node: start}}
		for len(stack) != 0 {
			if err := charge(ctr, 1); err != nil {
				return err
			}
			top := &stack[len(stack)-1]
			if top.next == len(forward[top.node]) {
				order = append(order, top.node)
				stack = stack[:len(stack)-1]
				continue
			}
			next := forward[top.node][top.next]
			top.next++
			if !seen[next] {
				seen[next] = true
				stack = append(stack, visit{node: next})
			}
		}
	}
	component := make([]int, len(fn.Phis))
	for i := range component {
		component[i] = -1
	}
	groups := [][]int{}
	for at := len(order) - 1; at >= 0; at-- {
		start := order[at]
		if component[start] >= 0 {
			continue
		}
		id := len(groups)
		component[start] = id
		pending, members := []int{start}, []int{}
		for len(pending) > 0 {
			if err := charge(ctr, 1); err != nil {
				return err
			}
			n := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			members = append(members, n)
			for _, next := range reverse[n] {
				if err := charge(ctr, 1); err != nil {
					return err
				}
				if component[next] < 0 {
					component[next] = id
					pending = append(pending, next)
				}
			}
		}
		groups = append(groups, members)
	}
	aliases := map[Origin]Origin{}
	resolve := func(o Origin) Origin {
		if replacement, known := aliases[o]; known {
			return replacement // dependency components were already resolved
		}
		return o
	}
	// Reverse component order visits inputs before their users. Each alias is
	// stored with its final representative, so rewriting uses is linear.
	for id := len(groups) - 1; id >= 0; id-- {
		var external Origin
		have, different := false, false
		for _, n := range groups[id] {
			for _, operand := range fn.Phis[n].Operands {
				if err := charge(ctr, 1); err != nil {
					return err
				}
				o := operand.Origin
				if j, known := index[o]; known && component[j] == id {
					continue
				}
				o = resolve(o)
				if !have {
					external, have = o, true
				} else if o != external {
					different = true
				}
			}
		}
		if !have {
			return fmt.Errorf("invalid_input: phi cycle has no incoming value")
		}
		if !different {
			for _, n := range groups[id] {
				aliases[origins[n]] = external
			}
		}
	}
	rewrite := func(list []Origin) error {
		for i, o := range list {
			if err := charge(ctr, 1); err != nil {
				return err
			}
			list[i] = resolve(o)
		}
		return nil
	}
	for i := range fn.Blocks {
		if err := rewrite(fn.Blocks[i].InOrig); err != nil {
			return err
		}
		if err := rewrite(fn.Blocks[i].OutOrig); err != nil {
			return err
		}
	}
	for i := range fn.Instructions {
		for _, list := range [][]Origin{fn.Instructions[i].BeforeOrigins, fn.Instructions[i].Uses, fn.Instructions[i].Results} {
			if err := rewrite(list); err != nil {
				return err
			}
		}
	}
	for _, state := range fn.EdgeStates {
		if err := rewrite(state.Origins); err != nil {
			return err
		}
	}
	retained := fn.Phis[:0]
	for i, p := range fn.Phis {
		if _, removed := aliases[origins[i]]; removed {
			continue
		}
		for j := range p.Operands {
			if err := charge(ctr, 1); err != nil {
				return err
			}
			p.Operands[j].Origin = resolve(p.Operands[j].Origin)
		}
		p.ID = ValueID(len(retained) + 1)
		retained = append(retained, p)
	}
	fn.Phis = retained
	return nil
}
