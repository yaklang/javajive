package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// LOAD; typed-one; ADD/SUB; [narrow]; DUP; STORE retains the NEW word.
// Simulation materializes that word at DUP. Its immutable materialization,
// original arithmetic producer, load binding and two uses must all agree;
// neither an equal-looking expression nor a same-named temporary suffices.
func (d *Decompiler) immediateLocalTypedPreUpdate(store *OpCode, ref *values.JavaRef, surviving, stored values.JavaValue) (*OpCode, values.JavaValue, string, bool) {
	if d == nil || ref == nil || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || ref.Type() == nil || surviving == nil || stored == nil {
		return nil, nil, "", false
	}
	primitive, ok := ref.Type().RawType().(*types.JavaPrimer)
	if !ok || primitive == nil {
		return nil, nil, "", false
	}
	protocol, ok := localPostProtocols[primitive.Name]
	if !ok {
		return nil, nil, "", false
	}
	n := 5
	if protocol.narrow != 0 {
		n++
	}
	path, closed := d.localUpdatePrivatePath(store, n)
	if !closed {
		return nil, nil, "", false
	}
	load, producer, duplicate := path[0], path[n-3], path[n-2]
	if !localPostAccess(load, protocol.load, true) || !localPostAccess(store, protocol.store, false) || GetRetrieveIdx(load) != GetStoreIdx(store) || path[1].Instr.OpCode != protocol.one || duplicate.Instr.OpCode != protocol.dup || protocol.narrow != 0 && path[3].Instr.OpCode != protocol.narrow {
		return nil, nil, "", false
	}
	if _, valid := localPostInstructionWidth(store); !valid {
		return nil, nil, "", false
	}
	op := values.INC
	switch path[2].Instr.OpCode {
	case protocol.add:
	case protocol.sub:
		op = values.DEC
	default:
		return nil, nil, "", false
	}
	if len(load.stackProduced) == 0 {
		return nil, nil, "", false
	}
	loaded := load.stackProduced[len(load.stackProduced)-1]
	if loaded == nil || d.stackValueProducers[loaded] != load || values.UnpackSoltValue(loaded) != ref {
		return nil, nil, "", false
	}
	tmp, ok := values.UnpackSoltValue(surviving).(*values.JavaRef)
	if !ok || tmp == nil || tmp == ref || values.UnpackSoltValue(stored) != tmp || d.stackValueProducers[surviving] != duplicate {
		return nil, nil, "", false
	}
	converted, infos := d.dupConvertedRefValue[duplicate], d.opcodeIdToRef[duplicate]
	if len(converted) != 1 || len(infos) != 1 || infos[0][0] != tmp || infos[0][1] != true || len(producer.stackProduced) == 0 {
		return nil, nil, "", false
	}
	word := producer.stackProduced[len(producer.stackProduced)-1]
	if word == nil || word.Type() == nil || tmp.Type() == nil {
		return nil, nil, "", false
	}
	wordType, wordNumeric := word.Type().RawType().(*types.JavaPrimer)
	tmpType, tmpNumeric := tmp.Type().RawType().(*types.JavaPrimer)
	if !wordNumeric || !tmpNumeric || wordType == nil || tmpType == nil || wordType.Name != primitive.Name || tmpType.Name != primitive.Name {
		return nil, nil, "", false
	}
	pc, kind, witness := tmp.OriginalStackMaterializationWitness(word)
	if !witness || converted[0] != word || pc != int(duplicate.CurrentOffset) || kind != protocol.dup || d.stackValueProducers[word] != producer {
		return nil, nil, "", false
	}
	for _, produced := range duplicate.stackProduced {
		if produced == surviving {
			return duplicate, loaded, op, true
		}
	}
	return nil, nil, "", false
}

// Some compilers write a narrowed NEW word and reload it instead of duplicating
// it. The reload owns the expression result; prove the original private path and
// all three accesses' exact binding before suppressing the intervening store.
func (d *Decompiler) immediateLocalTypedReloadUpdate(reload *OpCode, loaded values.JavaValue) (string, bool) {
	if d == nil || reload == nil || loaded == nil {
		return "", false
	}
	ref, ok := values.UnpackSoltValue(loaded).(*values.JavaRef)
	if !ok || ref == nil || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || ref.Type() == nil {
		return "", false
	}
	primitive, ok := ref.Type().RawType().(*types.JavaPrimer)
	if !ok || primitive == nil {
		return "", false
	}
	protocol, ok := localPostProtocols[primitive.Name]
	if !ok {
		return "", false
	}
	n := 5
	if protocol.narrow != 0 {
		n++
	}
	path, closed := d.localUpdatePrivatePath(reload, n)
	if !closed {
		return "", false
	}
	first, producer, store := path[0], path[n-3], path[n-2]
	// An earlier argument can leave an OLD read alive across this same update.
	// The post-result certificate may already own its write. A later reload is
	// then just a read of the updated local, not permission for a second update.
	if store.LocalStoreFoldedToPostOp {
		return "", false
	}
	if !localPostAccess(first, protocol.load, true) || !localPostAccess(store, protocol.store, false) || !localPostAccess(reload, protocol.load, true) || GetRetrieveIdx(first) != GetStoreIdx(store) || GetRetrieveIdx(first) != GetRetrieveIdx(reload) || path[1].Instr.OpCode != protocol.one || protocol.narrow != 0 && path[3].Instr.OpCode != protocol.narrow {
		return "", false
	}
	if _, valid := localPostInstructionWidth(reload); !valid {
		return "", false
	}
	op := values.INC
	switch path[2].Instr.OpCode {
	case protocol.add:
	case protocol.sub:
		op = values.DEC
	default:
		return "", false
	}
	infos := d.opcodeIdToRef[store]
	if len(infos) == 0 || infos[len(infos)-1][0] != ref || infos[len(infos)-1][1] != false || len(first.stackProduced) == 0 || len(producer.stackProduced) == 0 {
		return "", false
	}
	old := first.stackProduced[len(first.stackProduced)-1]
	word := producer.stackProduced[len(producer.stackProduced)-1]
	if old == nil || word == nil || values.UnpackSoltValue(old) != ref || d.stackValueProducers[old] != first || d.stackValueProducers[word] != producer || values.OriginalStackLifetimeUse(d.slotStoreValue[store]) != word || word.Type() == nil {
		return "", false
	}
	wordType, ok := word.Type().RawType().(*types.JavaPrimer)
	if !ok || wordType == nil || wordType.Name != primitive.Name {
		return "", false
	}
	return op, true
}
