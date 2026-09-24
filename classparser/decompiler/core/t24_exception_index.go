package core

import (
	"fmt"
	"sort"

	"github.com/yaklang/javajive/internal/workbudget"
)

// HandlerRange is one exception-table row. The protected interval is half-open
// [StartPc, EndPc). CatchType 0 is catch-all and shadows later rows for a PC.
type HandlerRange struct {
	StartPc, EndPc, HandlerPc, CatchType uint16
}

// IndexedExceptionEdge is the ordered exception successor of one throwing PC.
type IndexedExceptionEdge struct {
	ThrowPc      uint16
	HandlerPc    uint16
	CatchType    uint16
	HandlerOrder int
}

// ExceptionIndexStats records real work, not wall-clock. CandidateScans counts
// active-handler inspections. IndexWork counts event build/sort/apply (including
// rewind). Neither is a proof that dense fan-out is linear in throw sites.
type ExceptionIndexStats struct {
	CandidateScans uint64
	EmittedEdges   uint64
	IndexWork      uint64
	PeakBuffered   int
}

func validHandlerRange(h HandlerRange) bool {
	return h.EndPc > h.StartPc
}

func exceptionEdgesEqual(a, b []IndexedExceptionEdge) bool {
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

func chargeIndexWork(work *workbudget.Budget, n int64) error {
	if n <= 0 {
		return work.Check()
	}
	return work.Charge(workbudget.CounterGraphScans, n)
}

func addIndexWork(stats *ExceptionIndexStats, n int64) {
	if stats == nil || n <= 0 {
		return
	}
	stats.IndexWork += uint64(n)
}

// SlowExceptionEdges is the independent table-scan reference. It walks the
// caller's throwPC slice in given order (unsorted/duplicates allowed) and
// scans the table independently per PC. It does not share the sweep-line.
func SlowExceptionEdges(throwPCs []uint16, table []HandlerRange, work *workbudget.Budget) ([]IndexedExceptionEdge, ExceptionIndexStats, error) {
	out := make([]IndexedExceptionEdge, 0, 8)
	var stats ExceptionIndexStats
	for _, pc := range throwPCs {
		if err := chargeIndexWork(work, 0); err != nil {
			return out, stats, err
		}
		for order, h := range table {
			if err := chargeIndexWork(work, 1); err != nil {
				return out, stats, err
			}
			stats.CandidateScans++
			if !validHandlerRange(h) {
				continue
			}
			if pc < h.StartPc || pc >= h.EndPc {
				continue
			}
			if err := work.Charge(workbudget.CounterGraphEdges, 1); err != nil {
				return out, stats, err
			}
			out = append(out, IndexedExceptionEdge{ThrowPc: pc, HandlerPc: h.HandlerPc, CatchType: h.CatchType, HandlerOrder: order})
			stats.EmittedEdges++
			if len(out) > stats.PeakBuffered {
				stats.PeakBuffered = len(out)
			}
			if h.CatchType == 0 {
				break
			}
		}
	}
	return out, stats, nil
}

type handlerEvent struct {
	pc    uint16
	add   bool
	order int
}

type exceptionSweep struct {
	table    []HandlerRange
	events   []handlerEvent
	active   []int
	inActive []bool
	ei       int
	curPC    uint16
	started  bool
}

func newExceptionSweep(table []HandlerRange, work *workbudget.Budget, stats *ExceptionIndexStats) (*exceptionSweep, error) {
	nvalid := 0
	for _, h := range table {
		if validHandlerRange(h) {
			nvalid++
		}
	}
	// Charge construction of the event array before allocating it.
	build := int64(nvalid) * 2
	if err := chargeIndexWork(work, build); err != nil {
		return nil, err
	}
	addIndexWork(stats, build)
	events := make([]handlerEvent, 0, nvalid*2)
	for order, h := range table {
		if !validHandlerRange(h) {
			continue
		}
		events = append(events,
			handlerEvent{pc: h.StartPc, add: true, order: order},
			handlerEvent{pc: h.EndPc, add: false, order: order},
		)
	}
	sortCost := int64(len(events))
	if err := chargeIndexWork(work, sortCost); err != nil {
		return nil, err
	}
	addIndexWork(stats, sortCost)
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].pc != events[j].pc {
			return events[i].pc < events[j].pc
		}
		if events[i].add != events[j].add {
			return !events[i].add // removals before adds at the same PC
		}
		return events[i].order < events[j].order
	})
	return &exceptionSweep{
		table:    table,
		events:   events,
		active:   make([]int, 0, 8),
		inActive: make([]bool, len(table)),
	}, nil
}

func (s *exceptionSweep) reset(work *workbudget.Budget, stats *ExceptionIndexStats) error {
	n := int64(len(s.inActive))
	if err := chargeIndexWork(work, n); err != nil {
		return err
	}
	addIndexWork(stats, n)
	s.active = s.active[:0]
	for i := range s.inActive {
		s.inActive[i] = false
	}
	s.ei = 0
	s.started = false
	return nil
}

func (s *exceptionSweep) advanceTo(pc uint16, work *workbudget.Budget, stats *ExceptionIndexStats) error {
	if s.started && pc < s.curPC {
		if err := s.reset(work, stats); err != nil {
			return err
		}
	}
	s.started = true
	s.curPC = pc
	for s.ei < len(s.events) && s.events[s.ei].pc <= pc {
		if err := chargeIndexWork(work, 1); err != nil {
			return err
		}
		addIndexWork(stats, 1)
		ev := s.events[s.ei]
		s.ei++
		if ev.add {
			if !s.inActive[ev.order] {
				s.inActive[ev.order] = true
				s.active = insertOrderedInt(s.active, ev.order)
			}
		} else if s.inActive[ev.order] {
			s.inActive[ev.order] = false
			s.active = removeInt(s.active, ev.order)
		}
	}
	return chargeIndexWork(work, 0)
}

// ExceptionEdgeConsumer is invoked once per retained exception edge. The
// consumer must charge CounterGraphEdges before retaining the edge.
type ExceptionEdgeConsumer func(IndexedExceptionEdge) error

// StreamIndexedExceptionEdges walks throwPCs in caller order (unsorted and
// duplicate PCs allowed). Same-PC repeats reuse the active set. It never
// materializes the full O(throw×handler) slice; each retained edge is
// delivered through emit after a graph_scans inspect charge.
func StreamIndexedExceptionEdges(throwPCs []uint16, table []HandlerRange, work *workbudget.Budget, emit ExceptionEdgeConsumer) (ExceptionIndexStats, error) {
	var stats ExceptionIndexStats
	sweep, err := newExceptionSweep(table, work, &stats)
	if err != nil {
		return stats, err
	}
	var lastPC uint16
	have := false
	for _, pc := range throwPCs {
		if err := chargeIndexWork(work, 0); err != nil {
			return stats, err
		}
		if !have || pc != lastPC {
			if err := sweep.advanceTo(pc, work, &stats); err != nil {
				return stats, err
			}
			lastPC, have = pc, true
		}
		if len(sweep.active) == 0 {
			if err := chargeIndexWork(work, 0); err != nil {
				return stats, err
			}
			continue
		}
		buffered := 0
		for _, order := range sweep.active {
			if err := chargeIndexWork(work, 1); err != nil {
				return stats, err
			}
			stats.CandidateScans++
			h := sweep.table[order]
			e := IndexedExceptionEdge{ThrowPc: pc, HandlerPc: h.HandlerPc, CatchType: h.CatchType, HandlerOrder: order}
			buffered++
			if buffered > stats.PeakBuffered {
				stats.PeakBuffered = buffered
			}
			if emit != nil {
				if err := emit(e); err != nil {
					return stats, err
				}
			}
			stats.EmittedEdges++
			if h.CatchType == 0 {
				break
			}
		}
	}
	return stats, nil
}

// IndexedExceptionEdges collects streamed edges in caller throwPC order.
// Each retained edge is charged to CounterGraphEdges before append. PeakBuffered
// is the per-PC in-flight count, not the length of the collected slice.
func IndexedExceptionEdges(throwPCs []uint16, table []HandlerRange, work *workbudget.Budget) ([]IndexedExceptionEdge, ExceptionIndexStats, error) {
	out := make([]IndexedExceptionEdge, 0, 8)
	stats, err := StreamIndexedExceptionEdges(throwPCs, table, work, func(e IndexedExceptionEdge) error {
		if err := work.Charge(workbudget.CounterGraphEdges, 1); err != nil {
			return err
		}
		out = append(out, e)
		return nil
	})
	return out, stats, err
}

func insertOrderedInt(xs []int, v int) []int {
	i := sort.SearchInts(xs, v)
	if i < len(xs) && xs[i] == v {
		return xs
	}
	xs = append(xs, 0)
	copy(xs[i+1:], xs[i:])
	xs[i] = v
	return xs
}

func removeInt(xs []int, v int) []int {
	i := sort.SearchInts(xs, v)
	if i == len(xs) || xs[i] != v {
		return xs
	}
	return append(xs[:i], xs[i+1:]...)
}

// BuildSemanticCFG exports the production constructor for tests.
func (d *Decompiler) BuildSemanticCFG() (*SemanticCFG, error) {
	return d.buildSemanticCFG()
}

// ProductionExceptionEdges extracts ordered exception edges from a SemanticCFG.
func ProductionExceptionEdges(g *SemanticCFG) []IndexedExceptionEdge {
	if g == nil {
		return nil
	}
	out := make([]IndexedExceptionEdge, 0)
	for _, e := range g.Edges {
		if e.Kind != EdgeException || e.From == nil || e.To == nil {
			continue
		}
		out = append(out, IndexedExceptionEdge{
			ThrowPc:      e.From.CurrentOffset,
			HandlerPc:    e.To.CurrentOffset,
			HandlerOrder: e.HandlerOrder,
		})
	}
	return out
}

// ExceptionTableAsRanges copies a decompiler exception table into index rows.
func ExceptionTableAsRanges(table []*ExceptionTableEntry) []HandlerRange {
	out := make([]HandlerRange, 0, len(table))
	for _, h := range table {
		if h == nil {
			continue
		}
		out = append(out, HandlerRange{StartPc: h.StartPc, EndPc: h.EndPc, HandlerPc: h.HandlerPc, CatchType: h.CatchType})
	}
	return out
}

// MayThrowPCs lists production nodes that MayThrowOpcode, in CFG node order.
func MayThrowPCs(g *SemanticCFG) []uint16 {
	if g == nil {
		return nil
	}
	out := make([]uint16, 0)
	for _, n := range g.Nodes {
		if n == nil || n.Instr == nil {
			continue
		}
		if MayThrowOpcode(n.Instr.OpCode) {
			out = append(out, n.CurrentOffset)
		}
	}
	return out
}

func formatEdgeMismatch(slow, fast []IndexedExceptionEdge) string {
	return fmt.Sprintf("slow=%v fast=%v", slow, fast)
}
