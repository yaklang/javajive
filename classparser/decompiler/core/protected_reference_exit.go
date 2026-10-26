package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A definition inside try and its normal-exit reads are one source lifetime.
// JVM slot reuse after those reads does not give that later lifetime ownership
// of the escaped value. Prove a complete single-definition web using the frozen
// graph, then let the ordinary web binder separate its declaration and reads.
// No expression moves across a handler boundary and no initial value is added.
func (d *Decompiler) closedProtectedReferenceExit(webs *slotWeb, web int, store *OpCode) bool {
	g := d.semanticCFG
	if g == nil || g.Err != nil || len(g.Nodes) > 8192 || len(d.opCodes) > 8192 || len(webs.webOf) > 8192 || len(d.ExceptionTable) > 4096 || len(d.evaluationSnapshots) != 0 {
		return false
	}
	if d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(g.Nodes)+len(d.opCodes)+len(webs.webOf)+len(d.ExceptionTable))) != nil || d.Work.CheckAlloc(int64(len(g.Nodes)+len(d.opCodes))*96) != nil) {
		return false
	}
	for _, entry := range webs.entryWeb {
		if entry == web {
			return false
		}
	}
	// Select one actual protected interval crossed by a read. Membership is
	// only a lexical obligation; the immutable web proves value ownership.
	start, end, protected := uint16(0), uint16(65535), false
	for _, row := range d.ExceptionTable {
		if row == nil || row.EndPc <= row.StartPc {
			return false
		}
		if store.CurrentOffset >= row.StartPc && store.CurrentOffset < row.EndPc {
			protected = true
			if row.StartPc > start {
				start = row.StartPc
			}
			if row.EndPc < end {
				end = row.EndPc
			}
		}
	}
	if !protected {
		return false
	}
	present := map[*OpCode]bool{}
	decoded := map[*OpCode]bool{}
	for _, op := range d.opCodes {
		decoded[op] = true
	}
	slot := GetStoreIdx(store)
	if slot < 0 {
		return false
	}
	for _, op := range g.Nodes {
		if op == nil || op.Instr == nil {
			return false
		}
		present[op] = true
		access := LocalAccessOf(op.Instr.OpCode)
		if access.Read && GetRetrieveIdx(op) == slot || access.Write && GetStoreIdx(op) == slot {
			if _, known := webs.webOf[op]; !known || op.Instr.OpCode == OP_IINC {
				return false
			}
		}
	}
	definitions, escaped := 0, false
	for op, owner := range webs.webOf {
		if owner != web {
			continue
		}
		if !present[op] || !decoded[op] || op.IsCustom {
			return false
		}
		if isReferenceStoreOpcode(op.Instr.OpCode) {
			if op != store || GetStoreIdx(op) != slot || d.seedStoreRef(op) == nil {
				return false
			}
			definitions++
		} else if isReferenceLoadOpcode(op.Instr.OpCode) {
			if GetRetrieveIdx(op) != slot || len(op.stackProduced) != 1 {
				return false
			}
			if value, ok := op.stackProduced[0].(*values.SlotValue); !ok || value == nil || safeSeedOperand(value) == nil {
				return false
			}
			if op.CurrentOffset < start || op.CurrentOffset >= end {
				escaped = true
			}
		} else {
			return false
		}
	}
	return definitions == 1 && escaped
}
