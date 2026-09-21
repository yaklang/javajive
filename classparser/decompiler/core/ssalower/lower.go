package ssalower

import (
	"fmt"
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
)

type Range struct {
	Start, End uint16
	Handler    uint16
	CatchType  uint16
	Order      int
}

type EdgeAssign struct {
	Edge     methodir.EdgeID
	From, To methodir.InstrID
	Kind     core.EdgeKind
	Moves    []Move
	Split    bool
	SplitID  int
	Coverage []Range
	OriginPC uint16
}

// SplitBlock is a synthetic identity that exists only on one original edge.
// Phi copies Sequentialize onto it so they do not run on sibling successors
// or in the join block.
type SplitBlock struct {
	ID       int
	Edge     methodir.EdgeID
	From, To methodir.InstrID
	Kind     core.EdgeKind
	Moves    []Move
	Coverage []Range
	OriginPC uint16
}

type Lowered struct {
	IR       *methodir.MethodIR
	SSA      *ssabuild.Function
	Assigns  []EdgeAssign
	Splits   []SplitBlock
	Temps    int
	Coverage map[uint16][]Range
}

func Destroy(fn *ssabuild.Function) (*Lowered, error) {
	if fn == nil || fn.IR == nil {
		return nil, fmt.Errorf("invalid_input: nil SSA")
	}
	ir := fn.IR
	cov := cloneCoverage(coverageMap(ir))
	succN := map[methodir.InstrID]int{}
	predN := map[methodir.InstrID]int{}
	for _, e := range ir.Edges {
		succN[e.From]++
		predN[e.To]++
	}

	nextTmp := VarID(100000)
	fresh := func() VarID {
		nextTmp++
		return nextTmp
	}
	out := &Lowered{IR: ir, SSA: fn, Coverage: cov}
	nextSplit := 0
	for _, e := range ir.Edges {
		originPC := uint16(e.From)
		ea := EdgeAssign{
			Edge:     e.ID,
			From:     e.From,
			To:       e.To,
			Kind:     e.Kind,
			Coverage: rangesForPC(cov, originPC),
			OriginPC: originPC,
		}
		if e.Kind == core.EdgeException {
			// Exception edges keep pre-throw locals; do not move copies onto
			// them or invent a split PC that would leave [start,end).
			out.Assigns = append(out.Assigns, ea)
			continue
		}
		copies := phiCopies(fn, e)
		moves := Sequentialize(copies, fresh)
		critical := succN[e.From] > 1 && predN[e.To] > 1
		if critical && len(moves) > 0 {
			nextSplit++
			split := SplitBlock{
				ID:       nextSplit,
				Edge:     e.ID,
				From:     e.From,
				To:       e.To,
				Kind:     e.Kind,
				Moves:    moves,
				Coverage: append([]Range(nil), ea.Coverage...),
				OriginPC: originPC,
			}
			out.Splits = append(out.Splits, split)
			ea.Split = true
			ea.SplitID = split.ID
			// Moves live on the split identity, not the source (siblings)
			// and not the join (every pred).
			ea.Moves = nil
			out.Assigns = append(out.Assigns, ea)
			out.Assigns = append(out.Assigns, EdgeAssign{
				Edge:     e.ID,
				From:     e.From,
				To:       e.To,
				Kind:     e.Kind,
				Moves:    moves,
				Split:    true,
				SplitID:  split.ID,
				Coverage: append([]Range(nil), ea.Coverage...),
				OriginPC: originPC,
			})
			continue
		}
		ea.Moves = moves
		out.Assigns = append(out.Assigns, ea)
	}
	sort.Slice(out.Assigns, func(i, j int) bool {
		a, b := out.Assigns[i], out.Assigns[j]
		if a.Edge.String() != b.Edge.String() {
			return a.Edge.String() < b.Edge.String()
		}
		if a.SplitID != b.SplitID {
			return a.SplitID < b.SplitID
		}
		if a.Split != b.Split {
			return !a.Split
		}
		return len(a.Moves) < len(b.Moves)
	})
	out.Temps = int(nextTmp - 100000)
	return out, nil
}

func PhiDest(p ssabuild.Phi) VarID { return VarID(p.ID) + 1 }

func OriginVar(o ssabuild.Origin) VarID { return originVar(o) }

func phiCopies(fn *ssabuild.Function, e methodir.Edge) []Copy {
	var copies []Copy
	bl, ok := fn.IR.BlockOf(e.To)
	if !ok {
		return nil
	}
	for _, p := range fn.PhisOf(bl.ID) {
		for _, op := range p.Operands {
			if op.Edge == e.ID {
				copies = append(copies, Copy{Dst: PhiDest(p), Src: originVar(op.Origin)})
			}
		}
	}
	return copies
}

func originVar(o ssabuild.Origin) VarID {
	return VarID(int(o.Kind)*1_000_000 + int(o.PC)*100 + o.Slot)
}

func coverageMap(ir *methodir.MethodIR) map[uint16][]Range {
	out := map[uint16][]Range{}
	if ir == nil {
		return out
	}
	for _, h := range ir.Handlers {
		r := Range{Start: h.StartPC, End: h.EndPC, Handler: h.HandlerPC, CatchType: h.CatchType, Order: h.Order}
		for _, ins := range ir.Instrs {
			if ins.PC >= h.StartPC && ins.PC < h.EndPC && ins.MayThrow {
				out[ins.PC] = append(out[ins.PC], r)
			}
		}
	}
	return out
}

func cloneCoverage(m map[uint16][]Range) map[uint16][]Range {
	out := make(map[uint16][]Range, len(m))
	for pc, rs := range m {
		out[pc] = append([]Range(nil), rs...)
	}
	return out
}

func rangesForPC(cov map[uint16][]Range, pc uint16) []Range {
	rs := cov[pc]
	out := append([]Range(nil), rs...)
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

func CoverageEqual(a, b []Range) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func CoverageMapsEqual(a, b map[uint16][]Range) bool {
	if len(a) != len(b) {
		return false
	}
	for pc, rs := range a {
		if !CoverageEqual(rs, b[pc]) {
			return false
		}
	}
	return true
}

func (l *Lowered) MovesOn(id methodir.EdgeID) []Move {
	if l == nil {
		return nil
	}
	if s, ok := l.SplitFor(id); ok {
		return s.Moves
	}
	for _, a := range l.Assigns {
		if a.Edge == id {
			return a.Moves
		}
	}
	return nil
}

func (l *Lowered) SplitFor(id methodir.EdgeID) (SplitBlock, bool) {
	if l == nil {
		return SplitBlock{}, false
	}
	for _, s := range l.Splits {
		if s.Edge == id {
			return s, true
		}
	}
	return SplitBlock{}, false
}

func (l *Lowered) ThrowsUnmoved() bool {
	if l == nil || l.IR == nil {
		return true
	}
	orig := coverageMap(l.IR)
	if !CoverageMapsEqual(orig, l.Coverage) {
		return false
	}
	for _, a := range l.Assigns {
		if a.Kind == core.EdgeException {
			if !CoverageEqual(a.Coverage, rangesForPC(orig, a.OriginPC)) {
				return false
			}
			if !identityMoves(a.Moves) {
				return false
			}
		}
	}
	for _, s := range l.Splits {
		if s.Kind == core.EdgeException {
			return false
		}
		if !CoverageEqual(s.Coverage, rangesForPC(orig, s.OriginPC)) {
			return false
		}
	}
	return true
}

func identityMoves(moves []Move) bool {
	for _, m := range moves {
		if m.Dst != m.Src {
			return false
		}
	}
	return true
}
