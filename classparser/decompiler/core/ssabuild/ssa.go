package ssabuild

import (
	"fmt"
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
)

type ValueID uint32

type SlotKey struct {
	Local bool
	Index int
}

func (s SlotKey) String() string {
	if s.Local {
		return fmt.Sprintf("l%d", s.Index)
	}
	return fmt.Sprintf("s%d", s.Index)
}

type OriginKind uint8

const (
	OriginTop OriginKind = iota
	OriginParam
	OriginInstr
	OriginPhi
	OriginConst
)

type Origin struct {
	Kind OriginKind
	PC   uint16
	Slot int
	Aux  int
}

func (o Origin) Key() string {
	return fmt.Sprintf("%d:%d:%d:%d", o.Kind, o.PC, o.Slot, o.Aux)
}

type PhiOperand struct {
	Edge   methodir.EdgeID
	Origin Origin
	Val    ValueID
}

type Phi struct {
	ID       ValueID
	Block    methodir.BlockID
	Slot     SlotKey
	Type     frametransfer.Type
	Operands []PhiOperand
}

type BlockFrame struct {
	Reachable bool
	ID        methodir.BlockID
	First     uint16
	In        frametransfer.Frame
	Out       frametransfer.Frame
	InOrig    []Origin
	OutOrig   []Origin
}

// EntryEdgeKind is reserved for the permanent synthetic method-entry edge.
const EntryEdgeKind core.EdgeKind = 255

type EdgeState struct {
	Frame   frametransfer.Frame
	Origins []Origin
}

type ValueDefinition struct {
	ID     ValueID
	Origin Origin
}

type Function struct {
	Values       []ValueDefinition
	EdgeStates   map[methodir.EdgeID]EdgeState
	Instructions []InstructionValues
	entryEdge    *methodir.Edge
	incoming     map[uint16][]methodir.Edge
	IR           *methodir.MethodIR
	Blocks       []BlockFrame
	Phis         []Phi
	Params       []ValueID
	Work         uint64
}

func (f *Function) PhisOf(b methodir.BlockID) []Phi {
	var out []Phi
	for _, p := range f.Phis {
		if p.Block == b {
			out = append(out, p)
		}
	}
	return out
}

// PhisOfWithCounter filters and sorts block phis under the supplied work
// counter. Each source phi is charged before it can be copied into the result.
func (f *Function) PhisOfWithCounter(b methodir.BlockID, counter WorkCounter) ([]Phi, error) {
	if f == nil {
		return nil, nil
	}
	var out []Phi
	for _, p := range f.Phis {
		if err := charge(counter, 1); err != nil {
			return nil, err
		}
		if p.Block == b {
			out = append(out, p)
		}
	}
	if err := charge(counter, sortWorkEstimate(len(out))); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *Function) Normalize() string {
	if f == nil {
		return ""
	}
	phis := append([]Phi(nil), f.Phis...)
	sort.Slice(phis, func(i, j int) bool {
		if phis[i].Block != phis[j].Block {
			return phis[i].Block < phis[j].Block
		}
		if phis[i].Slot.Local != phis[j].Slot.Local {
			return phis[i].Slot.Local
		}
		return phis[i].Slot.Index < phis[j].Slot.Index
	})
	s := fmt.Sprintf("blocks=%d\n", len(f.Blocks))
	for _, b := range f.Blocks {
		s += fmt.Sprintf("block %d pc=%d in=%s out=%s\n", b.ID, b.First, b.In.Canonical(), b.Out.Canonical())
	}
	for _, p := range phis {
		s += fmt.Sprintf("phi %d b%d %s %s [", p.ID, p.Block, p.Slot, p.Type)
		ops := append([]PhiOperand(nil), p.Operands...)
		sort.Slice(ops, func(i, j int) bool { return ops[i].Edge.String() < ops[j].Edge.String() })
		for i, o := range ops {
			if i > 0 {
				s += ","
			}
			s += o.Edge.String() + "=" + o.Origin.Key()
		}
		s += "]\n"
	}
	return s
}

func (f *Function) Incoming(blockPC uint16) []methodir.Edge {
	if f == nil || f.IR == nil {
		return nil
	}
	if f.incoming != nil {
		return append([]methodir.Edge(nil), f.incoming[blockPC]...)
	}
	var out []methodir.Edge
	if f.entryEdge != nil && uint16(f.entryEdge.To) == blockPC {
		out = append(out, *f.entryEdge)
	}
	for _, e := range f.IR.Edges {
		if uint16(e.To) == blockPC {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out
}

// IncomingWithCounter returns a deterministic copy of a block's incoming
// edges, charging scan/copy/sort work before performing it.
func (f *Function) IncomingWithCounter(blockPC uint16, counter WorkCounter) ([]methodir.Edge, error) {
	if f == nil || f.IR == nil {
		return nil, nil
	}
	if f.incoming != nil {
		edges := f.incoming[blockPC]
		if err := charge(counter, uint64(len(edges))); err != nil {
			return nil, err
		}
		return append([]methodir.Edge(nil), edges...), nil
	}
	var entryCount uint64
	if f.entryEdge != nil && uint16(f.entryEdge.To) == blockPC {
		entryCount = 1
	}
	if err := charge(counter, uint64(len(f.IR.Edges))); err != nil {
		return nil, err
	}
	count := entryCount
	for _, e := range f.IR.Edges {
		if uint16(e.To) == blockPC {
			if count == ^uint64(0) {
				return nil, fmt.Errorf("analysis_budget_exceeded: incoming edge count overflow")
			}
			count++
		}
	}
	if count > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("analysis_budget_exceeded: incoming edge count overflow")
	}
	if err := charge(counter, count); err != nil {
		return nil, err
	}
	out := make([]methodir.Edge, 0, count)
	if entryCount != 0 {
		out = append(out, *f.entryEdge)
	}
	for _, e := range f.IR.Edges {
		if uint16(e.To) == blockPC {
			out = append(out, e)
		}
	}
	if err := charge(counter, sortWorkEstimate(len(out))); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out, nil
}

func (f *Function) Outgoing(from methodir.InstrID) []methodir.Edge {
	var out []methodir.Edge
	for _, e := range f.IR.Edges {
		if e.From == from {
			out = append(out, e)
		}
	}
	return out
}

func mayThrow(op int) bool { return core.MayThrowOpcode(op) }
