package ssalower

import (
	"fmt"
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
)

// ValueKey is a structural method-local identity. Domain separates phi
// definitions from origins; no PC/slot/Aux coordinate is arithmetically packed.
type ValueKey struct{ Domain, Kind, PC, Slot, Aux int }

func OriginKey(o ssabuild.Origin) ValueKey {
	return ValueKey{Kind: int(o.Kind), PC: int(o.PC), Slot: o.Slot, Aux: o.Aux}
}
func PhiKey(p ssabuild.Phi) ValueKey { return ValueKey{Domain: 1, Aux: int(p.ID)} }

type ValueInfo struct {
	Type      frametransfer.Type
	Width     int
	Origin    ssabuild.Origin
	Temporary bool
	Source    VarID
}

// ValueRegistry belongs to one lowering request and reserves temporaries after
// ALL method values, including values not read on the currently lowered edge.
type ValueRegistry struct {
	ids   map[ValueKey]VarID
	Info  map[VarID]ValueInfo
	next  VarID
	temps int
}

func NewValueRegistry(keys []ValueKey, aliases map[ValueKey]ValueKey) (*ValueRegistry, error) {
	return newValueRegistry(keys, aliases, nil)
}

func newValueRegistry(keys []ValueKey, aliases map[ValueKey]ValueKey, counter ssabuild.WorkCounter) (*ValueRegistry, error) {
	keyCount, aliasCount := uint64(len(keys)), uint64(len(aliases))
	if aliasCount > (^uint64(0)-keyCount)/2 {
		if err := chargeWork(counter, ^uint64(0)); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("analysis_budget_exceeded: value registry work overflow")
	}
	if err := chargeWork(counter, keyCount+2*aliasCount); err != nil {
		return nil, err
	}
	canonical := func(k ValueKey) (ValueKey, error) {
		for hops := 0; ; hops++ {
			if err := chargeWork(counter, 1); err != nil {
				return ValueKey{}, err
			}
			if hops > len(aliases) {
				return ValueKey{}, fmt.Errorf("invalid_input: value alias cycle at %+v", k)
			}
			n, ok := aliases[k]
			if !ok {
				return k, nil
			}
			k = n
		}
	}
	all := append([]ValueKey(nil), keys...)
	for a, b := range aliases {
		all = append(all, a, b)
	}
	set := make(map[ValueKey]bool, len(all))
	for _, k := range all {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		c, err := canonical(k)
		if err != nil {
			return nil, err
		}
		set[c] = true
	}
	if err := chargeWork(counter, uint64(len(set))); err != nil {
		return nil, err
	}
	sorted := make([]ValueKey, 0, len(set))
	for k := range set {
		sorted = append(sorted, k)
	}
	if err := chargeWork(counter, sortWork(len(sorted))); err != nil {
		return nil, err
	}
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		aa, bb := [5]int{a.Domain, a.Kind, a.PC, a.Slot, a.Aux}, [5]int{b.Domain, b.Kind, b.PC, b.Slot, b.Aux}
		for n := range aa {
			if aa[n] != bb[n] {
				return aa[n] < bb[n]
			}
		}
		return false
	})
	// Reserve map construction before make; capacities are bounded by the
	// already charged key and alias lists.
	if err := chargeScaledWork(counter, len(sorted), 2); err != nil {
		return nil, err
	}
	r := &ValueRegistry{ids: make(map[ValueKey]VarID, len(all)), Info: make(map[VarID]ValueInfo, len(sorted)), next: 1}
	for _, k := range sorted {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		r.ids[k] = r.next
		r.next++
	}
	for _, k := range all {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		c, err := canonical(k)
		if err != nil {
			return nil, err
		}
		r.ids[k] = r.ids[c]
	}
	return r, nil
}
func (r *ValueRegistry) Lookup(k ValueKey) (VarID, error) {
	if r == nil {
		return 0, fmt.Errorf("invalid_input: nil value registry")
	}
	id, ok := r.ids[k]
	if !ok {
		return 0, fmt.Errorf("invalid_input: unregistered value %+v", k)
	}
	return id, nil
}
func (r *ValueRegistry) Origin(o ssabuild.Origin) (VarID, error) { return r.Lookup(OriginKey(o)) }
func (r *ValueRegistry) Phi(p ssabuild.Phi) (VarID, error)       { return r.Lookup(PhiKey(p)) }
func (r *ValueRegistry) Fresh() (VarID, error) {
	if r.next <= 0 || r.next == VarID(int(^uint(0)>>1)) {
		return 0, fmt.Errorf("unsupported: value identity exhausted")
	}
	id := r.next
	r.next++
	r.temps++
	return id, nil
}

func registryFor(fn *ssabuild.Function) (*ValueRegistry, error) {
	return registryForWithCounter(fn, nil)
}

func registryForWithCounter(fn *ssabuild.Function, counter ssabuild.WorkCounter) (*ValueRegistry, error) {
	keys := []ValueKey{}
	aliases := map[ValueKey]ValueKey{}
	types := map[ValueKey]ValueInfo{}
	register := func(o ssabuild.Origin, t frametransfer.Type) error {
		if err := chargeWork(counter, 1); err != nil {
			return err
		}
		if !t.Computational() {
			return nil
		}
		k := OriginKey(o)
		keys = append(keys, k)
		types[k] = ValueInfo{Type: t.DropValue(), Width: t.Width(), Origin: o}
		return nil
	}
	for _, b := range fn.Blocks {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		for _, frame := range []struct {
			f frametransfer.Frame
			o []ssabuild.Origin
		}{{b.In, b.InOrig}, {b.Out, b.OutOrig}} {
			for i, t := range frame.f.Locals {
				if i < len(frame.o) {
					if err := register(frame.o[i], t); err != nil {
						return nil, err
					}
				} else if err := chargeWork(counter, 1); err != nil {
					return nil, err
				}
			}
			for i, t := range frame.f.Stack {
				j := len(frame.f.Locals) + i
				if j < len(frame.o) {
					if err := register(frame.o[j], t); err != nil {
						return nil, err
					}
				} else if err := chargeWork(counter, 1); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, v := range fn.EdgeStates {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		for i, t := range v.Frame.Locals {
			if i < len(v.Origins) {
				if err := register(v.Origins[i], t); err != nil {
					return nil, err
				}
			} else if err := chargeWork(counter, 1); err != nil {
				return nil, err
			}
		}
		for i, t := range v.Frame.Stack {
			j := len(v.Frame.Locals) + i
			if j < len(v.Origins) {
				if err := register(v.Origins[j], t); err != nil {
					return nil, err
				}
			} else if err := chargeWork(counter, 1); err != nil {
				return nil, err
			}
		}
	}
	for _, v := range fn.Instructions {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		for i, t := range v.Before.Locals {
			if i < len(v.BeforeOrigins) {
				if err := register(v.BeforeOrigins[i], t); err != nil {
					return nil, err
				}
			} else if err := chargeWork(counter, 1); err != nil {
				return nil, err
			}
		}
		for i, t := range v.Before.Stack {
			j := len(v.Before.Locals) + i
			if j < len(v.BeforeOrigins) {
				if err := register(v.BeforeOrigins[j], t); err != nil {
					return nil, err
				}
			} else if err := chargeWork(counter, 1); err != nil {
				return nil, err
			}
		}
		for _, o := range v.Uses {
			if err := chargeWork(counter, 1); err != nil {
				return nil, err
			}
			if o.Kind != ssabuild.OriginTop {
				keys = append(keys, OriginKey(o))
			}
		}
		for _, o := range v.Results {
			if err := chargeWork(counter, 1); err != nil {
				return nil, err
			}
			if o.Kind != ssabuild.OriginTop {
				keys = append(keys, OriginKey(o))
			}
		}
	}
	seenPhi := map[ssabuild.ValueID]bool{}
	if err := chargeWork(counter, uint64(len(fn.Blocks))); err != nil {
		return nil, err
	}
	blockByID := make(map[methodir.BlockID]ssabuild.BlockFrame, len(fn.Blocks))
	for _, b := range fn.Blocks {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		if _, found := blockByID[b.ID]; !found {
			blockByID[b.ID] = b
		}
	}
	for _, p := range fn.Phis {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		if p.ID == 0 || seenPhi[p.ID] || !p.Type.Computational() {
			return nil, fmt.Errorf("invalid_input: duplicate/invalid logical phi %d", p.ID)
		}
		seenPhi[p.ID] = true
		b, found := blockByID[p.Block]
		if !found {
			return nil, fmt.Errorf("invalid_input: phi block missing")
		}
		flat := p.Slot.Index
		n := len(b.In.Locals)
		if !p.Slot.Local {
			flat += n
			n = len(b.In.Stack)
		}
		if p.Slot.Index < 0 || p.Slot.Index >= n {
			return nil, fmt.Errorf("invalid_input: phi %d slot out of bounds", p.ID)
		}
		use := OriginKey(ssabuild.Origin{Kind: ssabuild.OriginPhi, PC: b.First, Slot: flat, Aux: int(b.ID)})
		if _, ok := aliases[use]; ok {
			return nil, fmt.Errorf("invalid_input: duplicate phi slot")
		}
		aliases[use] = PhiKey(p)
		keys = append(keys, PhiKey(p))
		types[PhiKey(p)] = ValueInfo{Type: p.Type.DropValue(), Width: p.Type.Width()}
		for _, op := range p.Operands {
			if err := chargeWork(counter, 1); err != nil {
				return nil, err
			}
			if op.Origin.Kind == ssabuild.OriginTop {
				return nil, fmt.Errorf("invalid_input: phi %d has top operand", p.ID)
			}
			k := OriginKey(op.Origin)
			keys = append(keys, k)
			if _, ok := types[k]; !ok {
				types[k] = ValueInfo{Type: p.Type.DropValue(), Width: p.Type.Width(), Origin: op.Origin}
			}
		}
	}
	for _, k := range keys {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		if k.Domain == 0 && k.Kind == int(ssabuild.OriginPhi) {
			if _, ok := aliases[k]; !ok {
				return nil, fmt.Errorf("invalid_input: phi use has no definition %+v", k)
			}
		}
	}
	r, err := newValueRegistry(keys, aliases, counter)
	if err != nil {
		return nil, err
	}
	// Definition metadata wins over aliased incoming/use metadata.
	for _, k := range keys {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		if info, ok := types[k]; ok {
			id, _ := r.Lookup(k)
			r.Info[id] = info
		}
	}
	for _, p := range fn.Phis {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		id, _ := r.Phi(p)
		r.Info[id] = types[PhiKey(p)]
	}
	return r, nil
}

func (r *ValueRegistry) sequentialize(copies []Copy) ([]Move, error) {
	return r.sequentializeWithCounter(copies, nil)
}

func (r *ValueRegistry) sequentializeWithCounter(copies []Copy, counter ssabuild.WorkCounter) ([]Move, error) {
	for _, c := range copies {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		d, dok := r.Info[c.Dst]
		s, sok := r.Info[c.Src]
		if !dok || !sok || d.Width <= 0 || d.Width != s.Width {
			return nil, fmt.Errorf("invalid_input: copy %d <- %d lacks compatible logical widths", c.Dst, c.Src)
		}
	}

	var freshErr error
	moves, err := sequentializeChecked(copies, func() VarID {
		id, e := r.Fresh()
		if e != nil {
			freshErr = e
		}
		return id
	}, counter)
	if freshErr != nil {
		return nil, freshErr
	}
	if err != nil {
		return nil, err
	}
	for _, m := range moves {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		if m.Tmp {
			info, ok := r.Info[m.Src]
			if !ok || info.Width == 0 {
				return nil, fmt.Errorf("invalid_input: temporary source %d lacks logical type", m.Src)
			}
			info.Temporary = true
			info.Source = m.Src
			r.Info[m.Dst] = info
		}
	}
	return moves, nil
}
