package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"reflect"
)

// A simulator ref can span disjoint reaching-definition webs. Solving its type
// globally makes an earlier definition dictate a later join, and changing its
// identity globally changes unrelated uses. Partition only completely decoded
// ordinary reference webs with that witnessed ownership conflict. Definitions
// and loads are rebound by opcode, before the existing per-web type solver.
func (d *Decompiler) partitionSharedReferenceWebs() {
	webs := d.slotWebs()
	if webs == nil {
		return
	}
	// Dynamic operands already own a declaration/snapshot protocol at their
	// original factory PC. Its forwarding operand is distinct from the LOAD's
	// produced SlotValue. Do not add a same-type binding split while that
	// consumer still owns the original declaration relation.
	snapshotOwned := map[*utils.VariableId]bool{}
	for _, snapshots := range d.evaluationSnapshots {
		for _, snapshot := range snapshots {
			if snapshot.Operand {
				if ref, ok := dynamicCaptureSeed(snapshot.Value).(*values.JavaRef); ok && ref != nil && ref.Id != nil {
					snapshotOwned[ref.Id] = true
				}
			}
		}
	}
	groups := map[int][]*OpCode{}
	owners := map[*values.JavaRef]map[int]bool{}
	firstOwner := map[*values.JavaRef]int{}
	order := []int{}
	complete := map[int]bool{}
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil || !isReferenceStoreOpcode(op.Instr.OpCode) {
			continue
		}
		w, known := webs.webOf[op]
		if !known {
			continue
		}
		if _, exists := groups[w]; !exists {
			order = append(order, w)
			complete[w] = true
		}
		groups[w] = append(groups[w], op)
		infos := d.opcodeIdToRef[op]
		if len(infos) != 1 || len(op.stackConsumed) != 1 {
			complete[w] = false
			continue
		}
		ref, ok := infos[0][0].(*values.JavaRef)
		if !ok || ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis {
			complete[w] = false
			continue
		}
		if owners[ref] == nil {
			owners[ref] = map[int]bool{}
			firstOwner[ref] = w
		}
		owners[ref][w] = true
	}
	for _, w := range webs.entryWeb {
		complete[w] = false
	}
	// A local-to-local copy can still have the same provisional type as an
	// earlier use of its destination slot. Its source web may widen only in
	// the following fixed-point solver. Record the actual LOAD producer now;
	// comparing those provisional types cannot establish lifetime equality.
	copyLoads := map[values.JavaValue]*OpCode{}
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil || !isReferenceLoadOpcode(op.Instr.OpCode) {
			continue
		}
		for _, value := range op.stackProduced {
			if _, ok := value.(*values.SlotValue); ok {
				copyLoads[value] = op
			}
		}
	}
	// A store absent from the decoded opcode list is not a complete web.
	for op, w := range webs.webOf {
		if op != nil && op.Instr != nil && isLocalStoreOpcode(op.Instr.OpCode) {
			found := false
			for _, store := range groups[w] {
				found = found || op == store
			}
			if !found {
				complete[w] = false
			}
		}
	}
	// A caught exception copied into a pending completion variable is also a
	// distinct computational definition, but the structured finally protocol
	// requires those copies to share one source binder. Track that constraint
	// through the same witnessed local copies instead of guessing from Throwable
	// types or variable names. Unknown handler entry ownership grants no split.
	handlerCopies := map[int]bool{}
	copyEdges := map[int][]int{}
	handlersKnown := true
	byPC := map[uint16]*OpCode{}
	for _, op := range d.opCodes {
		if op != nil {
			byPC[op.CurrentOffset] = op
		}
	}
	queue := []int{}
	for _, row := range d.ExceptionTable {
		if row == nil {
			handlersKnown = false
			continue
		}
		entry := byPC[row.HandlerPc]
		web, known := webs.webOf[entry]
		if !known || entry == nil || entry.Instr == nil || !isReferenceStoreOpcode(entry.Instr.OpCode) {
			handlersKnown = false
			continue
		}
		if !handlerCopies[web] {
			handlerCopies[web] = true
			queue = append(queue, web)
		}
	}
	for target, stores := range groups {
		for _, store := range stores {
			if len(store.stackConsumed) == 1 {
				if load := copyLoads[store.stackConsumed[0]]; load != nil {
					if source, known := webs.webOf[load]; known {
						copyEdges[source] = append(copyEdges[source], target)
					}
				}
			}
		}
	}
	for i := 0; i < len(queue); i++ {
		for _, target := range copyEdges[queue[i]] {
			if !handlerCopies[target] {
				handlerCopies[target] = true
				queue = append(queue, target)
			}
		}
	}
	for _, w := range order {
		if !complete[w] {
			continue
		}
		stores := groups[w]
		first := d.opcodeIdToRef[stores[0]][0][0].(*values.JavaRef)
		seedType := first.Type()
		singletonConflict := false
		if len(stores) == 1 {
			// A later disjoint single-definition web owns a distinct lifetime,
			// even when its type is identical to the earlier variable's type.
			// Otherwise two independent locals become one mutable source binder,
			// preventing a fresh loop-local value from being captured faithfully.
			// Keep the first web's identity and rebind only this later web.
			// Use the actual defining operand, including any original CHECKCAST,
			// instead of adding a check to make that stale declaration compile.
			value := stores[0].stackConsumed[0]
			var definedType types.JavaType
			if value != nil {
				definedType = value.Type()
			}
			if _, literal := values.UnpackSoltValue(value).(*values.JavaClassValue); literal {
				// The payload names the represented class, while the LDC
				// produces a java.lang.Class reference on the operand stack.
				definedType = types.NewJavaClass("java.lang.Class")
			}
			// Duplicated catch/finally paths can have separate computational
			// definitions but still require a shared pending-exception source
			// binder. Until those source constraints are represented explicitly,
			// the general equal-type permission requires no raw handlers. A
			// witnessed copy from a complete ordinary web is a new value whose
			// type still needs solving. Handler completion copies above and dynamic
			// snapshots retain their separate source-binding constraints.
			differentType := definedType != nil && seedType != nil && !reflect.DeepEqual(seedType.RawType(), definedType.RawType())
			independentCopy := false
			if load := copyLoads[value]; load != nil && GetRetrieveIdx(load) != GetStoreIdx(stores[0]) {
				if sourceWeb, known := webs.webOf[load]; known && sourceWeb != w && complete[sourceWeb] && handlersKnown && !handlerCopies[sourceWeb] {
					independentCopy = true
				}
			}
			separateLifetime := firstOwner[first] != w && (len(d.ExceptionTable) == 0 || independentCopy) && !snapshotOwned[first.Id]
			if len(owners[first]) > 1 && value != nil && values.UnpackSoltValue(value) != values.JavaNull && !values.IsNullLiteral(values.UnpackSoltValue(value)) &&
				definedType != nil && seedType != nil && (differentType || separateLifetime) {
				seedType = definedType
				singletonConflict = true
			}
			if !singletonConflict {
				continue
			}
		}
		conflict := false
		distinct := map[*values.JavaRef]bool{}
		for _, op := range stores {
			r := d.opcodeIdToRef[op][0][0].(*values.JavaRef)
			distinct[r] = true
			conflict = conflict || len(owners[r]) > 1
		}
		if !conflict || (len(distinct) < 2 && !singletonConflict) || seedType == nil {
			continue
		}
		d.tracef("var-fold", "partition shared reference web=%d stores=%d", w, len(stores))
		id := utils.NewRootVariableId()
		id.SetName(first.Id.String())
		canon := values.NewJavaRef(id, nil, seedType.Copy())
		canon.SolvedWebIdentity = id
		if singletonConflict {
			// ResetValue must not write a load's stale DFS snapshot back into
			// the freshly owned declaration while rebinding its use sites.
			canon.WebDeclType = seedType.Copy()
		}
		d.disFoldRef = append(d.disFoldRef, canon)
		for i, op := range stores {
			d.opcodeIdToRef[op][0] = [2]any{canon, i == 0}
		}
		for op, owner := range webs.webOf {
			if owner != w || op == nil || op.Instr == nil || !isReferenceLoadOpcode(op.Instr.OpCode) {
				continue
			}
			for _, v := range op.stackProduced {
				if slot, ok := v.(*values.SlotValue); ok {
					slot.ResetValue(canon)
				}
			}
		}
		if d.varUserMap != nil {
			moved := []*VarFoldRule{}
			remaining := map[*values.JavaRef][]*VarFoldRule{}
			d.varUserMap.ForEach(func(ref *values.JavaRef, pairs []*VarFoldRule) bool {
				remaining[ref] = nil
				for _, pair := range pairs {
					if pair == nil || pair.CurrentOpcode == nil || pair.Replace == nil {
						remaining[ref] = append(remaining[ref], pair)
						continue
					}
					op := pair.CurrentOpcode
					owner, known := webs.webOf[op]
					if known && owner == w && op.Instr != nil && isReferenceLoadOpcode(op.Instr.OpCode) {
						pair.Replace(canon)
						moved = append(moved, pair)
					} else {
						remaining[ref] = append(remaining[ref], pair)
					}
				}
				return true
			})
			for ref, pairs := range remaining {
				d.varUserMap.Set(ref, pairs)
			}
			d.varUserMap.Set(canon, moved)
		}
	}
}
