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
	groups := map[int][]*OpCode{}
	owners := map[*values.JavaRef]map[int]bool{}
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
		}
		owners[ref][w] = true
	}
	for _, w := range webs.entryWeb {
		complete[w] = false
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
	for _, w := range order {
		if !complete[w] {
			continue
		}
		stores := groups[w]
		first := d.opcodeIdToRef[stores[0]][0][0].(*values.JavaRef)
		seedType := first.Type()
		singletonConflict := false
		if len(stores) == 1 {
			// Disjoint single-definition webs still require distinct identities
			// when the simulator reused a narrower local from an earlier web.
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
			if len(owners[first]) > 1 && value != nil && values.UnpackSoltValue(value) != values.JavaNull && !values.IsNullLiteral(values.UnpackSoltValue(value)) &&
				definedType != nil && seedType != nil && !reflect.DeepEqual(seedType.RawType(), definedType.RawType()) {
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
