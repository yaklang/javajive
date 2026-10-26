package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// An IINC reads and writes its original incoming local. The DFS simulator's
// global slot table can instead leave a later, disjoint lifetime here, even
// when both lifetimes have the same inferred type. After simulation and web
// solving, bind a standalone update only when ALL immutable incoming paths
// reduce to one existing int source binding. Follow update cycles to actual
// stores; an unseeded cycle or entry/unknown input grants no such proof.
//
// This changes only the instruction's source identity. The original stores,
// load operands, signed delta and evaluation point are untouched. Folded
// pre/post updates own embedded operands and require a separate proof.
func (d *Decompiler) restoreStandaloneIncrementBindings() {
	if d == nil || d.getenv("JDEC_LIVEINTERVAL_OFF") != "" || d.semanticCFG == nil || d.semanticCFG.Err != nil || len(d.semanticCFG.Nodes) > 8192 || len(d.opCodes) > 8192 {
		return
	}
	g := d.semanticCFG
	if d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(g.Nodes)+len(d.opCodes))) != nil || d.Work.CheckAlloc(int64(len(g.Nodes)+len(d.opCodes))*128) != nil) {
		return
	}
	original, decoded := map[*OpCode]bool{}, map[*OpCode]bool{}
	for _, op := range g.Nodes {
		original[op] = true
	}
	for _, op := range d.opCodes {
		decoded[op] = true
	}
	remaining := 65536
	charge := func(n int) bool {
		remaining -= n
		return remaining >= 0 && (d.Work == nil || d.Work.Charge(workbudget.CounterGraphScans, int64(n)) == nil)
	}
	known := func(op *OpCode) bool {
		return op != nil && op.Instr != nil && !op.IsCustom && original[op] && decoded[op]
	}
	update := func(op *OpCode) bool {
		if !known(op) || op.Instr.OpCode != OP_IINC || op.IincFoldedToPostOp || op.IincFoldedToPreOp || len(op.stackConsumed) != 0 || len(op.stackProduced) != 0 {
			return false
		}
		width := 2
		if op.IsWide {
			width = 4
		}
		return len(op.Data) == width
	}
	for _, op := range d.opCodes {
		if !update(op) {
			continue
		}
		slot := GetStoreIdx(op)
		pending, seen := []*OpCode{op}, map[*OpCode]bool{}
		var canon *values.JavaRef
		valid := true
		for len(pending) > 0 && valid {
			input := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if seen[input] {
				continue
			}
			seen[input] = true
			if !charge(1) {
				return
			}
			definitions, entry := g.ReachingDefinitions(input, slot)
			if !charge(len(definitions)) {
				return
			}
			if g.Err != nil || entry || len(definitions) == 0 {
				valid = false
				break
			}
			for _, definition := range definitions {
				if !known(definition) || GetStoreIdx(definition) != slot {
					valid = false
					break
				}
				if definition.Instr.OpCode == OP_IINC {
					if !update(definition) {
						valid = false
						break
					}
					pending = append(pending, definition)
					continue
				}
				infos := d.opcodeIdToRef[definition]
				if !originalIntLocalPacket(definition, false) || len(infos) != 1 || len(definition.stackConsumed) != 1 {
					valid = false
					break
				}
				ref, ok := infos[0][0].(*values.JavaRef)
				if !ok || ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || !isExactPrimer(ref.Type(), types.JavaInteger) || canon != nil && canon != ref {
					valid = false
					break
				}
				canon = ref
			}
		}
		if valid && canon != nil && op.Ref != canon {
			op.Ref = canon
			d.disFoldRef = append(d.disFoldRef, canon)
		}
	}
}
