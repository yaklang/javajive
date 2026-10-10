package core

import "github.com/yaklang/javajive/internal/workbudget"

// A javac monitor snapshot is DUP; ASTORE; MONITORENTER. The exit's ALOAD must
// have that store as its sole reaching definition in the immutable CFG,
// including exceptional input states. Text spelling and mutable ref.Val are
// deliberately irrelevant. Then a two-state ownership analysis proves each
// acquisition/release and every exceptional boundary over the entire method.
func (d *Decompiler) originalMonitorOwners() map[*OpCode]int {
	out := map[*OpCode]int{}
	g := d.semanticCFG
	if g == nil || g.Err != nil || d.opcodeCodeLength != len(d.bytecodes) || len(g.Nodes) == 0 || len(g.Nodes) > 16384 {
		return out
	}
	charge := func() bool { return d.Work == nil || d.Work.Charge(workbudget.CounterGraphScans, 1) == nil }
	previous := func(at *OpCode) *OpCode {
		var p *OpCode
		for _, i := range g.incoming[at] {
			e := g.Edges[i]
			if e.Kind == EdgeException || e.Kind != EdgeFallthrough || p != nil {
				return nil
			}
			p = e.From
		}
		return p
	}
	stores := map[*OpCode]*OpCode{}
	for _, enter := range g.Nodes {
		if !charge() {
			return out
		}
		if enter == nil || enter.Instr == nil || enter.IsCustom || enter.Instr.OpCode != OP_MONITORENTER {
			continue
		}
		store := previous(enter)
		if store == nil || store.Instr == nil || store.IsCustom || !isReferenceStoreOpcode(store.Instr.OpCode) {
			continue
		}
		dup := previous(store)
		if dup == nil || dup.Instr == nil || dup.IsCustom || dup.Instr.OpCode != OP_DUP {
			continue
		}
		if len(stores) == 0 && d.Work != nil && d.Work.CheckAlloc(int64(len(g.Nodes))*512) != nil {
			return out
		}
		stores[store] = enter
	}
	if len(stores) == 0 || len(stores) > 64 {
		return out
	}
	exits := map[*OpCode][]*OpCode{}
	ambiguous := map[*OpCode]bool{}
	for _, exit := range g.Nodes {
		if !charge() {
			return out
		}
		if exit == nil || exit.Instr == nil || exit.IsCustom || exit.Instr.OpCode != OP_MONITOREXIT {
			continue
		}
		load := previous(exit)
		if load == nil || load.Instr == nil || load.IsCustom || !isReferenceLoadOpcode(load.Instr.OpCode) {
			continue
		}
		defs, entry := g.ReachingDefinitions(load, GetRetrieveIdx(load))
		for _, def := range defs {
			if owner := stores[def]; owner != nil {
				if entry || len(defs) != 1 {
					ambiguous[owner] = true
				} else {
					exits[owner] = append(exits[owner], exit)
				}
			}
		}
	}
	firstAll := map[*OpCode]int{}
	for _, edge := range g.Edges {
		if !charge() {
			return out
		}
		if edge.Kind != EdgeException {
			continue
		}
		if edge.HandlerOrder < 0 || edge.HandlerOrder >= len(d.ExceptionTable) || d.ExceptionTable[edge.HandlerOrder] == nil {
			return out
		}
		if d.ExceptionTable[edge.HandlerOrder].CatchType == 0 {
			if current, known := firstAll[edge.From]; !known || edge.HandlerOrder < current {
				firstAll[edge.From] = edge.HandlerOrder
			}
		}
	}
	firstCatchAll := func(n *OpCode) int {
		if first, known := firstAll[n]; known {
			return first
		}
		return -1
	}
	for _, owner := range stores {
		if ambiguous[owner] || len(exits[owner]) == 0 {
			continue
		}
		paired := map[*OpCode]bool{}
		for _, exit := range exits[owner] {
			paired[exit] = true
		}
		// Bit 1: no held acquisition. Bit 2: this exact snapshot is held. A union
		// containing both is not sufficient to erase any monitor operation.
		state := map[*OpCode]uint8{g.Nodes[0]: 1}
		queue := []*OpCode{g.Nodes[0]}
		queued := map[*OpCode]bool{g.Nodes[0]: true}
		valid := true
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			delete(queued, n)
			if !charge() {
				return map[*OpCode]int{}
			}
			before := state[n]
			after := before
			if n == owner {
				after = 2
			} else if paired[n] {
				after = 1
			}
			for _, i := range g.outgoing[n] {
				e := g.Edges[i]
				if first := firstCatchAll(n); e.Kind == EdgeException && first >= 0 && e.HandlerOrder > first {
					continue
				}
				s := after
				if e.Kind == EdgeException {
					s = before
				}
				joined := state[e.To] | s
				if joined != state[e.To] {
					state[e.To] = joined
					if !queued[e.To] {
						queued[e.To] = true
						queue = append(queue, e.To)
					}
				}
			}
		}
		if state[owner] == 0 || state[owner]&2 != 0 {
			continue
		}
		for _, n := range g.Nodes {
			if !charge() {
				return map[*OpCode]int{}
			}
			s := state[n]
			if s == 0 {
				continue
			}
			if paired[n] && s != 2 {
				valid = false
				break
			}
			if s&2 == 0 {
				continue
			}
			if len(g.outgoing[n]) == 0 {
				valid = false
				break
			}
			if n.Instr != nil && MayThrowOpcode(n.Instr.OpCode) {
				covered := firstCatchAll(n) >= 0
				if !covered {
					valid = false
					break
				}
			}
		}
		if valid {
			pc := int(owner.CurrentOffset)
			out[owner] = pc
			for _, exit := range exits[owner] {
				out[exit] = pc
			}
		}
	}
	return out
}
