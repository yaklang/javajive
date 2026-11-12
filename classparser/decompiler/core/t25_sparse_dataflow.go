package core

import (
	"fmt"
	"math"

	"github.com/yaklang/javajive/internal/workbudget"
)

// reachingDefID is a stable definition identity: -1 is the method-entry/nil
// pseudo-definition; non-negative values are SemanticCFG node indices.
type reachingDefID int

const entryDefID reachingDefID = -1

type defSet struct {
	ids []reachingDefID // sorted, unique
}

func addInt64(a, b int64) int64 {
	if a < 0 {
		a = 0
	}
	if b < 0 {
		b = 0
	}
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

func chargeSetWork(work *workbudget.Budget, n int64) error {
	if n <= 0 {
		return work.Check()
	}
	return work.Charge(workbudget.CounterSetElementWork, n)
}

func unionDef(a, b defSet, work *workbudget.Budget, merges *uint64) (defSet, bool, error) {
	if err := chargeSetWork(work, 0); err != nil {
		return defSet{}, false, err
	}
	if len(b.ids) == 0 {
		return a, false, nil
	}
	if len(a.ids) == 0 {
		// First assignment shares the incoming set; callers must not mutate it in place.
		return b, true, nil
	}
	need := addInt64(int64(len(a.ids)), int64(len(b.ids)))
	if err := chargeSetWork(work, need); err != nil {
		return defSet{}, false, err
	}
	// Determine whether the union grows before allocating. Reaching joins
	// usually re-propagate an already-covered set to a loop predecessor; the
	// temporary merged slice used to be allocated and immediately discarded.
	i, j := 0, 0
	changed := false
	unionSize := 0
	for i < len(a.ids) || j < len(b.ids) {
		switch {
		case i == len(a.ids):
			j++
			changed = true
		case j == len(b.ids):
			i++
		case a.ids[i] == b.ids[j]:
			i++
			j++
		case a.ids[i] < b.ids[j]:
			i++
		default:
			j++
			changed = true
		}
		unionSize++
	}
	if merges != nil {
		*merges += uint64(unionSize)
	}
	if !changed {
		return a, false, nil
	}
	out := make([]reachingDefID, 0, unionSize)
	i, j = 0, 0
	for i < len(a.ids) || j < len(b.ids) {
		var pick reachingDefID
		switch {
		case i == len(a.ids):
			pick = b.ids[j]
			j++
		case j == len(b.ids):
			pick = a.ids[i]
			i++
		case a.ids[i] == b.ids[j]:
			pick = a.ids[i]
			i++
			j++
		case a.ids[i] < b.ids[j]:
			pick = a.ids[i]
			i++
		default:
			pick = b.ids[j]
			j++
		}
		out = append(out, pick)
	}
	return defSet{ids: out}, true, nil
}

func singletonDef(id reachingDefID) defSet {
	return defSet{ids: []reachingDefID{id}}
}

func transferReaching(slot int, nodeIdx int, writeSlot, writeWidth int, defWidth func(reachingDefID) int, before defSet, work *workbudget.Budget) (defSet, error) {
	if writeSlot < 0 {
		return before, chargeSetWork(work, 0)
	}
	if writeSlot == slot {
		if err := chargeSetWork(work, 1); err != nil {
			return defSet{}, err
		}
		return singletonDef(reachingDefID(nodeIdx)), nil
	}
	if writeWidth == 2 && writeSlot+1 == slot {
		if err := chargeSetWork(work, 1); err != nil {
			return defSet{}, err
		}
		return singletonDef(entryDefID), nil
	}
	if writeSlot == slot+1 {
		n := int64(len(before.ids))
		if err := chargeSetWork(work, n); err != nil {
			return defSet{}, err
		}
		// A narrow write over the tail of a wide slot invalidates the wide
		// definitions and leaves the entry/live-in marker. Without a wide
		// definition the write changes nothing, and the immutable input set can
		// be shared instead of copied.
		sawWide := false
		for _, id := range before.ids {
			if defWidth(id) == 2 {
				sawWide = true
				break
			}
		}
		if !sawWide {
			return before, nil
		}
		// entryDefID is -1 and ids are sorted, so prepending the marker and
		// filtering once yields the same sorted, deduplicated set without sort.
		out := make([]reachingDefID, 0, len(before.ids)+1)
		out = append(out, entryDefID)
		for _, id := range before.ids {
			if id == entryDefID || defWidth(id) == 2 {
				continue
			}
			out = append(out, id)
		}
		return defSet{ids: out}, nil
	}
	return before, chargeSetWork(work, 0)
}

// SparseReaching is a per-slot reaching-definition solver with identity-set
// sharing, query copies, and insert charging. Production solveSlot uses this.
type SparseReaching struct {
	g      *SemanticCFG
	in     map[int][]defSet // slot -> per-node facts
	reach  map[int][]bool
	ready  map[int]bool
	merges uint64
	visits uint64
	// chargedQueries remembers which (slot,node) pairs already paid the
	// definition-set query charge. It deliberately does not retain a second
	// []*OpCode copy of the facts; callers still receive a fresh slice.
	chargedQueries map[queryKey]struct{}
	// Scratch state reused across sequential slot solves. Generation stamps
	// avoid clearing O(nodes) arrays for every slot.
	reachStamp []uint32
	queueStamp []uint32
	generation uint32
	queue      []int
	seen       []int
}

type queryKey struct {
	slot int
	node int
}

// NewSparseReaching binds to an immutable SemanticCFG snapshot and charges
// g.Work (nil Work is unlimited).
func NewSparseReaching(g *SemanticCFG) *SparseReaching {
	// The production path uses solveFacts and never touches these maps, so they
	// are allocated only when the query API needs them.
	return &SparseReaching{g: g}
}

func (s *SparseReaching) ensureQueryMaps() {
	if s.in == nil {
		s.in = map[int][]defSet{}
		s.reach = map[int][]bool{}
		s.ready = map[int]bool{}
	}
	if s.chargedQueries == nil {
		s.chargedQueries = map[queryKey]struct{}{}
	}
}

func (s *SparseReaching) Merges() uint64 { return s.merges }
func (s *SparseReaching) Visits() uint64 { return s.visits }

func (s *SparseReaching) work() *workbudget.Budget {
	if s == nil || s.g == nil {
		return nil
	}
	return s.g.Work
}

func (s *SparseReaching) nodeWrite(op *OpCode) (slot, width int) {
	if op == nil || op.Instr == nil {
		return -1, 1
	}
	access := LocalAccessOf(op.Instr.OpCode)
	if !access.Write {
		return -1, 1
	}
	return GetStoreIdx(op), access.Width
}

func (s *SparseReaching) defWidth(id reachingDefID, slot int) int {
	if id == entryDefID {
		if s.g != nil && s.g.liveInWidth(slot) == 2 {
			return 2
		}
		return 1
	}
	if s.g == nil || int(id) < 0 || int(id) >= len(s.g.Nodes) {
		return 1
	}
	n := s.g.Nodes[id]
	if n == nil || n.Instr == nil {
		return 1
	}
	return LocalAccessOf(n.Instr.OpCode).Width
}

func (s *SparseReaching) entryFact(slot int) defSet {
	if s.g != nil && s.g.liveInIsCat2Tail(slot) {
		return defSet{}
	}
	return singletonDef(entryDefID)
}

func (s *SparseReaching) solve(slot int) error {
	s.ensureQueryMaps()
	if s.ready[slot] {
		return nil
	}
	g := s.g
	if g == nil || len(g.Nodes) == 0 {
		s.ready[slot] = true
		return nil
	}
	facts, seen, err := s.solveFacts(slot)
	if err != nil {
		return err
	}
	reachable := make([]bool, len(g.Nodes))
	for _, node := range seen {
		reachable[node] = true
	}
	s.in[slot] = facts
	s.reach[slot] = reachable
	s.ready[slot] = true
	return nil
}

// solveFacts runs the per-slot fixed point and returns node-indexed immutable
// facts. The seen list is scratch owned by the solver; callers that retain
// reachability must copy it before the next solve.
func (s *SparseReaching) solveFacts(slot int) ([]defSet, []int, error) {
	g := s.g
	if g == nil || len(g.Nodes) == 0 {
		return nil, nil, nil
	}
	n := len(g.Nodes)
	// Bill one slot at a time so MaxSetElementWork L-1/L/L+1 can trip with
	// Used>0 and Used<=L. A single Charge(n) against L<n fails with Used=0.
	for i := 0; i < n; i++ {
		if err := chargeSetWork(g.Work, 1); err != nil {
			return nil, nil, err
		}
	}
	facts := make([]defSet, n)
	order := g.order
	if order == nil {
		order = map[*OpCode]int{}
		for i, node := range g.Nodes {
			order[node] = i
		}
	}
	if len(s.reachStamp) != n {
		s.reachStamp = make([]uint32, n)
		s.queueStamp = make([]uint32, n)
	}
	s.generation++
	if s.generation == 0 {
		clear(s.reachStamp)
		clear(s.queueStamp)
		s.generation = 1
	}
	gen := s.generation
	s.seen = s.seen[:0]
	s.queue = s.queue[:0]
	facts[0] = s.entryFact(slot)
	s.reachStamp[0] = gen
	s.seen = append(s.seen, 0)
	s.queueStamp[0] = gen
	s.queue = append(s.queue, 0)
	for head := 0; head < len(s.queue); head++ {
		if g.MaxUpdates > 0 && g.Updates >= g.MaxUpdates {
			g.Err = fmt.Errorf("analysis_budget_exceeded: reaching definitions after %d updates", g.Updates)
			return nil, nil, g.Err
		}
		idx := s.queue[head]
		s.queueStamp[idx] = 0
		s.visits++
		g.Updates++
		if err := g.Work.Charge(workbudget.CounterAnalysisUpdates, 1); err != nil {
			g.Err = err
			return nil, nil, err
		}
		op := g.Nodes[idx]
		before := facts[idx]
		writeSlot, writeWidth := s.nodeWrite(op)
		after, err := transferReaching(slot, idx, writeSlot, writeWidth, func(id reachingDefID) int {
			return s.defWidth(id, slot)
		}, before, g.Work)
		if err != nil {
			g.Err = err
			return nil, nil, err
		}
		if g.outgoing == nil {
			continue
		}
		for _, ei := range g.outgoing[op] {
			edge := g.Edges[ei]
			state := after
			if edge.Kind == EdgeException {
				state = before
			}
			to, ok := order[edge.To]
			if !ok {
				continue
			}
			changed := s.reachStamp[to] != gen
			if changed {
				s.reachStamp[to] = gen
				s.seen = append(s.seen, to)
			}
			merged, grew, err := unionDef(facts[to], state, g.Work, &s.merges)
			if err != nil {
				g.Err = err
				return nil, nil, err
			}
			if grew {
				facts[to] = merged
				changed = true
			} else if facts[to].ids == nil && len(state.ids) == 0 {
				facts[to] = defSet{}
			}
			if changed && s.queueStamp[to] != gen {
				if err := g.Work.Check(); err != nil {
					g.Err = err
					return nil, nil, err
				}
				s.queueStamp[to] = gen
				s.queue = append(s.queue, to)
			}
		}
	}
	return facts, s.seen, nil
}

// Definitions returns reaching stores at `at` for `slot`, including the entry
// flag. The slice is a fresh copy; mutating it cannot poison the solver.
func (s *SparseReaching) Definitions(at *OpCode, slot int) ([]*OpCode, bool, error) {
	if s.g == nil || at == nil {
		return nil, false, nil
	}
	node, ok := s.g.order[at]
	if !ok {
		for i, n := range s.g.Nodes {
			if n == at {
				node, ok = i, true
				break
			}
		}
		if !ok {
			return nil, false, nil
		}
	}
	s.ensureQueryMaps()
	key := queryKey{slot: slot, node: node}
	_, charged := s.chargedQueries[key]
	if err := s.solve(slot); err != nil {
		return nil, false, err
	}
	facts := s.in[slot]
	if node < 0 || node >= len(facts) {
		return nil, false, nil
	}
	set := facts[node]
	if !charged {
		if err := chargeSetWork(s.work(), int64(len(set.ids))); err != nil {
			return nil, false, err
		}
		s.chargedQueries[key] = struct{}{}
	}
	// Definition ids are SemanticCFG node indexes kept in sorted order, so
	// translating them in place already yields bytecode order.
	out := make([]*OpCode, 0, len(set.ids))
	entry := false
	for _, id := range set.ids {
		if id == entryDefID {
			entry = true
			continue
		}
		if int(id) >= 0 && int(id) < len(s.g.Nodes) {
			out = append(out, s.g.Nodes[id])
		}
	}
	return out, entry, nil
}

func defOffsets(defs []*OpCode) []uint16 {
	out := make([]uint16, 0, len(defs))
	for _, d := range defs {
		if d != nil {
			out = append(out, d.CurrentOffset)
		}
	}
	return out
}
