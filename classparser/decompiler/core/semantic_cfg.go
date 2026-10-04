package core

import (
	"fmt"
	"sort"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// EdgeKind records JVM flow, independently of the mutable structuring graph.
type EdgeKind uint8

const (
	EdgeFallthrough EdgeKind = iota
	EdgeTaken
	EdgeCase
	EdgeDefault
	EdgeException
)

type SemanticEdge struct {
	From, To     *OpCode
	Kind         EdgeKind
	CaseValue    int32
	HandlerOrder int
}

// SemanticCFG is a method-local snapshot taken before expression/region rewrites.
// Exception edges leave the throwing instruction's INPUT local state. They are
// never confused with the synthetic try-entry anchors used by the old structurer.
type SemanticCFG struct {
	Nodes      []*OpCode
	Edges      []SemanticEdge
	incoming   map[*OpCode][]int
	outgoing   map[*OpCode][]int
	slots      map[int]*slotDefinitions
	Updates    int
	order      map[*OpCode]int
	Err        error
	MaxUpdates int
	Work       *workbudget.Budget
	GraphScans int
	SetWork    int
	// IndexPeak/IndexEmitted/IndexWork are T24 stream stats (not a linearity proof).
	IndexPeak    int
	IndexEmitted uint64
	IndexWork    uint64
	FunctionType *types.JavaFuncType
	IsStatic     bool
	entryWidth   map[int]int
	// analysis holds the T26 graph-analysis cache. SemanticCFG is immutable after
	// construction; epoch stays 0. Lazy-initialized on first GetOrCompute.
	analysis *graphAnalysisState
}

// MayThrowOpcode conservatively includes resolution/linkage, allocation, array,
// monitor and invocation failures, not only explicit athrow instructions.
func MayThrowOpcode(op int) bool {
	switch op {
	case OP_IDIV, OP_LDIV, OP_IREM, OP_LREM, OP_ATHROW,
		OP_IALOAD, OP_LALOAD, OP_FALOAD, OP_DALOAD, OP_AALOAD, OP_BALOAD, OP_CALOAD, OP_SALOAD,
		OP_IASTORE, OP_LASTORE, OP_FASTORE, OP_DASTORE, OP_AASTORE, OP_BASTORE, OP_CASTORE, OP_SASTORE,
		OP_GETFIELD, OP_PUTFIELD, OP_GETSTATIC, OP_PUTSTATIC,
		OP_INVOKEVIRTUAL, OP_INVOKESPECIAL, OP_INVOKESTATIC, OP_INVOKEINTERFACE, OP_INVOKEDYNAMIC,
		OP_NEW, OP_NEWARRAY, OP_ANEWARRAY, OP_MULTIANEWARRAY, OP_ARRAYLENGTH,
		OP_CHECKCAST, OP_INSTANCEOF, OP_MONITORENTER, OP_MONITOREXIT, OP_LDC, OP_LDC_W, OP_LDC2_W:
		return true
	}
	return false
}

func (d *Decompiler) buildSemanticCFG() (*SemanticCFG, error) {
	if err := d.validateControlFlow(); err != nil {
		return nil, err
	}
	g := &SemanticCFG{order: map[*OpCode]int{}, MaxUpdates: 1000000, incoming: map[*OpCode][]int{}, outgoing: map[*OpCode][]int{}, slots: map[int]*slotDefinitions{}, Work: d.Work, FunctionType: d.FunctionType}
	if d.FunctionContext != nil {
		g.IsStatic = d.FunctionContext.IsStatic
	}
	g.entryWidth = liveInSlotWidths(g.IsStatic, g.FunctionType)
	if d.MaxAnalysisUpdates > 0 {
		g.MaxUpdates = d.MaxAnalysisUpdates
	}
	for _, op := range d.opCodes {
		if op.Instr.OpCode != OP_START && op.Instr.OpCode != OP_END {
			g.order[op] = len(g.Nodes)
			g.Nodes = append(g.Nodes, op)
		}
	}
	target := func(pc int) *OpCode { return d.opCodes[d.offsetToOpcodeIndex[uint16(pc)]] }
	add := func(from, to *OpCode, kind EdgeKind, key int32, order int) error {
		if err := g.Work.Charge(workbudget.CounterGraphEdges, 1); err != nil {
			g.Err = err
			return err
		}
		i := len(g.Edges)
		g.Edges = append(g.Edges, SemanticEdge{from, to, kind, key, order})
		g.incoming[to] = append(g.incoming[to], i)
		g.outgoing[from] = append(g.outgoing[from], i)
		return nil
	}
	for i, op := range g.Nodes {
		if g.Err != nil {
			return g, g.Err
		}
		fall := true
		switch op.Instr.OpCode {
		case OP_RETURN, OP_IRETURN, OP_LRETURN, OP_FRETURN, OP_DRETURN, OP_ARETURN, OP_ATHROW:
			fall = false
		case OP_GOTO, OP_GOTO_W:
			if err := add(op, target(op.BranchTarget), EdgeTaken, 0, 0); err != nil {
				return g, err
			}
			fall = false
		case OP_JSR, OP_JSR_W, OP_RET:
			return nil, fmt.Errorf("semantic CFG requires inlined jsr/ret at PC %d", op.CurrentOffset)
		case OP_LOOKUPSWITCH, OP_TABLESWITCH:
			fall = false
			var switchErr error
			op.SwitchJmpCase.ForEach(func(v int, pc int32) bool {
				switchErr = add(op, target(int(pc)), EdgeCase, int32(v), 0)
				return switchErr == nil
			})
			if switchErr != nil {
				return g, switchErr
			}
			if err := add(op, target(int(op.SwitchDefaultOffset)), EdgeDefault, 0, 0); err != nil {
				return g, err
			}
		case OP_IFEQ, OP_IFNE, OP_IFLT, OP_IFGE, OP_IFGT, OP_IFLE, OP_IF_ICMPEQ, OP_IF_ICMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPLE, OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IFNULL, OP_IFNONNULL:
			if err := add(op, target(op.BranchTarget), EdgeTaken, 0, 0); err != nil {
				return g, err
			}
		}
		if fall && i+1 < len(g.Nodes) {
			if err := add(op, g.Nodes[i+1], EdgeFallthrough, 0, 0); err != nil {
				return g, err
			}
		}
	}
	if err := g.addIndexedExceptionEdges(d, target, add); err != nil {
		return g, err
	}
	if g.Err != nil {
		return g, g.Err
	}
	if err := g.ValidateReducible(); err != nil {
		return nil, err
	}
	return g, nil
}

func liveInSlotWidths(isStatic bool, ft *types.JavaFuncType) map[int]int {
	out := map[int]int{}
	slot := 0
	if !isStatic {
		out[0] = 1
		slot = 1
	}
	if ft == nil {
		return out
	}
	for _, p := range ft.ParamTypes {
		w := 1
		if p != nil {
			w = GetTypeSize(p)
		}
		if w < 1 {
			w = 1
		}
		out[slot] = w
		slot += w
	}
	return out
}

func (g *SemanticCFG) liveInWidth(slot int) int {
	if g == nil || g.entryWidth == nil {
		return 0
	}
	return g.entryWidth[slot]
}

func (g *SemanticCFG) liveInIsCat2Tail(slot int) bool {
	if g == nil || slot <= 0 {
		return false
	}
	return g.liveInWidth(slot-1) == 2
}

func (g *SemanticCFG) addIndexedExceptionEdges(d *Decompiler, target func(int) *OpCode, add func(*OpCode, *OpCode, EdgeKind, int32, int) error) error {
	ranges := ExceptionTableAsRanges(d.ExceptionTable)
	throwOps := make([]*OpCode, 0, 8)
	for _, op := range g.Nodes {
		if op == nil || op.Instr == nil {
			continue
		}
		if MayThrowOpcode(op.Instr.OpCode) {
			throwOps = append(throwOps, op)
		}
	}
	var stats ExceptionIndexStats
	sweep, err := newExceptionSweep(ranges, g.Work, &stats)
	if err != nil {
		g.Err = err
		g.GraphScans += int(stats.IndexWork)
		g.IndexWork = stats.IndexWork
		return err
	}
	g.GraphScans += int(stats.IndexWork)
	var lastPC uint16
	have := false
	for _, op := range throwOps {
		if err := g.Work.Check(); err != nil {
			g.Err = err
			g.recordIndexStats(stats)
			return err
		}
		pc := op.CurrentOffset
		if !have || pc != lastPC {
			before := stats.IndexWork
			if err := sweep.advanceTo(pc, g.Work, &stats); err != nil {
				g.Err = err
				g.GraphScans += int(stats.IndexWork - before)
				g.recordIndexStats(stats)
				return err
			}
			g.GraphScans += int(stats.IndexWork - before)
			lastPC, have = pc, true
		}
		if len(sweep.active) == 0 {
			continue
		}
		buffered := 0
		for _, order := range sweep.active {
			if err := g.Work.Charge(workbudget.CounterGraphScans, 1); err != nil {
				g.Err = err
				g.recordIndexStats(stats)
				return err
			}
			g.GraphScans++
			stats.CandidateScans++
			h := sweep.table[order]
			buffered++
			if buffered > stats.PeakBuffered {
				stats.PeakBuffered = buffered
			}
			// add() charges CounterGraphEdges before appending the retained edge.
			if err := add(op, target(int(h.HandlerPc)), EdgeException, 0, order); err != nil {
				g.recordIndexStats(stats)
				return err
			}
			stats.EmittedEdges++
			if h.CatchType == 0 {
				break
			}
		}
	}
	g.recordIndexStats(stats)
	return nil
}

func (g *SemanticCFG) recordIndexStats(stats ExceptionIndexStats) {
	g.IndexPeak = stats.PeakBuffered
	g.IndexEmitted = stats.EmittedEdges
	g.IndexWork = stats.IndexWork
}

type definitionSet map[*OpCode]struct{} // nil is the stable method-entry definition
// Each slot is solved once on the immutable graph, then every load/store query is O(defs).
// This avoids repeated reverse walks and makes loop joins independent of DFS order.
type slotDefinitions struct {
	in        map[*OpCode]definitionSet
	reachable map[*OpCode]bool
}

func (g *SemanticCFG) solveSlot(slot int) *slotDefinitions {
	if cached := g.slots[slot]; cached != nil {
		return cached
	}
	facts := g.sparseSolveToSlotDefs(slot)
	g.slots[slot] = facts
	return facts
}

func (g *SemanticCFG) sparseSolveToSlotDefs(slot int) *slotDefinitions {
	facts := &slotDefinitions{in: map[*OpCode]definitionSet{}, reachable: map[*OpCode]bool{}}
	if g == nil || len(g.Nodes) == 0 {
		return facts
	}
	s := NewSparseReaching(g)
	if err := s.solve(slot); err != nil {
		if g.Err == nil {
			g.Err = err
		}
		return facts
	}
	arr := s.in[slot]
	reach := s.reach[slot]
	g.SetWork += int(s.merges)
	for i, set := range arr {
		if err := g.Work.Check(); err != nil {
			g.Err = err
			return facts
		}
		n := g.Nodes[i]
		if i >= len(reach) || !reach[i] {
			continue
		}
		if err := g.Work.Charge(workbudget.CounterSetElementWork, int64(len(set.ids))); err != nil {
			g.Err = err
			return facts
		}
		ds := definitionSet{}
		for _, id := range set.ids {
			if id == entryDefID {
				ds[nil] = struct{}{}
				continue
			}
			if int(id) >= 0 && int(id) < len(g.Nodes) {
				ds[g.Nodes[id]] = struct{}{}
			}
		}
		facts.in[n] = ds
		facts.reachable[n] = true
	}
	if len(g.Nodes) > 0 && facts.in[g.Nodes[0]] == nil {
		facts.in[g.Nodes[0]] = definitionSet{nil: {}}
		facts.reachable[g.Nodes[0]] = true
	}
	return facts
}

// ReachingDefinitions returns definition identities in bytecode order, including
// iinc's read/write identity. The separate entry flag is not a fabricated store.
func (g *SemanticCFG) ReachingDefinitions(at *OpCode, slot int) ([]*OpCode, bool) {
	facts := g.solveSlot(slot)
	set := facts.in[at]
	out := make([]*OpCode, 0, len(set))
	_, entry := set[nil]
	for op := range set {
		if op != nil {
			out = append(out, op)
		}
	}
	sort.Slice(out, func(i, j int) bool { return g.order[out[i]] < g.order[out[j]] })
	return out, entry
}

// reachingStores is the adapter used by existing JavaRef/web consumers. iinc
// updates the same source variable; follow its input definitions rather than
// requiring the old simulator to invent a second JavaRef for the increment.
func (d *Decompiler) reachingStores(at *OpCode, slot int) ([]*OpCode, bool) {
	if d.semanticCFG == nil {
		return reachingStoresOf(at, slot)
	}
	g := d.semanticCFG
	pending := []*OpCode{at}
	seen := map[*OpCode]bool{}
	defs := map[*OpCode]bool{}
	entry := false
	for len(pending) > 0 {
		cur := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		stores, e := g.ReachingDefinitions(cur, slot)
		entry = entry || e
		for _, store := range stores {
			if store.Instr.OpCode == OP_IINC {
				pending = append(pending, store)
			} else {
				defs[store] = true
			}
		}
	}
	out := make([]*OpCode, 0, len(defs))
	for op := range defs {
		out = append(out, op)
	}
	sort.Slice(out, func(i, j int) bool { return g.order[out[i]] < g.order[out[j]] })
	return out, entry
}
