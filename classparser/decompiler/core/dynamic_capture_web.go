package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A dynamic operand has an explicit original descriptor domain and retains
// its original LOAD's SlotValue. If a complete primitive definition web shares
// a simulator ref with another lifetime, bind only that web to a fresh source
// declaration. The snapshot then observes the repaired LOAD without changing
// its factory PC, producer expression, evaluation order or physical descriptor.
func (d *Decompiler) partitionSharedDynamicCaptureWebs() {
	if len(d.evaluationSnapshots) == 0 || len(d.opCodes) > 8192 {
		return
	}
	webs := d.slotWebs()
	if webs == nil || d.Work != nil && (d.Work.CheckAlloc(int64(len(d.opCodes))*256) != nil || d.Work.Charge(workbudget.CounterGraphScans, int64(len(d.opCodes))) != nil) {
		return
	}
	stores := map[int][]*OpCode{}
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil {
			return
		}
		if isLocalStoreOpcode(op.Instr.OpCode) {
			if web, known := webs.webOf[op]; known {
				stores[web] = append(stores[web], op)
			}
		}
	}
	type capture struct {
		value *values.SlotValue
		load  *OpCode
		kind  string
	}
	captures, order := map[int][]capture{}, []int{}
	captureCount := 0
	for position, site := range d.opCodes {
		if site.IsCustom || site.Instr.OpCode != OP_INVOKEDYNAMIC {
			continue
		}
		operands := []EvaluationSnapshot{}
		for _, snapshot := range d.evaluationSnapshots[site] {
			if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
				return
			}
			if snapshot.Operand {
				operands = append(operands, snapshot)
				if len(operands) > 64 {
					break
				}
			}
		}
		if len(operands) == 0 || len(operands) > 64 || position < len(operands) {
			continue
		}
		for index, snapshot := range operands {
			if !snapshot.Operand || snapshot.ExpectedType == nil || snapshot.OriginPC != int(site.CurrentOffset) {
				continue
			}
			typ, primitive := snapshot.ExpectedType.RawType().(*types.JavaPrimer)
			if !primitive || typ.Name != types.JavaInteger && typ.Name != types.JavaLong && typ.Name != types.JavaFloat && typ.Name != types.JavaDouble {
				continue
			}
			value, ok := snapshot.Value.(*values.SlotValue)
			if !ok {
				continue
			}
			// Pop creates a different forwarding SlotValue from stackProduced.
			// Identify the original contiguous LOAD packet by physical order,
			// then require its actual simulated ref to be this operand's seed.
			load := d.opCodes[position-len(operands)+index]
			if load.IsCustom || load.CurrentOffset >= site.CurrentOffset || !dynamicPrimitiveLoad(load.Instr.OpCode, typ.Name) {
				continue
			}
			seed, ok := dynamicCaptureSeed(value).(*values.JavaRef)
			matched := false
			for _, produced := range load.stackProduced {
				matched = matched || dynamicCaptureSeed(produced) == seed
			}
			if !ok || seed == nil || seed.IsParam || seed.IsThis || seed.CustomValue != nil || seed.StackVar != nil || !matched {
				continue
			}
			web, known := webs.webOf[load]
			if !known {
				continue
			}
			captureCount++
			if d.Work != nil && d.Work.CheckAlloc(int64(len(d.opCodes))*256+int64(captureCount)*128) != nil {
				return
			}
			if len(captures[web]) == 0 {
				order = append(order, web)
			}
			captures[web] = append(captures[web], capture{value, load, typ.Name})
		}
	}
	for _, web := range order {
		group := captures[web]
		kind := group[0].kind
		load := group[0].load
		valid := true
		for _, capture := range group {
			valid = valid && capture.kind == kind && GetRetrieveIdx(capture.load) == GetRetrieveIdx(load)
		}
		if !valid {
			continue
		}
		definitions := stores[web]
		if len(definitions) < 2 || len(definitions) > 64 {
			continue
		}
		slot := GetRetrieveIdx(load)
		if entry, found := webs.entryWeb[slot]; found && entry == web {
			continue
		}
		complete, conflict := true, false
		refs := map[*values.JavaRef]bool{}
		for _, store := range definitions {
			if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
				return
			}
			infos := d.opcodeIdToRef[store]
			if !dynamicPrimitiveStore(store.Instr.OpCode, kind) || GetStoreIdx(store) != slot || len(infos) != 1 || len(store.stackConsumed) != 1 {
				complete = false
				break
			}
			ref, known := infos[0][0].(*values.JavaRef)
			rhs := dynamicCaptureSeed(store.stackConsumed[0])
			if !known || ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || rhs == nil || !isExactPrimer(rhs.Type(), kind) {
				complete = false
				break
			}
			refs[ref] = true
		}
		if !complete {
			continue
		}
		conflict = len(refs) > 1
		for _, op := range d.opCodes {
			if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
				return
			}
			if op.Instr.OpCode == OP_IINC && GetStoreIdx(op) == slot {
				defs, _ := d.reachingStores(op, slot)
				for _, prior := range defs {
					if webs.webOf[prior] == web {
						complete = false
					}
				}
			}
			other, known := webs.webOf[op]
			if !known || other == web || !isLocalStoreOpcode(op.Instr.OpCode) {
				continue
			}
			for _, info := range d.opcodeIdToRef[op] {
				if ref, ok := info[0].(*values.JavaRef); ok && refs[ref] {
					conflict = true
				}
			}
		}
		if complete && conflict {
			canon := d.bindProvedPrimitiveWeb(webs, web, definitions, kind)
			for _, capture := range group {
				capture.value.ResetValue(canon)
			}
		}
	}
}

// Only stack forwarding wrappers belong to this certificate. Bound their
// depth before matching a seed; a malformed cycle cannot become a LOAD proof.
func dynamicCaptureSeed(value values.JavaValue) values.JavaValue {
	for depth := 0; depth < 32; depth++ {
		slot, forwarded := value.(*values.SlotValue)
		if !forwarded {
			return value
		}
		if slot == nil {
			return nil
		}
		value = slot.GetValue()
	}
	return nil
}

func dynamicPrimitiveLoad(opcode int, kind string) bool {
	for category, name := range []string{types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble} {
		if kind == name {
			return opcode == OP_ILOAD+category || opcode >= OP_ILOAD_0+category*4 && opcode <= OP_ILOAD_3+category*4
		}
	}
	return false
}
func dynamicPrimitiveStore(opcode int, kind string) bool {
	for category, name := range []string{types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble} {
		if kind == name {
			return opcode == OP_ISTORE+category || opcode >= OP_ISTORE_0+category*4 && opcode <= OP_ISTORE_3+category*4
		}
	}
	return false
}
