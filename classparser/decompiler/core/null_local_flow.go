package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Propagate only the null constant, using complete ordinary reaching-definition
// webs. The worklist starts from literal-null definitions; copies become known
// only after all their defining webs are known. Cycles, entry definitions,
// ambiguous simulator identities and missing values fail closed. No ref.Val or
// printed name is evidence. Each dependency edge is processed once.
func (d *Decompiler) propagateNullOnlyLocalLoads() {
	// Protected operand-stack merges are lowered later, and their definitions
	// are absent from the ordinary-edge web partition. That partition cannot
	// authorize constant propagation in an exception-bearing method.
	if len(d.ExceptionTable) != 0 {
		return
	}
	webs := d.slotWebs()
	if webs == nil {
		return
	}
	stores := map[int][]*OpCode{}
	entries := map[int]bool{}
	for _, w := range webs.entryWeb {
		entries[w] = true
	}
	for op, w := range webs.webOf {
		if op != nil && op.Instr != nil && isLocalStoreOpcode(op.Instr.OpCode) {
			stores[w] = append(stores[w], op)
		}
	}
	refWeb := map[*values.JavaRef]int{}
	ambiguous := map[*values.JavaRef]bool{}
	remember := func(ref *values.JavaRef, w int) {
		if ref == nil {
			return
		}
		if old, exists := refWeb[ref]; exists && old != w {
			ambiguous[ref] = true
		}
		refWeb[ref] = w
	}
	for op, infos := range d.opcodeIdToRef {
		w, known := webs.webOf[op]
		if !known || op == nil || op.Instr == nil || !isReferenceStoreOpcode(op.Instr.OpCode) {
			continue
		}
		for _, info := range infos {
			if ref, ok := info[0].(*values.JavaRef); ok {
				remember(ref, w)
			}
		}
	}
	for op, w := range webs.webOf {
		if op == nil || op.Instr == nil || !isReferenceLoadOpcode(op.Instr.OpCode) {
			continue
		}
		for _, v := range op.stackProduced {
			if ref, ok := values.UnpackSoltValue(v).(*values.JavaRef); ok {
				remember(ref, w)
			}
		}
	}
	referenceWeb := func(v values.JavaValue) (int, bool) {
		ref, ok := values.UnpackSoltValue(v).(*values.JavaRef)
		if !ok || ref == nil || ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || ambiguous[ref] {
			return 0, false
		}
		w, known := refWeb[ref]
		return w, known
	}
	pending := map[int]int{}
	users := map[int][]int{}
	queue := []int{}
	for w, defs := range stores {
		if entries[w] || len(defs) == 0 {
			continue
		}
		deps := map[int]bool{}
		valid := true
		for _, op := range defs {
			if !isReferenceStoreOpcode(op.Instr.OpCode) || len(op.stackConsumed) != 1 {
				valid = false
				break
			}
			v := values.UnpackSoltValue(op.stackConsumed[0])
			if values.IsNullLiteral(v) {
				continue
			}
			dep, known := referenceWeb(v)
			if !known {
				valid = false
				break
			}
			deps[dep] = true
		}
		if !valid {
			continue
		}
		pending[w] = len(deps)
		for dep := range deps {
			users[dep] = append(users[dep], w)
		}
		if len(deps) == 0 {
			queue = append(queue, w)
		}
	}
	proved := map[int]bool{}
	for i := 0; i < len(queue); i++ {
		w := queue[i]
		proved[w] = true
		for _, user := range users[w] {
			pending[user]--
			if pending[user] == 0 {
				queue = append(queue, user)
			}
		}
	}
	literal := func() values.JavaValue { return values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object")) }
	for op, w := range webs.webOf {
		if !proved[w] || op == nil || op.Instr == nil || !isReferenceLoadOpcode(op.Instr.OpCode) {
			continue
		}
		for _, v := range op.stackProduced {
			if slot, ok := v.(*values.SlotValue); ok {
				slot.ResetValue(literal())
			}
		}
	}
	for op, snapshots := range d.evaluationSnapshots {
		for i := range snapshots {
			if snapshots[i].Operand {
				w, known := referenceWeb(snapshots[i].Value)
				if (known && proved[w]) || values.IsNullLiteral(values.UnpackSoltValue(snapshots[i].Value)) {
					snapshots[i].Value = literal()
				}
			}
		}
		d.evaluationSnapshots[op] = snapshots
	}
	if d.varUserMap != nil {
		d.varUserMap.ForEach(func(_ *values.JavaRef, pairs []*VarFoldRule) bool {
			for _, p := range pairs {
				if p == nil || p.CurrentOpcode == nil || p.Replace == nil {
					continue
				}
				op := p.CurrentOpcode
				w, known := webs.webOf[op]
				if known && proved[w] && op.Instr != nil && isReferenceLoadOpcode(op.Instr.OpCode) {
					p.Replace(literal())
				}
			}
			return true
		})
	}
}
func isReferenceStoreOpcode(op int) bool {
	switch op {
	case OP_ASTORE, OP_ASTORE_0, OP_ASTORE_1, OP_ASTORE_2, OP_ASTORE_3:
		return true
	}
	return false
}
