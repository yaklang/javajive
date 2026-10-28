package core

import "github.com/yaklang/javajive/internal/workbudget"

// IINC reads and writes one source location, but a later reuse of that JVM
// slot is not an update to every earlier local. Exclude increments only after
// their complete input definitions are proved disjoint on the immutable CFG.
// Expanding prior IINC inputs preserves an update chain's source identity.
func (d *Decompiler) originalIncrementsOutsideWeb(webs *slotWeb, web, slot int) bool {
	g := d.semanticCFG
	if g == nil || g.Err != nil || len(g.Nodes) > 8192 || len(d.opCodes) > 8192 || len(webs.webOf) > 8192 {
		return false
	}
	if d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(g.Nodes)+len(d.opCodes)+len(webs.webOf))) != nil || d.Work.CheckAlloc(int64(len(g.Nodes)+len(d.opCodes))*96) != nil) {
		return false
	}
	decoded := map[*OpCode]bool{}
	original := map[*OpCode]bool{}
	for _, op := range d.opCodes {
		decoded[op] = true
	}
	for _, op := range g.Nodes {
		if op == nil || op.Instr == nil {
			return false
		}
		original[op] = true
	}
	for op, owner := range webs.webOf {
		if owner == web && !original[op] {
			return false
		}
	}
	found, steps := false, 0
	for _, op := range g.Nodes {
		if op.Instr.OpCode != OP_IINC || GetStoreIdx(op) != slot {
			continue
		}
		if !decoded[op] || op.IsCustom || len(op.Data) != 2 {
			return false
		}
		pending, seen := []*OpCode{op}, map[*OpCode]bool{}
		terminal := false
		for len(pending) > 0 {
			input := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if seen[input] {
				continue
			}
			seen[input] = true
			steps++
			if steps > 65536 || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
				return false
			}
			definitions, entry := g.ReachingDefinitions(input, slot)
			if g.Err != nil || entry || len(definitions) == 0 {
				return false
			}
			steps += len(definitions)
			if steps > 65536 || d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, int64(len(definitions))) != nil {
				return false
			}
			for _, definition := range definitions {
				if !original[definition] || !decoded[definition] || definition.IsCustom || GetStoreIdx(definition) != slot {
					return false
				}
				if definition.Instr.OpCode == OP_IINC {
					if len(definition.Data) != 2 {
						return false
					}
					pending = append(pending, definition)
					continue
				}
				owner, known := webs.webOf[definition]
				if !known || owner == web || !isLocalStoreOpcode(definition.Instr.OpCode) {
					return false
				}
				terminal = true
			}
		}
		if !terminal {
			return false
		}
		found = true
	}
	for _, op := range d.opCodes {
		if op != nil && op.Instr != nil && op.Instr.OpCode == OP_IINC && GetStoreIdx(op) == slot && !original[op] {
			return false
		}
	}
	return found
}
