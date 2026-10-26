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
// This changes identity only: the stored operand and destination declaration
// retain their existing types. Boolean declarations, handler completions,
// dynamic captures and incomplete/multiple-definition webs keep their existing
// proofs. Narrow declarations require an exact duplicated-word source below.
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
		if !ok || primitive == nil {
			continue
		}
		category := primitive.Name
		narrow := category == types.JavaByte || category == types.JavaShort || category == types.JavaChar
		if narrow {
			category = types.JavaInteger
		}
		if !dynamicPrimitiveStore(store.Instr.OpCode, category) {
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
		source, sourceRef := values.UnpackSoltValue(store.stackConsumed[0]).(*values.JavaRef)
		if !sourceRef || source == nil || source.Id == nil || source.CustomValue != nil || source.StackVar != nil || d.localHasBooleanDescriptorUse(ref) || d.localHasBooleanDescriptorUse(source) {
			continue
		}
		if load != nil {
			sourceWeb, sourceKnown := webs.webOf[load]
			if narrow || !dynamicPrimitiveLoad(load.Instr.OpCode, category) || GetRetrieveIdx(load) == GetStoreIdx(store) || !sourceKnown || sourceWeb == web || !isExactPrimer(source.Type(), primitive.Name) {
				continue
			}
		} else if !d.duplicatedPrimitiveCopySource(store, source, primitive.Name) {
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
				if !dynamicPrimitiveLoad(op.Instr.OpCode, category) || len(op.stackProduced) != 1 {
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

// DUP materializes one evaluated word for several consumers. A following
// STORE copies that word into a local lifetime, just as LOAD; STORE does.
// Keep both the stack temporary and the STORE at their original positions;
// only the destination's proved def-use web may receive a distinct binder.
func (d *Decompiler) duplicatedPrimitiveCopySource(store *OpCode, source *values.JavaRef, kind string) bool {
	if d == nil || store == nil || store.Instr == nil || store.IsCustom || source == nil || source.Type() == nil || len(store.stackConsumed) != 1 || d.semanticCFG == nil || d.semanticCFG.Err != nil {
		return false
	}
	if _, valid := localPostInstructionWidth(store); !valid {
		return false
	}
	g := d.semanticCFG
	incoming := g.incoming[store]
	if len(incoming) != 1 || incoming[0] < 0 || incoming[0] >= len(g.Edges) {
		return false
	}
	edge := g.Edges[incoming[0]]
	duplicate := edge.From
	if edge.To != store || edge.Kind != EdgeFallthrough || duplicate == nil || duplicate.Instr == nil || duplicate.IsCustom || len(g.outgoing[duplicate]) != 1 || g.outgoing[duplicate][0] != incoming[0] || int(duplicate.CurrentOffset)+1 != int(store.CurrentOffset) {
		return false
	}
	if _, known := g.order[duplicate]; !known {
		return false
	}
	if _, known := g.order[store]; !known {
		return false
	}
	code := OP_DUP
	if kind == types.JavaLong || kind == types.JavaDouble {
		code = OP_DUP2
	}
	consumed, produced := 1, 2
	if code == OP_DUP2 {
		// The simulator first materializes the category-2 word, then pops
		// that temporary before pushing its two copies. Those bookkeeping
		// entries must all denote this same cell, never two category-1 words.
		consumed, produced = 2, 3
	}
	if duplicate.Instr.OpCode != code || len(duplicate.Data) != 0 || duplicate.IsWide || len(duplicate.stackProduced) != produced || len(duplicate.stackConsumed) != consumed {
		return false
	}
	infos, converted := d.opcodeIdToRef[duplicate], d.dupConvertedRefValue[duplicate]
	if len(infos) != 1 || infos[0][0] != source || infos[0][1] != true || len(converted) != 1 || converted[0] != duplicate.stackConsumed[0] {
		return false
	}
	pc, originalCode, known := source.OriginalStackMaterializationWitness(converted[0])
	if !known || pc != int(duplicate.CurrentOffset) || originalCode != code {
		return false
	}
	// Byte/short/char stack producers are computational ints. Widening
	// their stored copy to the already declared int preserves its word;
	// boolean and other conversions require a different proof.
	primitive, ok := source.Type().RawType().(*types.JavaPrimer)
	if !ok || primitive == nil || primitive.Name != kind && !(kind == types.JavaInteger && (primitive.Name == types.JavaByte || primitive.Name == types.JavaShort || primitive.Name == types.JavaChar)) {
		return false
	}
	operand := store.stackConsumed[0]
	if d.stackValueProducers[operand] != duplicate || code == OP_DUP2 && duplicate.stackConsumed[1] != operand {
		return false
	}
	for _, cell := range duplicate.stackProduced {
		if cell != operand {
			return false
		}
	}
	return true
}
