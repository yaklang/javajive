package core

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

// reachingDefID is a stable definition identity: -1 is the method-entry/nil
// pseudo-definition; non-negative values are SemanticCFG node indices.
type reachingDefID int

const entryDefID reachingDefID = -1

type defSet struct {
	ids []reachingDefID // sorted, unique
}

func (s defSet) clone() defSet {
	if len(s.ids) == 0 {
		return defSet{}
	}
	out := make([]reachingDefID, len(s.ids))
	copy(out, s.ids)
	return defSet{ids: out}
}

func (s defSet) key() string {
	if len(s.ids) == 0 {
		return ""
	}
	parts := make([]string, len(s.ids))
	for i, id := range s.ids {
		parts[i] = strconv.Itoa(int(id))
	}
	return strings.Join(parts, ",")
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
	out := make([]reachingDefID, 0, len(a.ids)+len(b.ids))
	i, j := 0, 0
	changed := false
	for i < len(a.ids) || j < len(b.ids) {
		var pick reachingDefID
		switch {
		case i == len(a.ids):
			pick = b.ids[j]
			j++
			changed = true
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
			changed = true
		}
		if merges != nil {
			*merges++
		}
		out = append(out, pick)
	}
	return defSet{ids: out}, changed, nil
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
		out := make([]reachingDefID, 0, len(before.ids)+1)
		sawNil := false
		for _, id := range before.ids {
			if defWidth(id) == 2 {
				sawNil = true
				continue
			}
			out = append(out, id)
		}
		if sawNil {
			out = append([]reachingDefID{entryDefID}, out...)
			sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
			u := out[:0]
			var last reachingDefID
			for i, id := range out {
				if i == 0 || id != last {
					u = append(u, id)
					last = id
				}
			}
			out = u
		}
		return defSet{ids: out}, nil
	}
	return before, chargeSetWork(work, 0)
}

// SparseReaching is a per-slot reaching-definition solver with identity-set
// sharing, query copies, and insert charging. Production solveSlot uses this.
type SparseReaching struct {
	g          *SemanticCFG
	in         map[int][]defSet // slot -> per-node facts
	reach      map[int][]bool
	ready      map[int]bool
	merges     uint64
	visits     uint64
	queryCache map[queryKey]cachedQuery
}

type queryKey struct {
	slot int
	node int
}

type cachedQuery struct {
	defs  []*OpCode
	entry bool
}

// NewSparseReaching binds to an immutable SemanticCFG snapshot and charges
// g.Work (nil Work is unlimited).
func NewSparseReaching(g *SemanticCFG) *SparseReaching {
	return &SparseReaching{
		g:          g,
		in:         map[int][]defSet{},
		reach:      map[int][]bool{},
		ready:      map[int]bool{},
		queryCache: map[queryKey]cachedQuery{},
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
	if s.ready[slot] {
		return nil
	}
	g := s.g
	if g == nil || len(g.Nodes) == 0 {
		s.ready[slot] = true
		return nil
	}
	n := len(g.Nodes)
	// Bill one slot at a time so MaxSetElementWork L-1/L/L+1 can trip with
	// Used>0 and Used<=L. A single Charge(n) against L<n fails with Used=0.
	for i := 0; i < n; i++ {
		if err := chargeSetWork(g.Work, 1); err != nil {
			return err
		}
	}
	facts := make([]defSet, n)
	reachable := make([]bool, n)
	order := g.order
	if order == nil {
		order = map[*OpCode]int{}
		for i, node := range g.Nodes {
			order[node] = i
		}
	}
	facts[0] = s.entryFact(slot)
	reachable[0] = true
	queue := make([]int, 0, 8)
	queued := make([]bool, n)
	queue = append(queue, 0)
	queued[0] = true
	for head := 0; head < len(queue); head++ {
		if g.MaxUpdates > 0 && g.Updates >= g.MaxUpdates {
			g.Err = fmt.Errorf("analysis_budget_exceeded: reaching definitions after %d updates", g.Updates)
			return g.Err
		}
		idx := queue[head]
		queued[idx] = false
		s.visits++
		g.Updates++
		if err := g.Work.Charge(workbudget.CounterAnalysisUpdates, 1); err != nil {
			g.Err = err
			return err
		}
		op := g.Nodes[idx]
		before := facts[idx]
		writeSlot, writeWidth := s.nodeWrite(op)
		after, err := transferReaching(slot, idx, writeSlot, writeWidth, func(id reachingDefID) int {
			return s.defWidth(id, slot)
		}, before, g.Work)
		if err != nil {
			g.Err = err
			return err
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
			changed := !reachable[to]
			reachable[to] = true
			merged, grew, err := unionDef(facts[to], state, g.Work, &s.merges)
			if err != nil {
				g.Err = err
				return err
			}
			if grew {
				facts[to] = merged
				changed = true
			} else if facts[to].ids == nil && reachable[to] && len(state.ids) == 0 {
				facts[to] = defSet{}
			}
			if changed && !queued[to] {
				if err := g.Work.Check(); err != nil {
					g.Err = err
					return err
				}
				queued[to] = true
				queue = append(queue, to)
			}
		}
	}
	s.in[slot] = facts
	s.reach[slot] = reachable
	s.ready[slot] = true
	return nil
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
	key := queryKey{slot: slot, node: node}
	if cached, hit := s.queryCache[key]; hit {
		cp := make([]*OpCode, len(cached.defs))
		copy(cp, cached.defs)
		return cp, cached.entry, nil
	}
	if err := s.solve(slot); err != nil {
		return nil, false, err
	}
	facts := s.in[slot]
	if node < 0 || node >= len(facts) {
		return nil, false, nil
	}
	set := facts[node]
	if err := chargeSetWork(s.work(), int64(len(set.ids))); err != nil {
		return nil, false, err
	}
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
	sort.Slice(out, func(i, j int) bool { return s.g.order[out[i]] < s.g.order[out[j]] })
	s.queryCache[key] = cachedQuery{defs: out, entry: entry}
	cp := make([]*OpCode, len(out))
	copy(cp, out)
	return cp, entry, nil
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
