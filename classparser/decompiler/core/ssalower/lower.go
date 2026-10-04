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
	// EntryMoves initialize entry-block phis from the synthetic method-entry edge.
	EntryMoves []Move
	Values     *ValueRegistry
	Exception  ExceptionPlan
	IR         *methodir.MethodIR
	SSA        *ssabuild.Function
	Assigns    []EdgeAssign
	Splits     []SplitBlock
	Temps      int
	Coverage   map[uint16][]Range
}

func Destroy(fn *ssabuild.Function) (*Lowered, error) {
	return DestroyWithOptions(fn, Options{MaxSpills: 100000})
}

// DestroyWithOptions returns a complete emission plan or nil and an error.
// Consumers must emit Exception.Sites before throwing instructions and bind
// Exception.Catch from catch parameters in addition to normal edge moves.
func DestroyWithOptions(fn *ssabuild.Function, opt Options) (*Lowered, error) {
	return DestroyWithWorkCounter(fn, opt, nil)
}

// DestroyWithWorkCounter is the request-budgeted lowering entry point. The
// legacy Options shape stays stable for callers that do not need accounting.
func DestroyWithWorkCounter(fn *ssabuild.Function, opt Options, counter ssabuild.WorkCounter) (*Lowered, error) {
	if fn == nil || fn.IR == nil {
		return nil, fmt.Errorf("invalid_input: nil SSA")
	}
	ir := fn.IR
	cov, err := coverageMapWithCounter(ir, counter)
	if err != nil {
		return nil, err
	}
	succN := map[methodir.InstrID]int{}
	predN := map[methodir.InstrID]int{}
	for _, e := range ir.Edges {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		if _, ok := fn.EdgeStates[e.ID]; !ok {
			continue
		}
		succN[e.From]++
		predN[e.To]++
	}

	values, err := registryForWithCounter(fn, counter)
	if err != nil {
		return nil, err
	}
	out := &Lowered{IR: ir, SSA: fn, Coverage: cov, Values: values}
	plan, err := exceptionPlan(fn, values, cov, opt.MaxSpills, counter)
	if err != nil {
		return nil, err
	}
	out.Exception = plan
	entryIncoming, err := fn.IncomingWithCounter(ir.EntryPC, counter)
	if err != nil {
		return nil, err
	}
	for _, e := range entryIncoming {
		if e.Kind != ssabuild.EntryEdgeKind {
			continue
		}
		copies, err := phiCopiesCheckedWithCounter(fn, values, e, counter)
		if err != nil {
			return nil, err
		}
		moves, err := values.sequentializeWithCounter(copies, counter)
		if err != nil {
			return nil, err
		}
		out.EntryMoves = moves
	}
	nextSplit := 0
	for _, e := range ir.Edges {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		if _, ok := fn.EdgeStates[e.ID]; !ok {
			continue
		}
		originPC := uint16(e.From)
		coverage, err := rangesForPCWithCounter(cov, originPC, counter)
		if err != nil {
			return nil, err
		}
		ea := EdgeAssign{
			Edge:     e.ID,
			From:     e.From,
			To:       e.To,
			Kind:     e.Kind,
			Coverage: coverage,
			OriginPC: originPC,
		}
		if e.Kind == core.EdgeException {
			// Exception transfers are represented by out.Exception: spills
			// before the throw and catch-parameter bindings at the handler.
			if err := chargeWork(counter, 1); err != nil {
				return nil, err
			}
			out.Assigns = append(out.Assigns, ea)
			continue
		}
		copies, err := phiCopiesCheckedWithCounter(fn, values, e, counter)
		if err != nil {
			return nil, err
		}
		moves, err := values.sequentializeWithCounter(copies, counter)
		if err != nil {
			return nil, err
		}
		critical := succN[e.From] > 1 && predN[e.To] > 1
		if critical && len(moves) > 0 {
			if err := chargeWork(counter, 2); err != nil {
				return nil, err
			}
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
			if err := chargeWork(counter, 1); err != nil {
				return nil, err
			}
			out.Assigns = append(out.Assigns, ea)

			continue
		}
		ea.Moves = moves
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		out.Assigns = append(out.Assigns, ea)
	}
	if err := chargeWork(counter, sortWork(len(out.Assigns))); err != nil {
		return nil, err
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
	out.Temps = values.temps
	return out, nil
}

// phiCopiesChecked resolves both ends in the same method registry.
func phiCopiesChecked(fn *ssabuild.Function, values *ValueRegistry, e methodir.Edge) ([]Copy, error) {
	return phiCopiesCheckedWithCounter(fn, values, e, nil)
}

func phiCopiesCheckedWithCounter(fn *ssabuild.Function, values *ValueRegistry, e methodir.Edge, counter ssabuild.WorkCounter) ([]Copy, error) {
	var copies []Copy
	bl, ok := fn.IR.BlockOf(e.To)
	if !ok {
		return nil, fmt.Errorf("invalid_input: edge target missing")
	}
	phis, err := fn.PhisOfWithCounter(bl.ID, counter)
	if err != nil {
		return nil, err
	}
	for _, p := range phis {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		var found bool
		for _, op := range p.Operands {
			if err := chargeWork(counter, 1); err != nil {
				return nil, err
			}
			if op.Edge == e.ID {
				if found {
					return nil, fmt.Errorf("invalid_input: duplicate phi operand")
				}
				found = true
				dst, err := values.Phi(p)
				if err != nil {
					return nil, err
				}
				src, err := values.Origin(op.Origin)
				if err != nil {
					return nil, err
				}
				copies = append(copies, Copy{Dst: dst, Src: src})
			}
		}
		if !found {
			return nil, fmt.Errorf("invalid_input: phi %d missing edge %s", p.ID, e.ID)
		}
	}
	if err := chargeWork(counter, sortWork(len(copies))); err != nil {
		return nil, err
	}
	sort.Slice(copies, func(i, j int) bool { return copies[i].Dst < copies[j].Dst })
	return copies, nil
}

func coverageMap(ir *methodir.MethodIR) map[uint16][]Range {
	out, _ := coverageMapWithCounter(ir, nil)
	return out
}

func coverageMapWithCounter(ir *methodir.MethodIR, counter ssabuild.WorkCounter) (map[uint16][]Range, error) {
	out := map[uint16][]Range{}
	if ir == nil {
		return out, nil
	}
	for _, h := range ir.Handlers {
		if err := chargeWork(counter, 1); err != nil {
			return nil, err
		}
		r := Range{Start: h.StartPC, End: h.EndPC, Handler: h.HandlerPC, CatchType: h.CatchType, Order: h.Order}
		for _, ins := range ir.Instrs {
			if err := chargeWork(counter, 1); err != nil {
				return nil, err
			}
			if ins.PC >= h.StartPC && ins.PC < h.EndPC && ins.MayThrow {
				if err := chargeWork(counter, 1); err != nil {
					return nil, err
				}
				out[ins.PC] = append(out[ins.PC], r)
			}
		}
	}
	return out, nil
}

func cloneCoverage(m map[uint16][]Range) map[uint16][]Range {
	out := make(map[uint16][]Range, len(m))
	for pc, rs := range m {
		out[pc] = append([]Range(nil), rs...)
	}
	return out
}

func rangesForPC(cov map[uint16][]Range, pc uint16) []Range {
	out, _ := rangesForPCWithCounter(cov, pc, nil)
	return out
}

func rangesForPCWithCounter(cov map[uint16][]Range, pc uint16, counter ssabuild.WorkCounter) ([]Range, error) {
	rs := cov[pc]
	if err := chargeWork(counter, uint64(len(rs))+sortWork(len(rs))); err != nil {
		return nil, err
	}
	out := append([]Range(nil), rs...)
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out, nil
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
	ok, _ := l.ThrowsUnmovedWithCounter(nil)
	return ok
}

// ThrowsUnmovedWithCounter checks the exception-coverage invariant while
// charging the validation traversal to the same request budget as lowering.
func (l *Lowered) ThrowsUnmovedWithCounter(counter ssabuild.WorkCounter) (bool, error) {
	if l == nil || l.IR == nil {
		return true, nil
	}
	orig, err := coverageMapWithCounter(l.IR, counter)
	if err != nil {
		return false, err
	}
	equal, err := coverageMapsEqualWithCounter(orig, l.Coverage, counter)
	if err != nil {
		return false, err
	}
	if !equal {
		return false, nil
	}
	for _, a := range l.Assigns {
		if err := chargeWork(counter, 1); err != nil {
			return false, err
		}
		if a.Kind == core.EdgeException {
			expected, err := rangesForPCWithCounter(orig, a.OriginPC, counter)
			if err != nil {
				return false, err
			}
			equal, err := coverageEqualWithCounter(a.Coverage, expected, counter)
			if err != nil {
				return false, err
			}
			if !equal {
				return false, nil
			}
			if err := chargeWork(counter, uint64(len(a.Moves))); err != nil {
				return false, err
			}
			if !identityMoves(a.Moves) {
				return false, nil
			}
		}
	}
	for _, s := range l.Splits {
		if err := chargeWork(counter, 1); err != nil {
			return false, err
		}
		if s.Kind == core.EdgeException {
			return false, nil
		}
		expected, err := rangesForPCWithCounter(orig, s.OriginPC, counter)
		if err != nil {
			return false, err
		}
		equal, err := coverageEqualWithCounter(s.Coverage, expected, counter)
		if err != nil {
			return false, err
		}
		if !equal {
			return false, nil
		}
	}
	return true, nil
}

func coverageMapsEqualWithCounter(a, b map[uint16][]Range, counter ssabuild.WorkCounter) (bool, error) {
	if len(a) != len(b) {
		return false, nil
	}
	for pc, ranges := range a {
		if err := chargeWork(counter, uint64(len(ranges))+1); err != nil {
			return false, err
		}
		if !CoverageEqual(ranges, b[pc]) {
			return false, nil
		}
	}
	return true, nil
}

func coverageEqualWithCounter(a, b []Range, counter ssabuild.WorkCounter) (bool, error) {
	if err := chargeWork(counter, uint64(len(a))); err != nil {
		return false, err
	}
	return CoverageEqual(a, b), nil
}

func identityMoves(moves []Move) bool {
	for _, m := range moves {
		if m.Dst != m.Src {
			return false
		}
	}
	return true
}
