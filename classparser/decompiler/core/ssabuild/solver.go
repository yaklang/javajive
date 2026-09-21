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
	ctr := opt.Counter
	if ctr == nil {
		ctr = &LimitCounter{Max: uint64(maxUp)}
	}

	fn := &Function{IR: ir, Blocks: make([]BlockFrame, len(ir.Blocks))}
	index := map[methodir.BlockID]int{}
	for i, b := range ir.Blocks {
		fn.Blocks[i] = BlockFrame{ID: b.ID, First: b.FirstPC}
		index[b.ID] = i
	}
	blockOf := map[uint16]methodir.BlockID{}
	for _, b := range ir.Blocks {
		blockOf[b.FirstPC] = b.ID
	}

	entry, err := initialFrame(ir)
	if err != nil {
		return nil, err
	}
	if len(fn.Blocks) == 0 {
		return fn, nil
	}
	fn.Blocks[0].In = entry
	fn.Blocks[0].InOrig = paramOrigins(entry)

	ef := edgeFrames{normal: map[methodir.EdgeID]frametransfer.Frame{}, orig: map[methodir.EdgeID][]Origin{}}

	pending := []methodir.BlockID{fn.Blocks[0].ID}
	inQ := map[methodir.BlockID]bool{fn.Blocks[0].ID: true}
	rng := rand.New(rand.NewSource(opt.ShuffleSeed))

	pop := func() methodir.BlockID {
		var i int
		if opt.Shuffle && len(pending) > 1 {
			i = rng.Intn(len(pending))
		} else {
			i = 0
		}
		id := pending[i]
		pending = append(pending[:i], pending[i+1:]...)
		inQ[id] = false
		return id
	}

	enqueue := func(id methodir.BlockID) {
		if !inQ[id] {
			inQ[id] = true
			pending = append(pending, id)
		}
	}

	seen := map[methodir.BlockID]bool{}
	for len(pending) > 0 {
		if err := charge(ctr, 1); err != nil {
			return nil, err
		}
		fn.Work++
		bid := pop()
		bi := index[bid]
		b := ir.Blocks[bid]
		preds := fn.Incoming(b.FirstPC)
		joinOrig := func(preds []methodir.Edge) []Origin {
			var joined []Origin
			for _, e := range preds {
				og, ok := ef.orig[e.ID]
				if !ok {
					continue
				}
				if joined == nil {
					joined = append([]Origin(nil), og...)
					continue
				}
				n := len(joined)
				if len(og) < n {
					n = len(og)
				}
				for i := 0; i < n; i++ {
					if joined[i].Key() != og[i].Key() {
						joined[i] = Origin{Kind: OriginPhi, PC: b.FirstPC, Slot: i, Aux: int(bid)}
					}
				}
			}
			return joined
		}
		if bid != fn.Blocks[0].ID {
			var joined *frametransfer.Frame
			for _, e := range preds {
				fr, ok := ef.normal[e.ID]
				if !ok {
					continue
				}
				if joined == nil {
					cp := fr.Clone()
					joined = &cp
					continue
				}
				j, err := frametransfer.JoinFrames(*joined, fr)
				if err != nil {
					return nil, err
				}
				if err := charge(ctr, 1); err != nil {
					return nil, err
				}
				*joined = j
			}
			if joined == nil {
				continue
			}
			og := joinOrig(preds)
			if seen[bid] && fn.Blocks[bi].In.Equal(*joined) && originsEqual(fn.Blocks[bi].InOrig, og) {
				continue
			}
			fn.Blocks[bi].In = *joined
			fn.Blocks[bi].InOrig = og
		} else if len(preds) > 0 {
			joined := fn.Blocks[bi].In.Clone()
			have := false
			for _, e := range preds {
				fr, ok := ef.normal[e.ID]
				if !ok {
					continue
				}
				if !have {
					joined = fr.Clone()
					have = true
					continue
				}
				j, err := frametransfer.JoinFrames(joined, fr)
				if err != nil {
					return nil, err
				}
				if err := charge(ctr, 1); err != nil {
					return nil, err
				}
				joined = j
			}
			og := joinOrig(preds)
			if have && (!fn.Blocks[bi].In.Equal(joined) || !originsEqual(fn.Blocks[bi].InOrig, og)) {
				fn.Blocks[bi].In = joined
				fn.Blocks[bi].InOrig = og
			} else if seen[bid] && have && fn.Blocks[bi].In.Equal(joined) && originsEqual(fn.Blocks[bi].InOrig, og) {
				continue
			}
		} else if seen[bid] {
			continue
		}
		seen[bid] = true
		in := fn.Blocks[bi].In
		cur := in.Clone()
		curOrig := fn.Blocks[bi].InOrig
		if len(curOrig) == 0 {
			curOrig = paramOrigins(in)
		}

		for _, iid := range b.InstrIDs {
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
			afterOrig := transferOrigins(beforeOrig, before, after, ins)
			for _, e := range fn.Outgoing(iid) {
				var fr frametransfer.Frame
				var og []Origin
				if e.Kind == core.EdgeException {
					if ex != nil {
						fr = ex.Clone()
					} else {
						fr = frametransfer.Frame{Locals: append([]frametransfer.Type(nil), before.Locals...), Stack: []frametransfer.Type{frametransfer.RefOf("java/lang/Throwable")}}
					}
					og = originsFromFrame(fr, Origin{Kind: OriginInstr, PC: ins.PC, Slot: -1})
					for i := 0; i < len(fr.Locals) && i < len(beforeOrig); i++ {
						og[i] = beforeOrig[i]
					}
				} else {
					fr = after.Clone()
					og = append([]Origin(nil), afterOrig...)
				}
				ef.normal[e.ID] = fr
				ef.orig[e.ID] = og
				if tb, ok := blockOf[uint16(e.To)]; ok {
					enqueue(tb)
				}
			}
			cur = after
			curOrig = afterOrig
		}
		fn.Blocks[bi].Out = cur
		fn.Blocks[bi].OutOrig = append([]Origin(nil), curOrig...)
	}

	if err := assignPhis(fn, ef); err != nil {
		return nil, err
	}
	return fn, nil
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
	max := 4
	for _, in := range ir.Instrs {
		loc := effectiveLocal(in)
		if loc < 0 {
			continue
		}
		width := core.LocalAccessOf(in.Opcode).Width
		if width < 1 {
			width = 1
		}
		if need := loc + width; need > max {
			max = need
		}
	}
	f := frametransfer.NewFrame(max)
	slot := 0
	if !ir.IsStatic {
		if ir.Name == "<init>" {
			_ = f.StoreLocal(0, frametransfer.T(frametransfer.UninitThis))
		} else {
			cls := ir.ClassName
			if cls == "" {
				cls = "java/lang/Object"
			}
			_ = f.StoreLocal(0, frametransfer.RefOf(cls))
		}
		slot = 1
	}
	args, _, _, _ := frametransfer.ParseDescriptor(ir.Descriptor)
	for _, a := range args {
		_ = f.StoreLocal(slot, a)
		if a.Kind.IsCat2Head() {
			slot += 2
		} else {
			slot++
		}
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
		if t.Kind == frametransfer.Top || t.Kind.IsTail() {
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

func transferOrigins(before []Origin, beforeF, afterF frametransfer.Frame, ins methodir.Instr) []Origin {
	out := originsFromFrame(afterF, Origin{Kind: OriginInstr, PC: ins.PC})
	nl := len(afterF.Locals)
	wrote := -1
	if core.LocalAccessOf(ins.Opcode).Write {
		wrote = effectiveLocal(ins)
	}
	for i := 0; i < nl; i++ {
		if wrote == i {
			out[i] = Origin{Kind: OriginInstr, PC: ins.PC, Slot: i}
			continue
		}
		if i < len(beforeF.Locals) && i < len(before) && afterF.Locals[i].SameLattice(beforeF.Locals[i]) && afterF.Locals[i].Kind != frametransfer.Top {
			out[i] = before[i]
		}
	}
	if len(afterF.Stack) == len(beforeF.Stack) {
		same := true
		for i := range afterF.Stack {
			if !afterF.Stack[i].SameLattice(beforeF.Stack[i]) {
				same = false
				break
			}
		}
		if same {
			for i := range afterF.Stack {
				bi := len(beforeF.Locals) + i
				ai := nl + i
				if bi >= 0 && bi < len(before) && ai < len(out) {
					out[ai] = before[bi]
				}
			}
		}
	}
	return out
}

type edgeFrames struct {
	normal map[methodir.EdgeID]frametransfer.Frame
	orig   map[methodir.EdgeID][]Origin
}

func assignPhis(fn *Function, ef edgeFrames) error {
	type key struct {
		b methodir.BlockID
		s SlotKey
	}
	next := ValueID(1)
	var phis []Phi
	for _, b := range fn.Blocks {
		preds := fn.Incoming(b.First)
		if len(preds) < 2 {
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
				og := ef.orig[e.ID]
				fr := ef.normal[e.ID]
				idx := sk.Index
				if !sk.Local {
					idx = len(fr.Locals) + sk.Index
				}
				var o Origin
				if idx >= 0 && idx < len(og) {
					o = og[idx]
				} else {
					o = Origin{Kind: OriginTop, Slot: idx}
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
