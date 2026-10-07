package core

import (
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The JVM stack holds values, whereas a deferred Java expression can read a
// mutable location again. A surviving primitive operand crossing a statement
// write therefore needs an immutable copy at its original producer, before the
// writer's RHS effects. Copying it at the eventual arithmetic/CMP consumer is
// too late. Reference allocation/initialization requires a separate proof.
func (d *Decompiler) preserveStackAcrossWrite(sim *StackSimulationImpl, write *OpCode) error {
	if sim == nil || write == nil || write.Instr == nil || sim.Size() == 0 || !stackLifetimeWrite(write.Instr.OpCode) {
		return nil
	}
	if err := d.chargeNodeCopies(sim.Size()); err != nil {
		return err
	}
	items := make([]values.JavaValue, 0, sim.Size())
	for s := sim.stackEntry; s != nil && s.parent != nil; s = s.parent {
		items = append(items, s.value)
	}
	type capture struct {
		index    int
		value    values.JavaValue
		producer *OpCode
	}
	var plan []capture
	for i := len(items) - 1; i >= 0; i-- {
		value := items[i]
		producer := d.stackValueProducers[value]
		if value == nil || value.Type() == nil || producer == nil || producer.Instr == nil || producer == write {
			continue
		}
		typ, primitive := value.Type().RawType().(*types.JavaPrimer)
		if !primitive || typ.Name == types.JavaString || typ.Name == types.JavaVoid {
			continue
		}
		real := values.UnpackSoltValue(value)
		if _, constant := real.(*values.JavaLiteral); constant {
			continue
		}
		if _, ref := real.(*values.JavaRef); ref && !LocalAccessOf(producer.Instr.OpCode).Read {
			continue
		}
		// A pure local expression commutes with heap writes and stores to
		// unrelated locals. Capture only a dependency that the store changes;
		// observable or opaque expressions retain their original evaluation.
		effect, reads := values.InspectValue(value)
		if effect == 0 {
			conflict := false
			for _, info := range d.opcodeIdToRef[write] {
				written, _ := info[0].(*values.JavaRef)
				for read := range reads {
					conflict = conflict || values.SameLocal(read, written)
				}
			}
			if LocalAccessOf(producer.Instr.OpCode).Read && LocalAccessOf(write.Instr.OpCode).Write {
				conflict = conflict || GetRetrieveIdx(producer) == GetStoreIdx(write)
			}
			if !conflict {
				continue
			}
		}
		if !d.stackLifetimeProducerPath(producer, write) {
			if d.Work != nil && d.Work.Err() != nil {
				return d.Work.Err()
			}
			return fmt.Errorf("stack value at %d crosses write %d without producer dominance", producer.CurrentOffset, write.CurrentOffset)
		}
		plan = append(plan, capture{i, value, producer})
	}
	// Complete every path certificate before publishing any new declaration.
	// A later invalid operand must not leave a partially installed snapshot plan.
	for _, item := range plan {
		i, value, producer := item.index, item.value, item.producer
		if d.stackLifetimeCopies == nil {
			d.stackLifetimeCopies = map[values.JavaValue]*values.JavaRef{}
		}
		saved := d.stackLifetimeCopies[value]
		if saved == nil {
			saved = sim.NewVar(value)
			saved.ResetVarType(value.Type().Copy())
			d.stackLifetimeCopies[value] = saved
			d.disFoldRef = append(d.disFoldRef, saved)
			if d.evaluationSnapshots == nil {
				d.evaluationSnapshots = map[*OpCode][]EvaluationSnapshot{}
			}
			d.evaluationSnapshots[producer] = append(d.evaluationSnapshots[producer], EvaluationSnapshot{Ref: saved, Value: value, OriginPC: int(producer.CurrentOffset), Operand: true})
		}
		items[i] = saved
		for _, use := range d.stackLifetimeUseViews[value] {
			use.ResetValue(saved)
		}
	}
	// Do not mutate shared predecessor stack entries or count these private
	// copies as decoded pushes/pops. Their defining nodes belong to the producer.
	sim.stackEntry = NewEmptyStackEntry()
	for i := len(items) - 1; i >= 0; i-- {
		sim.Push(items[i])
	}
	return nil
}

// Stack evidence is shared by predecessor simulations. A rendered consumer
// needs its own view because a later-visited branch may reveal a lifetime
// hazard. Redirect those views after the complete copy plan succeeds, including
// consumers visited before the writing arm; never rewrite the snapshot RHS or
// the original stack word. Literals and already materialized refs need no view.
func (d *Decompiler) stackLifetimeUseView(value values.JavaValue) (values.JavaValue, error) {
	if value == nil || value.Type() == nil || d.stackValueProducers[value] == nil || d.stackValueProducers[value].Instr == nil {
		return value, nil
	}
	if saved := d.stackLifetimeCopies[value]; saved != nil {
		return saved, nil
	}
	typ, primitive := value.Type().RawType().(*types.JavaPrimer)
	if !primitive || typ.Name == types.JavaString || typ.Name == types.JavaVoid {
		return value, nil
	}
	if _, constant := values.UnpackSoltValue(value).(*values.JavaLiteral); constant {
		return value, nil
	}
	// A comparison owns the independently tracked operand views already.
	// Its direct zero-branch consumer needs that comparison node to preserve
	// the original unordered bias; it is not another mutable location read.
	if _, comparison := value.(*values.JavaCompare); comparison {
		return value, nil
	}
	if _, local := values.UnpackSoltValue(value).(*values.JavaRef); local && !LocalAccessOf(d.stackValueProducers[value].Instr.OpCode).Read {
		return value, nil
	}
	if err := d.chargeNodeCopies(1); err != nil {
		return value, err
	}
	view := values.NewSlotValue(value, value.Type())
	if d.stackLifetimeUseViews == nil {
		d.stackLifetimeUseViews = map[values.JavaValue][]*values.SlotValue{}
	}
	d.stackLifetimeUseViews[value] = append(d.stackLifetimeUseViews[value], view)
	return view, nil
}
func stackLifetimeWrite(kind int) bool {
	if LocalAccessOf(kind).Write {
		return true
	}
	switch kind {
	case OP_PUTFIELD, OP_PUTSTATIC, OP_IASTORE, OP_LASTORE, OP_FASTORE, OP_DASTORE, OP_AASTORE, OP_BASTORE, OP_CASTORE, OP_SASTORE:
		return true
	}
	return false
}

// The original producer must dominate the write: every backward route either
// reaches that producer or remains in a closed loop entered through it. Any
// other entry, unknown edge or producer outside the bounded region refuses.
// Forks are allowed because the immutable word already existed before them.
// The copy stays at the producer under its original exception handlers.
func (d *Decompiler) stackLifetimeProducerPath(producer, write *OpCode) bool {
	if producer == nil || write == nil || producer.Instr == nil || write.Instr == nil {
		return false
	}
	pending := []*OpCode{write}
	seen := map[*OpCode]bool{}
	reached := false
	edges := 0
	for len(pending) > 0 {
		n := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if n == producer {
			reached = true
			continue
		}
		if seen[n] {
			continue
		}
		if n == nil || n.Instr == nil || n.CurrentOffset < producer.CurrentOffset || len(n.Source) == 0 || len(seen) >= 511 {
			return false
		}
		edges += len(n.Source)
		if edges > 8192 {
			return false
		}
		if d.Work != nil && (d.Work.Charge(workbudget.CounterGraphScans, 1) != nil || d.Work.Charge(workbudget.CounterGraphEdges, int64(len(n.Source))) != nil) {
			return false
		}
		seen[n] = true
		incoming := map[*OpCode]bool{}
		for _, prev := range n.Source {
			if prev == nil || incoming[prev] {
				return false
			}
			incoming[prev] = true
			edges += len(prev.Target)
			if edges > 8192 || d.Work != nil && d.Work.Charge(workbudget.CounterGraphEdges, int64(len(prev.Target))) != nil {
				return false
			}
			witnesses := 0
			for _, next := range prev.Target {
				if next == n {
					witnesses++
				}
			}
			if witnesses != 1 {
				return false
			}
			pending = append(pending, prev)
		}
	}
	return reached
}

// Constructor spill removal can use a lifetime copy's real value producer,
// rather than inventing a decoded STORE or DUP. This identity witness grants
// no motion: the caller still proves the entire entry sequence, unique use,
// argument order, dependencies and unchanged handler domain.
func (d *Decompiler) stackLifetimeSnapshotProducer(value values.JavaValue, ref *values.JavaRef, producer *OpCode) bool {
	if d == nil || value == nil || ref == nil || producer == nil || producer.Instr == nil ||
		ref.IsParam || ref.IsThis || d.stackValueProducers[value] != producer || d.stackLifetimeCopies[value] != ref {
		return false
	}
	witnesses := 0
	for _, snapshot := range d.evaluationSnapshots[producer] {
		if snapshot.Ref == ref {
			if snapshot.Value != value || !snapshot.Operand || snapshot.OriginPC != int(producer.CurrentOffset) {
				return false
			}
			witnesses++
		}
	}
	produced := false
	for _, original := range producer.stackProduced {
		produced = produced || original == value
	}
	return witnesses == 1 && produced
}
