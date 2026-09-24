package ssabuild

import (
	"fmt"
	"sort"
)

// bindValues assigns IDs only after convergence. Phi identities are structural;
// instruction result identities are method-local (PC, result index), so revisiting
// a block changes operand bindings without allocating another definition.
func bindValues(fn *Function, ctr WorkCounter) error {
	for _, b := range fn.Blocks {
		if err := charge(ctr, uint64(1+len(b.InOrig)+len(b.OutOrig))); err != nil {
			return err
		}
	}
	for _, r := range fn.Instructions {
		if err := charge(ctr, uint64(1+len(r.Uses)+len(r.Results)+len(r.BeforeOrigins))); err != nil {
			return err
		}
	}
	for _, s := range fn.EdgeStates {
		if err := charge(ctr, uint64(1+len(s.Origins))); err != nil {
			return err
		}
	}
	ids := map[Origin]ValueID{}
	for _, p := range fn.Phis {
		var block *BlockFrame
		for i := range fn.Blocks {
			if fn.Blocks[i].ID == p.Block {
				block = &fn.Blocks[i]
				break
			}
		}
		if block == nil {
			return fmt.Errorf("invalid_input: missing phi block")
		}
		slot := p.Slot.Index
		if !p.Slot.Local {
			slot += len(block.In.Locals)
		}
		ids[Origin{Kind: OriginPhi, PC: block.First, Slot: slot, Aux: int(p.Block)}] = p.ID
	}
	all := map[Origin]bool{}
	add := func(origins []Origin) {
		for _, o := range origins {
			if o.Kind != OriginTop {
				all[o] = true
			}
		}
	}
	for _, b := range fn.Blocks {
		add(b.InOrig)
		add(b.OutOrig)
	}
	for _, r := range fn.Instructions {
		add(r.Uses)
		add(r.Results)
		add(r.BeforeOrigins)
	}
	for _, s := range fn.EdgeStates {
		add(s.Origins)
	}
	for _, p := range fn.Phis {
		for _, o := range p.Operands {
			add([]Origin{o.Origin})
		}
	}
	origins := make([]Origin, 0, len(all))
	for o := range all {
		origins = append(origins, o)
	}
	sort.Slice(origins, func(i, j int) bool { return origins[i].Key() < origins[j].Key() })
	next := ValueID(len(fn.Phis) + 1)
	for _, o := range origins {
		if _, ok := ids[o]; ok {
			continue
		}
		if o.Kind == OriginPhi {
			return fmt.Errorf("invalid_input: undefined phi origin %s", o.Key())
		}
		ids[o] = next
		next++
	}
	for _, o := range origins {
		fn.Values = append(fn.Values, ValueDefinition{ID: ids[o], Origin: o})
	}
	sort.Slice(fn.Values, func(i, j int) bool { return fn.Values[i].ID < fn.Values[j].ID })
	for i := range fn.Phis {
		for j := range fn.Phis[i].Operands {
			op := &fn.Phis[i].Operands[j]
			op.Val = ids[op.Origin]
		}
	}
	for i := range fn.Instructions {
		r := &fn.Instructions[i]
		for _, o := range r.Uses {
			r.UseIDs = append(r.UseIDs, ids[o])
		}
		for _, o := range r.Results {
			r.ResultIDs = append(r.ResultIDs, ids[o])
		}
	}
	if fn.entryEdge != nil {
		entry := fn.EdgeStates[fn.entryEdge.ID]
		for i, t := range entry.Frame.Locals {
			if t.Computational() {
				fn.Params = append(fn.Params, ids[entry.Origins[i]])
			}
		}
	}
	return nil
}
