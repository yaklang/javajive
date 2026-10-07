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
		if value == nil || value.Type() == nil || producer == nil || producer.Instr == nil {
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
		if !d.stackLifetimeProducerPath(producer, write) {
			return fmt.Errorf("stack value at %d crosses write %d without a closed producer path", producer.CurrentOffset, write.CurrentOffset)
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
	}
	// Do not mutate shared predecessor stack entries or count these private
	// copies as decoded pushes/pops. Their defining nodes belong to the producer.
	sim.stackEntry = NewEmptyStackEntry()
	for i := len(items) - 1; i >= 0; i-- {
		sim.Push(items[i])
	}
	return nil
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

// A forward single-entry path certifies that this producer runs exactly once
// before the write. Snapshots stay at the producer under its original handlers;
// no evaluation is hoisted across a handler boundary. Joins/backedges require
// edge-owned copies rather than selecting one representative stack expression.
func (d *Decompiler) stackLifetimeProducerPath(producer, write *OpCode) bool {
	if producer == nil || write == nil || producer.Instr == nil || write.Instr == nil {
		return false
	}
	for n, steps := write, 0; steps < 512; steps++ {
		if d.Work != nil && d.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		if n == producer {
			return true
		}
		if n == nil || len(n.Source) != 1 {
			return false
		}
		prev := n.Source[0]
		if prev == nil || prev.CurrentOffset >= n.CurrentOffset || len(prev.Target) != 1 || prev.Target[0] != n {
			return false
		}
		n = prev
	}
	return false
}
