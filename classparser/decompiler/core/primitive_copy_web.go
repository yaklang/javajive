package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A local copy snapshots the current word; it does not alias subsequent writes
// to the source or a later lifetime of the destination slot. The simulator can
// reuse one JavaRef for both lifetimes. Partition a witnessed copy by the solved
// definition web before source scopes rename it or dead-store analysis runs.
// This changes identity only: both sides and every load retain the same exact
// primitive type. Boolean/narrow domains, handler completions, dynamic capture
// snapshots and incomplete/multiple-definition webs keep their existing proofs.
func (d *Decompiler) partitionSharedPrimitiveCopies() {
	if d == nil || d.getenv("JDEC_LIVEINTERVAL_OFF") != "" || len(d.ExceptionTable) != 0 || len(d.opCodes) > 8192 || len(d.evaluationSnapshots) != 0 || !d.hasMultipleNumericStores() {
		return
	}
	// Typed ++/-- packets connect their read and write webs into one
	// source location. Count those definitions together before splitting
	// a copy, including long/float/double updates without an IINC opcode.
	webs := d.localUpdateSourceWebs()
	if webs == nil {
		return
	}
	if d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, int64(len(d.opCodes)+len(webs.webOf))) != nil || d.Work.CheckAlloc(int64(len(d.opCodes)+len(webs.webOf))*192) != nil) {
		return
	}
	decoded := map[*OpCode]bool{}
	loads := map[values.JavaValue]*OpCode{}
	ambiguousLoads := map[values.JavaValue]bool{}
	owners := map[*values.JavaRef]map[int]bool{}
	stores := map[int][]*OpCode{}
	entries := map[int]bool{}
	incrementedSlots := map[int]bool{}
	for _, web := range webs.entryWeb {
		entries[web] = true
	}
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			continue
		}
		decoded[op] = true
		if op.Instr.OpCode == OP_IINC {
			// IINC defines a new reaching web but updates the same source
			// variable. A single-store proof cannot partition its initializer
			// without also rebinding the recurrence's read and write.
			incrementedSlots[GetStoreIdx(op)] = true
		}
		if isLocalLoadOpcode(op.Instr.OpCode) && len(op.stackProduced) == 1 {
			if slot, ok := op.stackProduced[0].(*values.SlotValue); ok && slot != nil {
				ambiguousLoads[slot] = ambiguousLoads[slot] || loads[slot] != nil
				loads[slot] = op
			}
		}
		web, known := webs.webOf[op]
		infos := d.opcodeIdToRef[op]
		if known && isLocalStoreOpcode(op.Instr.OpCode) && len(infos) == 1 {
			if ref, ok := infos[0][0].(*values.JavaRef); ok && ref != nil {
				if owners[ref] == nil {
					owners[ref] = map[int]bool{}
				}
				owners[ref][web] = true
			}
		}
	}
	for op, web := range webs.webOf {
		if op != nil && op.Instr != nil && isLocalStoreOpcode(op.Instr.OpCode) {
			stores[web] = append(stores[web], op)
		}
	}
	for _, store := range d.opCodes {
		if store == nil || store.Instr == nil || !isLocalStoreOpcode(store.Instr.OpCode) || len(store.stackConsumed) != 1 {
			continue
		}
		web, known := webs.webOf[store]
		infos := d.opcodeIdToRef[store]
		if !known || entries[web] || incrementedSlots[GetStoreIdx(store)] || len(stores[web]) != 1 || len(infos) != 1 {
			continue
		}
		ref, ok := infos[0][0].(*values.JavaRef)
		if !ok || ref == nil || ref.Id == nil || ref.IsThis || ref.IsParam || ref.CustomValue != nil || ref.StackVar != nil || ref.SolvedWebIdentity != nil || len(owners[ref]) < 2 || ref.Type() == nil {
			continue
		}
		primitive, ok := ref.Type().RawType().(*types.JavaPrimer)
		if !ok || primitive == nil || (primitive.Name != types.JavaInteger && primitive.Name != types.JavaLong && primitive.Name != types.JavaFloat && primitive.Name != types.JavaDouble) || !dynamicPrimitiveStore(store.Instr.OpCode, primitive.Name) {
			continue
		}
		// A consumer-owned stack-lifetime view can wrap the original LOAD.
		// Follow only transparent views, never a local's Val/initializer.
		var load *OpCode
		operand := store.stackConsumed[0]
		for depth := 0; depth < 16 && operand != nil; depth++ {
			if ambiguousLoads[operand] {
				break
			}
			if loads[operand] != nil {
				load = loads[operand]
				break
			}
			slot, ok := operand.(*values.SlotValue)
			if !ok || slot == nil {
				break
			}
			operand = slot.GetValue()
		}
		if load == nil || !dynamicPrimitiveLoad(load.Instr.OpCode, primitive.Name) || GetRetrieveIdx(load) == GetStoreIdx(store) {
			continue
		}
		sourceWeb, sourceKnown := webs.webOf[load]
		source, sourceRef := values.UnpackSoltValue(store.stackConsumed[0]).(*values.JavaRef)
		if !sourceKnown || sourceWeb == web || !sourceRef || source == nil || source.Id == nil || source.CustomValue != nil || source.StackVar != nil || !isExactPrimer(source.Type(), primitive.Name) || d.localHasBooleanDescriptorUse(ref) || d.localHasBooleanDescriptorUse(source) {
			continue
		}
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, int64(len(webs.webOf))) != nil {
			return
		}
		valid, uses := true, 0
		for op, owner := range webs.webOf {
			if owner != web {
				continue
			}
			if !decoded[op] || op == nil || op.Instr == nil || op.Instr.OpCode == OP_IINC {
				valid = false
				break
			}
			if isLocalLoadOpcode(op.Instr.OpCode) {
				if !dynamicPrimitiveLoad(op.Instr.OpCode, primitive.Name) || len(op.stackProduced) != 1 {
					valid = false
					break
				}
				slot, ok := op.stackProduced[0].(*values.SlotValue)
				if !ok || slot == nil || !isExactPrimer(slot.Type(), primitive.Name) {
					valid = false
					break
				}
				uses++
			}
		}
		if valid && uses != 0 {
			d.bindProvedPrimitiveWeb(webs, web, stores[web], primitive.Name)
		}
	}
}
