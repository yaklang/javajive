package ssalower

import "fmt"

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

func Sequentialize(copies []Copy, nextTemp func() VarID) []Move {
	var remain []Copy
	for _, c := range copies {
		if c.Dst == c.Src {
			continue
		}
		remain = append(remain, c)
	}
	var out []Move
	for len(remain) > 0 {
		emitted := false
		for i := 0; i < len(remain); i++ {
			c := remain[i]
			if sourceOf(remain, c.Dst) {
				continue
			}
			out = append(out, Move{Dst: c.Dst, Src: c.Src})
			remain = append(remain[:i], remain[i+1:]...)
			emitted = true
			break
		}
		if emitted {
			continue
		}
		c := remain[0]
		t := nextTemp()
		out = append(out, Move{Dst: t, Src: c.Dst, Tmp: true})
		for i := range remain {
			if remain[i].Src == c.Dst {
				remain[i].Src = t
			}
		}
	}
	return out
}

func sourceOf(copies []Copy, id VarID) bool {
	for _, c := range copies {
		if c.Src == id {
			return true
		}
	}
	return false
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
