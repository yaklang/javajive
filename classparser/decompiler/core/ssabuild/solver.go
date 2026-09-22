package ssabuild

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
)

type Options struct {
	MaxUpdates  int
	Counter     WorkCounter
	LIFO        bool
	Shuffle     bool
	ShuffleSeed int64
}

func Build(ir *methodir.MethodIR, opt Options) (*Function, error) {
	if ir == nil {
		return nil, fmt.Errorf("invalid_input: nil MethodIR")
	}
	maxUp := opt.MaxUpdates
	if maxUp == 0 {
		maxUp = 1000000
	}
	if maxUp < 0 {
		return nil, fmt.Errorf("invalid_input: negative update budget")
	}
	ctr := opt.Counter
	if ctr == nil {
		ctr = &LimitCounter{Max: uint64(maxUp)}
	}
	fn := &Function{IR: ir, Blocks: make([]BlockFrame, len(ir.Blocks)), EdgeStates: map[methodir.EdgeID]EdgeState{}}
	if len(ir.Blocks) == 0 {
		return fn, nil
	}
	index := map[methodir.BlockID]int{}
	blockOf := map[uint16]methodir.BlockID{}
	for i, b := range ir.Blocks {
		fn.Blocks[i] = BlockFrame{ID: b.ID, First: b.FirstPC}
		index[b.ID] = i
		blockOf[b.FirstPC] = b.ID
	}
	entryID, ok := blockOf[ir.EntryPC]
	if !ok {
		return nil, fmt.Errorf("invalid_input: missing entry block")
	}
	entry, err := initialFrame(ir)
	if err != nil {
		return nil, err
	}
	// A distinct edge kind retains method entry independently of all backedges.
	entryEdge := methodir.Edge{ID: methodir.EdgeID{From: methodir.InstrID(ir.EntryPC), To: methodir.InstrID(ir.EntryPC), Kind: EntryEdgeKind}, From: methodir.InstrID(ir.EntryPC), To: methodir.InstrID(ir.EntryPC), Kind: EntryEdgeKind}
	fn.entryEdge = &entryEdge
	incoming := map[methodir.BlockID][]methodir.Edge{entryID: {entryEdge}}
	outgoing := map[methodir.InstrID][]methodir.Edge{}
	for _, e := range ir.Edges {
		tb, ok := blockOf[uint16(e.To)]
		if !ok {
			return nil, fmt.Errorf("invalid_input: edge target is not a block entry")
		}
		incoming[tb] = append(incoming[tb], e)
		outgoing[e.From] = append(outgoing[e.From], e)
	}
	for id := range incoming {
		sort.Slice(incoming[id], func(i, j int) bool { return incoming[id][i].ID.String() < incoming[id][j].ID.String() })
	}
	fn.incoming = map[uint16][]methodir.Edge{}
	for bid, edges := range incoming {
		fn.incoming[ir.Blocks[index[bid]].FirstPC] = edges
	}
	ef := edgeFrames{normal: map[methodir.EdgeID]frametransfer.Frame{entryEdge.ID: entry}, orig: map[methodir.EdgeID][]Origin{entryEdge.ID: paramOrigins(entry)}}
	pending := []methodir.BlockID{entryID}
	inQ := map[methodir.BlockID]bool{entryID: true}
	rng := rand.New(rand.NewSource(opt.ShuffleSeed))
	records := map[uint16]InstructionValues{}
	for len(pending) > 0 {
		if err := charge(ctr, 1); err != nil {
			return nil, err
		}
		fn.Work++
		qi := 0
		if opt.LIFO {
			qi = len(pending) - 1
		} else if opt.Shuffle && len(pending) > 1 {
			qi = rng.Intn(len(pending))
		}
		bid := pending[qi]
		pending = append(pending[:qi], pending[qi+1:]...)
		inQ[bid] = false
		bi := index[bid]
		b := ir.Blocks[bi]
		var joined *frametransfer.Frame
		var origins []Origin
		for _, e := range incoming[bid] {
			fr, available := ef.normal[e.ID]
			if !available {
				continue
			} // absence is unreachable, never TOP
			og := ef.orig[e.ID]
			if err := charge(ctr, uint64(1+len(og))); err != nil {
				return nil, err
			}
			if joined == nil {
				cp := fr.Clone()
				joined = &cp
				origins = append([]Origin(nil), og...)
				continue
			}
			j, err := frametransfer.JoinFrames(*joined, fr)
			if err != nil {
				return nil, err
			}
			if len(origins) != len(og) {
				return nil, fmt.Errorf("invalid_input: origin join shape mismatch")
			}
			for i := range origins {
				if origins[i] != og[i] {
					origins[i] = Origin{Kind: OriginPhi, PC: b.FirstPC, Slot: i, Aux: int(bid)}
				}
			}
			*joined = j
		}
		if joined == nil {
			continue
		}
		// TOP is a reachable unavailable local, and a wide pair is one SSA value.
		normalizeOrigins(*joined, origins)
		bf := &fn.Blocks[bi]
		if bf.Reachable && bf.In.Equal(*joined) && originsEqual(bf.InOrig, origins) {
			continue
		}
		bf.Reachable = true
		bf.In = *joined
		bf.InOrig = origins
		cur := joined.Clone()
		curOrig := append([]Origin(nil), origins...)
		for _, iid := range b.InstrIDs {
			if err := charge(ctr, uint64(1+len(curOrig))); err != nil {
				return nil, err
			}
			ins, ok := ir.InstrByID(iid)
			if !ok {
				return nil, fmt.Errorf("invalid_input: missing instr %d", iid)
			}
			before := cur.Clone()
			beforeOrig := append([]Origin(nil), curOrig...)
			ft := frametransfer.FromIR(ins)
			if loc := effectiveLocal(ins); loc >= 0 {
				ft.Local = loc
			}
			after, ex, err := frametransfer.Transfer(cur, ft)
			if err != nil {
				return nil, err
			}
			afterOrig, record, err := transferOrigins(beforeOrig, before, after, ins)
			if err != nil {
				return nil, err
			}
			record.Before = before
			record.BeforeOrigins = beforeOrig
			records[ins.PC] = record
			for _, e := range outgoing[iid] {
				if err := charge(ctr, 1); err != nil {
					return nil, err
				}
				fr := after.Clone()
				og := append([]Origin(nil), afterOrig...)
				if e.Kind == core.EdgeException {
					if ex == nil {
						return nil, fmt.Errorf("invalid_input: exception edge from nonthrowing pc %d", ins.PC)
					}
					fr = ex.Clone()
					if err := fr.Validate(); err != nil {
						return nil, err
					}
					og = originsFromFrame(fr, Origin{Kind: OriginInstr, PC: ins.PC, Slot: -1})
					copy(og[:len(fr.Locals)], beforeOrig[:len(before.Locals)])
					normalizeOrigins(fr, og) // constructor exceptions may invalidate aliases
				}
				old, exists := ef.normal[e.ID]
				if exists && old.Equal(fr) && originsEqual(ef.orig[e.ID], og) {
					continue
				}
				ef.normal[e.ID] = fr
				ef.orig[e.ID] = og
				tb := blockOf[uint16(e.To)]
				if !inQ[tb] {
					inQ[tb] = true
					pending = append(pending, tb)
				}
			}
			cur = after
			curOrig = afterOrig
		}
		bf.Out = cur
		bf.OutOrig = append([]Origin(nil), curOrig...)
	}
	for _, ins := range ir.Instrs {
		if r, ok := records[ins.PC]; ok {
			fn.Instructions = append(fn.Instructions, r)
		}
	}
	for id, fr := range ef.normal {
		fn.EdgeStates[id] = EdgeState{Frame: fr.Clone(), Origins: append([]Origin(nil), ef.orig[id]...)}
	}
	if err := assignPhis(fn, ef, ctr); err != nil {
		return nil, err
	}
	if err := bindValues(fn, ctr); err != nil {
		return nil, err
	}
	return fn, nil
}

func normalizeOrigins(f frametransfer.Frame, origins []Origin) {
	for _, part := range []struct {
		types  []frametransfer.Type
		offset int
	}{{f.Locals, 0}, {f.Stack, len(f.Locals)}} {
		for i, t := range part.types {
			at := part.offset + i
			if t.Kind == frametransfer.Top {
				origins[at] = Origin{Kind: OriginTop, Slot: at}
			} else if t.Kind.IsTail() && i > 0 {
				origins[at] = origins[at-1]
			}
		}
	}
}

func effectiveLocal(ins methodir.Instr) int {
	if ins.Local >= 0 {
		return ins.Local
	}
	acc := core.LocalAccessOf(ins.Opcode)
	if acc.Slot >= 0 {
		return acc.Slot
	}
	if !acc.Read && !acc.Write {
		return -1
	}
	if ins.Wide && len(ins.Data) >= 2 {
		return int(binary.BigEndian.Uint16(ins.Data))
	}
	if len(ins.Data) > 0 {
		return int(ins.Data[0])
	}
	return -1
}

func initialFrame(ir *methodir.MethodIR) (frametransfer.Frame, error) {
	args, _, _, err := frametransfer.ParseDescriptor(ir.Descriptor)
	if err != nil {
		return frametransfer.Frame{}, err
	}
	slots := 0
	if !ir.IsStatic {
		slots = 1
	}
	for _, a := range args {
		slots += a.Width()
	}
	max := slots
	for _, in := range ir.Instrs {
		loc := effectiveLocal(in)
		if loc < 0 {
			continue
		}
		width := core.LocalAccessOf(in.Opcode).Width
		if width < 1 {
			width = 1
		}
		if loc+width > max {
			max = loc + width
		}
	}
	if max > 65535 {
		return frametransfer.Frame{}, fmt.Errorf("invalid_input: method locals exceed JVM limit")
	}
	f := frametransfer.NewFrame(max)
	f.ThisClass = ir.ClassName
	slot := 0
	if !ir.IsStatic {
		typ := frametransfer.RefOf(ir.ClassName)
		if ir.Name == "<init>" {
			typ = frametransfer.T(frametransfer.UninitThis)
		}
		if err := f.StoreLocal(0, typ); err != nil {
			return frametransfer.Frame{}, err
		}
		slot = 1
	}
	for _, a := range args {
		if err := f.StoreLocal(slot, a); err != nil {
			return frametransfer.Frame{}, err
		}
		slot += a.Width()
	}
	return f, nil
}

func paramOrigins(f frametransfer.Frame) []Origin {
	n := len(f.Locals) + len(f.Stack)
	out := make([]Origin, n)
	for i := 0; i < n; i++ {
		var t frametransfer.Type
		if i < len(f.Locals) {
			t = f.Locals[i]
		} else {
			t = f.Stack[i-len(f.Locals)]
		}
		if t.Kind.IsTail() && i > 0 {
			out[i] = out[i-1]
		} else if t.Kind == frametransfer.Top {
			out[i] = Origin{Kind: OriginTop, Slot: i}
		} else {
			out[i] = Origin{Kind: OriginParam, Slot: i}
		}
	}
	return out
}

func originsEqual(a, b []Origin) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key() != b[i].Key() {
			return false
		}
	}
	return true
}

func originsFromFrame(f frametransfer.Frame, def Origin) []Origin {
	n := len(f.Locals) + len(f.Stack)
	out := make([]Origin, n)
	for i := 0; i < n; i++ {
		out[i] = def
		var t frametransfer.Type
		if i < len(f.Locals) {
			t = f.Locals[i]
		} else {
			t = f.Stack[i-len(f.Locals)]
		}
		if t.Kind == frametransfer.Top {
			out[i] = Origin{Kind: OriginTop, Slot: i}
		} else if def.Kind == OriginTop && i < len(f.Locals) && t.Kind != frametransfer.Top {
			out[i] = Origin{Kind: OriginParam, Slot: i}
		}
	}
	return out
}

type edgeFrames struct {
	normal map[methodir.EdgeID]frametransfer.Frame
	orig   map[methodir.EdgeID][]Origin
}

func assignPhis(fn *Function, ef edgeFrames, ctr WorkCounter) error {
	type key struct {
		b methodir.BlockID
		s SlotKey
	}
	next := ValueID(1)
	var phis []Phi
	for _, b := range fn.Blocks {
		if err := charge(ctr, uint64(1+len(b.InOrig))); err != nil {
			return err
		}
		preds := fn.Incoming(b.First)
		if !b.Reachable || len(preds) < 2 {
			continue
		}
		nloc := len(b.In.Locals)
		nstack := len(b.In.Stack)
		for i := 0; i < nloc+nstack; i++ {
			sk := SlotKey{Local: i < nloc, Index: i}
			if i >= nloc {
				sk.Index = i - nloc
			}
			var t frametransfer.Type
			if sk.Local {
				t = b.In.Locals[sk.Index]
			} else {
				t = b.In.Stack[sk.Index]
			}
			if t.Kind == frametransfer.Top || t.Kind.IsTail() {
				continue
			}
			var ops []PhiOperand
			seen := map[string]Origin{}
			diff := false
			var first Origin
			for _, e := range preds {
				if err := charge(ctr, 1); err != nil {
					return err
				}
				og, available := ef.orig[e.ID]
				if !available {
					continue
				}
				fr := ef.normal[e.ID]
				idx := sk.Index
				if !sk.Local {
					idx = len(fr.Locals) + sk.Index
				}
				var o Origin
				if idx >= 0 && idx < len(og) {
					o = og[idx]
				} else {
					return fmt.Errorf("invalid_input: missing reachable phi origin at %d", idx)
				}
				ops = append(ops, PhiOperand{Edge: e.ID, Origin: o})
				if len(seen) == 0 {
					first = o
				} else if first.Key() != o.Key() {
					diff = true
				}
				seen[o.Key()] = o
			}
			if !diff {
				continue
			}
			sort.Slice(ops, func(i, j int) bool { return ops[i].Edge.String() < ops[j].Edge.String() })
			p := Phi{ID: next, Block: b.ID, Slot: sk, Type: t.DropValue(), Operands: ops}
			next++
			phis = append(phis, p)
		}
	}
	sort.Slice(phis, func(i, j int) bool {
		if phis[i].Block != phis[j].Block {
			return phis[i].Block < phis[j].Block
		}
		if phis[i].Slot.Local != phis[j].Slot.Local {
			return phis[i].Slot.Local
		}
		return phis[i].Slot.Index < phis[j].Slot.Index
	})
	for i := range phis {
		phis[i].ID = ValueID(i + 1)
	}
	fn.Phis = phis
	return nil
}
