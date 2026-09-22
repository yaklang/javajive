package ssalower

import (
	"container/heap"
	"fmt"
)

type VarID int

type Var struct {
	ID    VarID
	Name  string
	Width int
	Kind  string
}

type Copy struct {
	Dst VarID
	Src VarID
}

type Move struct {
	Dst VarID
	Src VarID
	Tmp bool
}

// Sequentialize is the legacy convenience API. Invalid input panics instead of
// returning a partial plan. Production lowering uses SequentializeChecked.
func Sequentialize(copies []Copy, nextTemp func() VarID) []Move {
	moves, err := SequentializeChecked(copies, nextTemp)
	if err != nil {
		panic(err)
	}
	return moves
}

type idHeap []VarID

func (h idHeap) Len() int           { return len(h) }
func (h idHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h idHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *idHeap) Push(v any)        { *h = append(*h, v.(VarID)) }
func (h *idHeap) Pop() any          { a := *h; v := a[len(a)-1]; *h = a[:len(a)-1]; return v }

// SequentializeChecked schedules logical values, never individual wide slots.
// Destinations are unique even for self copies. A cycle saves a destination's
// OLD value and redirects every reader before overwriting it. The allocator
// must reserve the whole method's namespace, not just this edge's values.
func SequentializeChecked(copies []Copy, nextTemp func() VarID) ([]Move, error) {
	pending := map[VarID]VarID{}
	readers := map[VarID]map[VarID]bool{}
	occupied, destinations := map[VarID]bool{}, map[VarID]bool{}
	for _, c := range copies {
		if c.Dst <= 0 || c.Src <= 0 || destinations[c.Dst] {
			return nil, fmt.Errorf("invalid_input: invalid or duplicate copy destination %d", c.Dst)
		}
		destinations[c.Dst] = true
		occupied[c.Dst], occupied[c.Src] = true, true
		if c.Dst == c.Src {
			continue
		}
		pending[c.Dst] = c.Src
		if readers[c.Src] == nil {
			readers[c.Src] = map[VarID]bool{}
		}
		readers[c.Src][c.Dst] = true
	}
	ready, all := &idHeap{}, &idHeap{}
	for d := range pending {
		heap.Push(all, d)
		if len(readers[d]) == 0 {
			heap.Push(ready, d)
		}
	}
	var out []Move
	for len(pending) > 0 {
		for ready.Len() > 0 {
			d := heap.Pop(ready).(VarID)
			s, ok := pending[d]
			if !ok || len(readers[d]) != 0 {
				continue
			}
			out = append(out, Move{Dst: d, Src: s})
			delete(pending, d)
			delete(readers[s], d)
			if _, ok := pending[s]; ok && len(readers[s]) == 0 {
				heap.Push(ready, s)
			}
		}
		if len(pending) == 0 {
			break
		}
		var victim VarID
		for all.Len() > 0 {
			v := heap.Pop(all).(VarID)
			if _, ok := pending[v]; ok {
				victim = v
				break
			}
		}
		if victim == 0 || nextTemp == nil {
			return nil, fmt.Errorf("invalid_input: cycle requires fresh temporary")
		}
		tmp := nextTemp()
		if tmp <= 0 || occupied[tmp] {
			return nil, fmt.Errorf("invalid_input: temporary %d is not fresh", tmp)
		}
		occupied[tmp] = true
		out = append(out, Move{Dst: tmp, Src: victim, Tmp: true})
		readers[tmp] = map[VarID]bool{}
		for d := range readers[victim] {
			pending[d] = tmp
			readers[tmp][d] = true
		}
		delete(readers, victim)
		heap.Push(ready, victim)
	}
	return out, nil
}

func ParallelEval(copies []Copy, state map[VarID]int64) map[VarID]int64 {
	next := make(map[VarID]int64, len(state))
	for k, v := range state {
		next[k] = v
	}
	for _, c := range copies {
		next[c.Dst] = state[c.Src]
	}
	return next
}

func SerialEval(moves []Move, state map[VarID]int64) map[VarID]int64 {
	st := make(map[VarID]int64, len(state)+4)
	for k, v := range state {
		st[k] = v
	}
	for _, m := range moves {
		st[m.Dst] = st[m.Src]
	}
	return st
}

func StatesEqual(a, b map[VarID]int64, ids []VarID) bool {
	for _, id := range ids {
		if a[id] != b[id] {
			return false
		}
	}
	return true
}

func TempName(id VarID) string { return fmt.Sprintf("t%d", id) }
