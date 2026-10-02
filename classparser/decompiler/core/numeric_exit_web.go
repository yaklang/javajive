package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// Join identity follows the immutable reaching-definition partition, including
// stores on an early-break arm. DFS's mutable slot table may have reused a ref
// for an unrelated temporary; never mutate that ref globally. Bind each proved
// web's definitions and load sites to a fresh declaration instead.
//
// Long/float/double have exact source domains in JVM descriptors. The int
// category needs a finite constant-domain proof excluding boolean values.
// Entry parameters, incomplete definitions and mixed domains remain excluded.
func (d *Decompiler) unifyNumericExitWebs() {
	webs := d.slotWebs()
	if webs == nil {
		return
	}
	groups := map[int][]*OpCode{}
	order := []int{}
	for _, op := range d.opCodes {
		if op == nil || op.Instr == nil || !isLocalStoreOpcode(op.Instr.OpCode) {
			continue
		}
		web, ok := webs.webOf[op]
		if !ok {
			continue
		}
		if _, seen := groups[web]; !seen {
			order = append(order, web)
		}
		groups[web] = append(groups[web], op)
	}
	entries := map[int]bool{}
	for _, web := range webs.entryWeb {
		entries[web] = true
	}
	for _, web := range order {
		stores := groups[web]
		if len(stores) < 2 || entries[web] {
			continue
		}
		kind := ""
		intConstants, nonBoolean := true, false
		valid := true
		count := 0
		for op, w := range webs.webOf {
			if w == web && op != nil && op.Instr != nil && isLocalStoreOpcode(op.Instr.OpCode) {
				count++
			}
		}
		if count != len(stores) {
			continue
		}
		for _, store := range stores {
			infos := d.opcodeIdToRef[store]
			if len(infos) != 1 || len(store.stackConsumed) != 1 {
				valid = false
				break
			}
			ref, ok := infos[0][0].(*values.JavaRef)
			if !ok || ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis {
				valid = false
				break
			}
			rhs := values.UnpackSoltValue(store.stackConsumed[0])
			if rhs == nil || rhs.Type() == nil {
				valid = false
				break
			}
			p, ok := rhs.Type().RawType().(*types.JavaPrimer)
			if !ok || (p.Name != types.JavaLong && p.Name != types.JavaFloat && p.Name != types.JavaDouble && p.Name != types.JavaInteger) {
				valid = false
				break
			}
			if kind != "" && kind != p.Name {
				valid = false
				break
			}
			kind = p.Name
			if p.Name == types.JavaInteger {
				literal, ok := rhs.(*values.JavaLiteral)
				if !ok {
					intConstants = false
				} else if number, ok := literal.Data.(int); !ok {
					intConstants = false
				} else {
					nonBoolean = nonBoolean || (number != 0 && number != 1)
				}
				if d.localHasBooleanDescriptorUse(ref) {
					intConstants = false
				}
			}
		}
		// IINC has a read/write definition outside operand-stack stores. Until
		// that definition participates in the canonicalization, a finite
		// integer constant proof must reject increments of this physical slot.
		if kind == types.JavaInteger {
			for _, op := range d.opCodes {
				if op != nil && op.Instr != nil && op.Instr.OpCode == OP_IINC && GetStoreIdx(op) == GetStoreIdx(stores[0]) {
					intConstants = false
				}
			}
		}
		if kind == types.JavaInteger && (!intConstants || !nonBoolean) {
			valid = false
		}
		if !valid {
			continue
		}
		// Preserve an already consistent simulator identity. Canonicalization
		// is needed only when a proved web has split refs, a stale load, or a
		// ref reused by another web. Rebuilding every numeric web needlessly
		// changes declarations and can disturb downstream structuring.
		firstRef := d.opcodeIdToRef[stores[0]][0][0].(*values.JavaRef)
		// A proved multi-definition int web also needs an explicit join
		// identity across lexical branch/monitor exits, even if DFS happened
		// to reuse one ref. Later declaration placement must not split it.
		needsRepair := kind == types.JavaInteger
		for op, infos := range d.opcodeIdToRef {
			w, known := webs.webOf[op]
			if !known || op == nil || op.Instr == nil || !isLocalStoreOpcode(op.Instr.OpCode) {
				continue
			}
			for _, info := range infos {
				ref, ok := info[0].(*values.JavaRef)
				if ok && ((w == web && ref != firstRef) || (w != web && ref == firstRef)) {
					needsRepair = true
				}
			}
		}
		for op, w := range webs.webOf {
			if w != web || op == nil || op.Instr == nil || !isLocalLoadOpcode(op.Instr.OpCode) {
				continue
			}
			for _, value := range op.stackProduced {
				if ref, ok := values.UnpackSoltValue(value).(*values.JavaRef); !ok || ref != firstRef {
					needsRepair = true
				}
			}
		}
		if !needsRepair {
			continue
		}
		// Identity and provisional spelling are separate. Prebinding a shared
		// try/catch variable can retain the current id before lexical minting,
		// so a new identity must inherit a valid simulator spelling rather than
		// render an unnamed root as var-1. Collision renaming still distinguishes
		// unrelated ids; this name never participates in the web proof.
		first := d.opcodeIdToRef[stores[0]][0][0].(*values.JavaRef)
		id := utils.NewRootVariableId()
		id.SetName(first.Id.String())
		canon := values.NewJavaRef(id, nil, types.NewJavaPrimer(kind))
		canon.WebDeclType = canon.Type().Copy()
		canon.SolvedWebIdentity = canon.Id
		// Preserve definitions even if a stale simulator ref appears single-use.
		d.disFoldRef = append(d.disFoldRef, canon)
		for i, store := range stores {
			old := d.opcodeIdToRef[store][0][0].(*values.JavaRef)
			d.disFoldRef = append(d.disFoldRef, old)
			d.opcodeIdToRef[store][0] = [2]any{canon, i == 0}
		}
		for op, w := range webs.webOf {
			if w != web || op == nil || op.Instr == nil || !isLocalLoadOpcode(op.Instr.OpCode) {
				continue
			}
			for _, value := range op.stackProduced {
				if slot, ok := value.(*values.SlotValue); ok {
					slot.ResetValue(canon)
				}
			}
		}
		if d.varUserMap != nil {
			d.varUserMap.ForEach(func(ref *values.JavaRef, pairs []*VarFoldRule) bool {
				for _, pair := range pairs {
					if pair == nil || pair.CurrentOpcode == nil || pair.Replace == nil {
						continue
					}
					op := pair.CurrentOpcode
					w, known := webs.webOf[op]
					if known && w == web && op.Instr != nil && isLocalLoadOpcode(op.Instr.OpCode) {
						pair.Replace(canon)
						d.disFoldRef = append(d.disFoldRef, ref)
					}
				}
				return true
			})
		}
	}
}
