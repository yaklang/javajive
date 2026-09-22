package ssalower

import (
	"fmt"
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
)

// Spill reads an already materialized pre-throw local. An emitter must read
// LocalSlot at PC, never re-evaluate the instruction recorded in Source's origin.
// Destination is a dedicated phi local declared outside the protected region.
type Spill struct {
	Handler             methodir.BlockID
	Destination, Source VarID
	LocalSlot           int
	Type                frametransfer.Type
}
type SpillSite struct {
	PC       uint16
	Coverage []Range
	Spills   []Spill
	Moves    []Move
}

// CatchBinding binds a handler's stack value from its catch parameter, which
// exists only AFTER throwing. It must never be used as a pre-throw copy source.
type CatchBinding struct {
	Handler     methodir.BlockID
	Destination VarID
	Type        frametransfer.Type
}
type ExceptionPlan struct {
	Sites []SpillSite
	Catch []CatchBinding
}
type Options struct{ MaxSpills int }

func exceptionPlan(fn *ssabuild.Function, values *ValueRegistry, cov map[uint16][]Range, maxSpills int) (ExceptionPlan, error) {
	fail := func(format string, args ...any) (ExceptionPlan, error) {
		return ExceptionPlan{}, fmt.Errorf("unsupported: exception phi: "+format, args...)
	}
	if maxSpills <= 0 {
		return fail("positive spill budget required")
	}
	byEdge := map[methodir.EdgeID]methodir.Edge{}
	handlers := map[methodir.BlockID]bool{}
	for _, e := range fn.IR.Edges {
		if _, ok := byEdge[e.ID]; ok {
			return fail("duplicate edge %s", e.ID)
		}
		byEdge[e.ID] = e
		if e.Kind != core.EdgeException {
			continue
		}
		if _, ok := fn.EdgeStates[e.ID]; !ok {
			continue
		}
		b, ok := fn.IR.BlockOf(e.To)
		if !ok {
			return fail("handler block missing")
		}
		handlers[b.ID] = true
	}
	out := ExceptionPlan{}
	sites := map[uint16]*SpillSite{}
	before := map[uint16]ssabuild.InstructionValues{}
	for _, v := range fn.Instructions {
		before[v.PC] = v
	}
	count := 0
	// Iterate blocks and phis in deterministic order, never a map traversal.
	blocks := append([]ssabuild.BlockFrame(nil), fn.Blocks...)
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].ID < blocks[j].ID })
	for _, b := range blocks {
		if !handlers[b.ID] {
			continue
		}
		// A catch parameter has one reference stack value. Mixed normal entry cannot
		// be represented by this handler-local emission plan.
		for _, e := range fn.Incoming(b.First) {
			if _, ok := fn.EdgeStates[e.ID]; ok && e.Kind != core.EdgeException {
				return fail("handler %d has normal or entry predecessor", b.ID)
			}
		}
		if len(b.In.Stack) != 1 || b.In.Stack[0].Kind != frametransfer.Ref || len(b.InOrig) != len(b.In.Locals)+1 {
			return fail("handler %d lacks one typed catch stack value", b.ID)
		}
		catchID, err := values.Origin(b.InOrig[len(b.In.Locals)])
		if err != nil {
			return ExceptionPlan{}, err
		}
		out.Catch = append(out.Catch, CatchBinding{Handler: b.ID, Destination: catchID, Type: b.In.Stack[0].DropValue()})
		phis := fn.PhisOf(b.ID)
		sort.Slice(phis, func(i, j int) bool { return phis[i].ID < phis[j].ID })
		for _, p := range phis {
			expected := map[methodir.EdgeID]bool{}
			for _, e := range fn.Incoming(b.First) {
				if _, ok := fn.EdgeStates[e.ID]; ok {
					expected[e.ID] = true
				}
			}
			if len(expected) == 0 || len(p.Operands) != len(expected) {
				return fail("phi %d missing predecessor operand", p.ID)
			}
			used := map[methodir.EdgeID]bool{}
			for _, op := range p.Operands {
				if !expected[op.Edge] || used[op.Edge] {
					return fail("phi %d operand/edge mismatch", p.ID)
				}
				used[op.Edge] = true
				state := fn.EdgeStates[op.Edge]
				slot := p.Slot.Index
				if !p.Slot.Local {
					slot += len(state.Frame.Locals)
				}
				if slot < 0 || slot >= len(state.Origins) || state.Origins[slot] != op.Origin {
					return fail("phi %d operand does not match edge snapshot", p.ID)
				}
			}
			dst, err := values.Phi(p)
			if err != nil {
				return ExceptionPlan{}, err
			}
			if !p.Slot.Local {
				if p.Slot.Index != 0 || dst != catchID {
					return fail("phi %d is not catch binding", p.ID)
				}
				continue
			}
			if !spillType(p.Type) {
				return fail("phi %d has uninitialized or noncomputational type", p.ID)
			}
			for _, op := range p.Operands {
				e := byEdge[op.Edge]
				pc := uint16(e.From)
				state := fn.EdgeStates[op.Edge]
				pre, ok := before[pc]
				slot := p.Slot.Index
				if !ok || slot < 0 || slot >= len(pre.Before.Locals) || slot >= len(pre.BeforeOrigins) || slot >= len(state.Frame.Locals) || slot >= len(state.Origins) {
					return fail("PC %d lacks pre-throw local snapshot", pc)
				}
				typ := pre.Before.Locals[slot]
				if !spillType(typ) || !typ.DropValue().Equal(p.Type.DropValue()) || !state.Frame.Locals[slot].DropValue().Equal(typ.DropValue()) {
					return fail("PC %d local %d requires type conversion or initialization", pc, slot)
				}
				if pre.BeforeOrigins[slot] != op.Origin || state.Origins[slot] != op.Origin {
					return fail("PC %d operand is not before-state local %d", pc, slot)
				}
				src, err := values.Origin(op.Origin)
				if err != nil {
					return ExceptionPlan{}, err
				}
				site := sites[pc]
				if site == nil {
					site = &SpillSite{PC: pc, Coverage: rangesForPC(cov, pc)}
					sites[pc] = site
				}
				duplicate := false
				for _, s := range site.Spills {
					if s.Destination == dst {
						if s.Source != src || s.LocalSlot != slot {
							return fail("PC %d conflicting destination %d", pc, dst)
						}
						duplicate = true
					}
				}
				if duplicate {
					continue
				}
				if count >= maxSpills {
					return fail("spill budget exceeded")
				}
				count++
				site.Spills = append(site.Spills, Spill{Handler: b.ID, Destination: dst, Source: src, LocalSlot: slot, Type: p.Type.DropValue()})
			}
		}
	}
	pcs := make([]int, 0, len(sites))
	for pc := range sites {
		pcs = append(pcs, int(pc))
	}
	sort.Ints(pcs)
	for _, pc := range pcs {
		s := sites[uint16(pc)]
		sort.Slice(s.Spills, func(i, j int) bool { return s.Spills[i].Destination < s.Spills[j].Destination })
		copies := make([]Copy, 0, len(s.Spills))
		for _, sp := range s.Spills {
			copies = append(copies, Copy{Dst: sp.Destination, Src: sp.Source})
		}
		moves, err := values.sequentialize(copies)
		if err != nil {
			return ExceptionPlan{}, err
		}
		s.Moves = moves
		out.Sites = append(out.Sites, *s)
	}
	return out, nil
}
func spillType(t frametransfer.Type) bool {
	return t.Computational() && t.Kind != frametransfer.UninitNew && t.Kind != frametransfer.UninitThis
}
